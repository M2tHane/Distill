package gui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/dade/agentlog"
	"github.com/dade/distill/internal/env"
)

type sessionsState struct {
	all      []agentlog.Session
	loaded   bool
	scanning bool
	err      string

	tab      int // 0: every agent, then agents[tab-1]
	query    string
	selected map[string]agentlog.Session // by path, across tabs
	expanded map[string]bool             // projects shown open, by directory
	list     ui.ListState

	// The transcript open beside the list.
	view viewState

	confirmDelete bool
	toDelete      []agentlog.Session
	trashOpen     bool
	trashItems    []agentlog.TrashItem
}

func (s *sessionsState) init() {
	s.selected = map[string]agentlog.Session{}
	s.expanded = map[string]bool{}
	s.view.split = 520
}

func (s *sessionsState) count(a agentlog.Agent) int {
	n := 0
	for _, x := range s.all {
		if x.Agent == a {
			n++
		}
	}
	return n
}

// group is a project's sessions.
type group struct {
	dir      string
	sessions []agentlog.Session
}

// row is a row of the list: a project, or one of its sessions.
type row struct {
	g       *group
	s       *agentlog.Session
	last    bool // the last session of its project
	project bool
}

// visible returns the sessions of the tab matching the query, by project,
// the project last active first.
func (s *sessionsState) visible() []*group {
	q := strings.ToLower(strings.TrimSpace(s.query))
	byDir := map[string]*group{}
	var groups []*group
	for _, x := range s.all {
		if s.tab > 0 && x.Agent != env.Agents[s.tab-1] {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(x.Title), q) && !strings.Contains(strings.ToLower(x.CWD), q) {
			continue
		}
		g := byDir[x.CWD]
		if g == nil {
			g = &group{dir: x.CWD}
			byDir[x.CWD] = g
			groups = append(groups, g)
		}
		g.sessions = append(g.sessions, x)
	}
	// all is newest first, so groups are in order of their latest session.
	return groups
}

func (s *sessionsState) rows(groups []*group) []row {
	var rows []row
	for _, g := range groups {
		rows = append(rows, row{g: g, project: true})
		if s.expanded[g.dir] || s.query != "" {
			for i := range g.sessions {
				rows = append(rows, row{g: g, s: &g.sessions[i], last: i == len(g.sessions)-1})
			}
		}
	}
	return rows
}

func (a *app) sessionsPage(c *ui.Context) {
	s := &a.sessions
	t := c.Theme()
	header(c, "会话", func() {
		if s.scanning {
			ui.Spinner(c).Size(14, 14)
		}
		if iconButton(c, iconRefresh, "刷新").Clicked() && !s.scanning {
			go a.rescan()
		}
		b := ui.Button(c, "").Children(func() {
			ui.Icon(c, iconTrash).FontSize(13)
			ui.Text(c, "回收站").FontSize(13)
		})
		if b.Clicked() {
			a.openTrash()
		}
	})
	if s.view.session == nil {
		ui.Column(c).Grow(1).MinHeight(0).Children(func() { a.sessionList(c) })
	} else {
		ui.Split(c, &s.view.split, func() {
			ui.Column(c).Fill().MinWidth(0).Children(func() { a.sessionList(c) })
		}, func() {
			ui.Column(c).Fill().MinWidth(0).BorderWidth(0, 0, 0, 1).BorderColor(t.Border).Children(func() { a.transcriptPane(c) })
		}).Grow(1).MinHeight(0)
	}
	a.deleteDialog(c)
	a.trashDialog(c)
}

func (a *app) sessionList(c *ui.Context) {
	s := &a.sessions
	t := c.Theme()
	groups := s.visible()
	var shown []agentlog.Session
	for _, g := range groups {
		shown = append(shown, g.sessions...)
	}

	// The agents' tabs and the filter.
	ui.Row(c).Height(paneHeader).Padding(0, 16, 0, 20).Gap(10).Shrink(0).Children(func() {
		a.agentTabs(c)
		ui.Spacer(c)
		searchBox(c, &s.query, "筛选会话").Grow(1).MinWidth(110).MaxWidth(240)
	})

	// Select all, and what the selection can do.
	ui.Row(c).Padding(4, 16, 8, 20).Gap(10).Children(func() {
		all, some := true, false
		for _, x := range shown {
			if _, ok := s.selected[x.Path]; ok {
				some = true
			} else {
				all = false
			}
		}
		all = all && len(shown) > 0
		v := all
		cb := ui.Checkbox(c.Key("all"), &v, fmt.Sprintf("%d 个会话", len(shown)))
		if some && !all {
			cb.Mixed()
		}
		if cb.Changed() {
			for _, x := range shown {
				if v {
					s.selected[x.Path] = x
				} else {
					delete(s.selected, x.Path)
				}
			}
		}
		ui.Spacer(c)
		if s.err != "" {
			ui.Text(c, s.err).TextColor(t.Danger).FontSize(12).SingleLine()
		}
	})

	rows := s.rows(groups)
	s.list.Key = func(i int) any {
		if rows[i].project {
			return "p:" + rows[i].g.dir
		}
		return rows[i].s.Path
	}
	ui.List(c, &s.list, len(rows), func(i int) {
		r := rows[i]
		if r.project {
			a.projectRow(c, r.g)
		} else {
			a.sessionRow(c, r.s)
		}
	}).Grow(1).Padding(0, 16, 16, 16).Children(func() {
		if len(rows) == 0 {
			msg := "没有会话"
			if !s.loaded {
				msg = "正在读取会话…"
			} else if s.query != "" {
				msg = "没有匹配的会话"
			}
			muted(c, msg).Padding(24).AlignSelf(ui.Center)
		}
	})
	a.selectionBar(c)
}

func (a *app) projectRow(c *ui.Context, g *group) {
	s := &a.sessions
	t := c.Theme()
	open := s.expanded[g.dir] || s.query != ""
	n := 0
	for _, x := range g.sessions {
		if _, ok := s.selected[x.Path]; ok {
			n++
		}
	}
	ui.Row(c).MarginY(4).Padding(10, 14).Gap(10).Radius(10).Background(ui.Hex("#F4F4F5")).Children(func() {
		v := n == len(g.sessions)
		cb := ui.Checkbox(c.Key("cb"), &v, "")
		if n > 0 && n < len(g.sessions) {
			cb.Mixed()
		}
		if cb.Changed() {
			for _, x := range g.sessions {
				if v {
					s.selected[x.Path] = x
				} else {
					delete(s.selected, x.Path)
				}
			}
		}
		head := ui.Row(c).Grow(1).MinWidth(0).Gap(10).Cursor(ui.CursorPointer).Children(func() {
			rot := float32(0)
			if open {
				rot = 90
			}
			ui.Icon(c, iconChevron).FontSize(13).Rotate(rot).TextColor(t.TextMuted)
			name := filepath.Base(g.dir)
			if g.dir == "" {
				name = "（未知目录）"
			}
			ui.Text(c, name).Bold().SingleLine().Shrink(0).MaxWidth(260)
			muted(c, g.dir).Font("monospace").FontSize(11).SingleLine().Shrink(1).MinWidth(0)
			ui.Spacer(c)
			if s.tab == 0 && s.view.session == nil {
				for _, ag := range env.Agents {
					if k := countAgent(g.sessions, ag); k > 0 {
						agentCount(c, ag, k)
					}
				}
			}
			muted(c, fmt.Sprintf("%d 个会话", len(g.sessions))).FontSize(12).SingleLine().Shrink(0)
		})
		if head.Clicked() {
			s.expanded[g.dir] = !s.expanded[g.dir]
		}
	})
}

// agentTabs chooses the agent whose sessions show: every agent, or one,
// each with its mark and its count.
func (a *app) agentTabs(c *ui.Context) {
	s := &a.sessions
	t := c.Theme()
	seg := ui.SegmentedBase(c, &s.tab, len(env.Agents)+1)
	seg.Track.Padding(3).Gap(2).Radius(9).Background(t.Surface).Shrink(0).Label("Agent").Children(func() {
		for i := 0; i <= len(env.Agents); i++ {
			label, n := "全部", len(s.all)
			if i > 0 {
				label, n = env.Agents[i-1].Label(), s.count(env.Agents[i-1])
			}
			b := seg.Segment(i).Padding(4, 10).Radius(7).Gap(6).Label(label)
			if i == s.tab {
				b.Background(t.Background).Shadow(0, 1, 2, 0, ui.RGBA(0, 0, 0, 0.1))
			}
			b.Children(func() {
				if i > 0 {
					agentMark(c, env.Agents[i-1], 16, true)
				}
				ui.Text(c, label).SingleLine()
				muted(c, strconv.Itoa(n)).FontSize(12).SingleLine()
			})
		}
	})
}

// agentCount is an agent's logo, then a count on the agent's color, both
// as high as the count's text.
func agentCount(c *ui.Context, ag agentlog.Agent, n int) {
	const h = 16
	ui.Row(c).Gap(4).Shrink(0).Children(func() {
		agentMark(c, ag, h, true)
		ui.Row(c).Height(h).Padding(0, 5).Radius(4).Background(agentColors[ag].Alpha(0.12)).Children(func() {
			ui.Text(c, strconv.Itoa(n)).FontSize(11).Bold().TextColor(agentColors[ag])
		})
	})
}

func countAgent(list []agentlog.Session, ag agentlog.Agent) int {
	n := 0
	for _, x := range list {
		if x.Agent == ag {
			n++
		}
	}
	return n
}

func (a *app) sessionRow(c *ui.Context, x *agentlog.Session) {
	s := &a.sessions
	t := c.Theme()
	now := time.Now()
	_, checked := s.selected[x.Path]
	current := s.view.session != nil && s.view.session.Path == x.Path
	r := ui.Row(c).Padding(8, 14, 8, 14).Gap(10).Radius(8)
	if current {
		r.Background(t.Accent.Alpha(0.08))
	} else if r.Hovered() {
		r.Background(ui.RGBA(0, 0, 0, 0.03))
	}
	r.Children(func() {
		v := checked
		if ui.Checkbox(c.Key("cb"), &v, "").Changed() {
			if v {
				s.selected[x.Path] = *x
			} else {
				delete(s.selected, x.Path)
			}
		}
		// Under the project's name: as wide as the project row's arrow.
		ui.Box(c).Width(13).Shrink(0)
		body := ui.Column(c).Grow(1).MinWidth(0).Gap(2).Cursor(ui.CursorPointer).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				if s.tab == 0 {
					agentMark(c, x.Agent, 16, true)
				}
				title := x.Title
				if title == "" {
					title = "（无标题）"
				}
				ui.Text(c, title).SingleLine().Shrink(1)
			})
			meta := []string{ago(x.Updated, now)}
			if len(x.Models) > 0 {
				meta = append(meta, strings.Join(x.Models, ", "))
			}
			meta = append(meta, fmt.Sprintf("%d 条消息", x.Messages), humanSize(x.Size), shortID(x.ID))
			if x.Archived {
				meta = append(meta, "已归档")
			}
			muted(c, strings.Join(meta, " · ")).FontSize(12).SingleLine()
		})
		if body.Clicked() {
			a.openSession(*x)
		}
		if iconButton(c, iconDownload, "导出 MD").Clicked() {
			a.exportMarkdown([]agentlog.Session{*x})
		}
		if iconButton(c, iconTrash, "删除").Clicked() {
			s.toDelete = []agentlog.Session{*x}
			s.confirmDelete = true
		}
	})
}

// selectionBar shows what is selected and what can be done with it.
func (a *app) selectionBar(c *ui.Context) {
	s := &a.sessions
	t := c.Theme()
	if len(s.selected) == 0 {
		return
	}
	ui.Row(c).Padding(10, 16, 10, 20).Gap(10).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Background(t.Background).Children(func() {
		var parts []string
		for _, ag := range env.Agents {
			n := 0
			for _, x := range s.selected {
				if x.Agent == ag {
					n++
				}
			}
			if n > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", ag.Label(), n))
			}
		}
		ui.Text(c, fmt.Sprintf("已选 %d 个", len(s.selected))).Bold()
		muted(c, strings.Join(parts, " · ")).FontSize(12)
		if ui.Button(c, "清空").Clicked() {
			clear(s.selected)
		}
		ui.Spacer(c)
		sel := a.selectedSessions()
		if ui.Button(c, "").Children(func() {
			ui.Icon(c, iconDownload).FontSize(13)
			ui.Text(c, "导出 MD")
		}).Clicked() {
			a.exportMarkdown(sel)
		}
		if ui.Button(c, "").Children(func() {
			ui.Icon(c, iconTrash).FontSize(13)
			ui.Text(c, "删除")
		}).Clicked() {
			s.toDelete = sel
			s.confirmDelete = true
		}
		if ui.PrimaryButton(c, "").Children(func() {
			ui.Icon(c, iconSparkles).FontSize(13)
			ui.Text(c, "AI 提炼")
		}).Clicked() {
			a.openDistill(sel)
		}
	})
}

// selectedSessions returns the selection, newest first.
func (a *app) selectedSessions() []agentlog.Session {
	var list []agentlog.Session
	for _, x := range a.sessions.selected {
		list = append(list, x)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Updated.After(list[j].Updated) })
	return list
}
