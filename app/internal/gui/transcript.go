package gui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
)

// viewState is the transcript shown beside the list.
type viewState struct {
	session    *agentlog.Session
	transcript *agentlog.Transcript
	loading    bool
	err        string
	split      float32 // the list's width
	toolsOpen  map[int]bool
	inputOpen  map[[2]int]bool
	list       ui.ListState
	gen        int // the load in flight, to drop stale ones
	// How the window was widened for the transcript, to undo on closing it:
	// how much wider, and how far left it moved to stay on the screen.
	grew, moved int
}

// transcriptWidth is the room a transcript gets beside the list.
const transcriptWidth = 640

// sidebarWidth is the width of the sidebar, beside the pages.
const sidebarWidth = 220

func (a *app) openSession(x agentlog.Session) {
	v := &a.sessions.view
	if v.session != nil && v.session.Path == x.Path {
		return
	}
	if v.session == nil {
		a.widen()
	}
	v.session = &x
	v.transcript, v.err, v.loading = nil, "", true
	v.toolsOpen, v.inputOpen = map[int]bool{}, map[[2]int]bool{}
	v.list = ui.ListState{}
	v.gen++
	gen := v.gen
	go func() {
		t, err := agentlog.Load(a.srcs, x)
		a.win.Update(func() {
			if v.gen != gen {
				return
			}
			v.loading = false
			if err != nil {
				v.err = err.Error()
				return
			}
			v.transcript = t
		})
	}()
}

// widen makes the window wider for a transcript beside the list, which
// keeps its width, as far as the screen allows; the list gives up the rest.
// A window in full screen or maximized keeps its size.
func (a *app) widen() {
	v := &a.sessions.view
	v.grew, v.moved = 0, 0
	listW := float32(1180 - sidebarWidth)
	if a.win == nil {
		v.split = max(360, listW-transcriptWidth)
		return
	}
	cw, _ := a.win.ContentSize()
	listW = float32(cw - sidebarWidth)
	if !a.win.IsFullScreen() && !a.win.IsMaximized() {
		b := a.win.Bounds()
		w, x := b.Width+transcriptWidth, b.X
		if wa := mygo.Screen.DisplayMatching(b).WorkArea; wa.Width > 0 {
			w = min(w, max(b.Width, wa.Width))
			x = max(wa.X, min(x, wa.X+wa.Width-w))
		}
		v.grew, v.moved = w-b.Width, b.X-x
		if v.grew > 0 {
			a.win.SetBounds(mygo.Rectangle{X: x, Y: b.Y, Width: w, Height: b.Height})
		}
	}
	v.split = max(360, listW+float32(v.grew)-transcriptWidth)
}

// closeSession closes the transcript, and narrows the window by as much
// as opening it widened it.
func (a *app) closeSession() {
	v := &a.sessions.view
	v.session, v.transcript, v.err = nil, nil, ""
	if v.grew > 0 && a.win != nil && !a.win.IsFullScreen() && !a.win.IsMaximized() {
		b := a.win.Bounds()
		w := max(b.Width-v.grew, minWindowWidth)
		a.win.SetBounds(mygo.Rectangle{X: b.X + v.moved, Y: b.Y, Width: w, Height: b.Height})
	}
	v.grew, v.moved = 0, 0
}

func (a *app) transcriptPane(c *ui.Context) {
	v := &a.sessions.view
	t := c.Theme()
	x := v.session
	ui.Row(c).Height(paneHeader).Padding(0, 12, 0, 18).Gap(8).Shrink(0).Children(func() {
		agentMark(c, x.Agent, 22, true)
		ui.Column(c).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, x.Title).Bold().SingleLine()
			meta := []string{x.Agent.Label(), filepath.Base(x.CWD), x.Updated.Local().Format("2006-01-02 15:04"), fmt.Sprintf("%d 条消息", x.Messages)}
			muted(c, strings.Join(meta, " · ")).FontSize(12).SingleLine()
		})
		if iconButton(c, iconDownload, "导出 MD").Clicked() {
			a.exportMarkdown([]agentlog.Session{*x})
		}
		if iconButton(c, iconSparkles, "AI 提炼这个会话").Clicked() {
			a.openDistill([]agentlog.Session{*x})
		}
		if iconButton(c, iconX, "关闭").Clicked() {
			a.closeSession()
		}
	})
	ui.Divider(c)
	switch {
	case v.loading:
		ui.Column(c).Grow(1).Center().Children(func() { ui.Spinner(c) })
	case v.err != "":
		ui.Text(c, v.err).TextColor(t.Danger).Padding(20)
	case v.transcript != nil:
		msgs := v.transcript.Messages
		v.list.Label = func(i int) string { return msgs[i].Text }
		ui.List(c, &v.list, len(msgs), func(i int) {
			bubble(c, msgs[i], i, v.toolsOpen, v.inputOpen, a.md)
		}).Grow(1).Padding(16, 18).Gap(12).Children(func() {
			if len(msgs) == 0 {
				muted(c, "这个会话没有对话内容").Padding(24).AlignSelf(ui.Center)
			}
		})
	}
}

// bubble draws a message: the user's on the right, the assistant's on the
// left with its tool calls folded under a line that opens them.
func bubble(c *ui.Context, m agentlog.Message, i int, toolsOpen map[int]bool, inputOpen map[[2]int]bool, md *mdView) {
	t := c.Theme()
	user := m.Role == agentlog.User
	ui.Row(c).FillWidth().MinWidth(0).Justify(map[bool]ui.Align{true: ui.End, false: ui.Start}[user]).Children(func() {
		b := ui.Column(c).MaxWidthPercent(66).MinWidth(0).Shrink(1).Padding(10, 14).Gap(8).Radius(14).Selectable()
		if user {
			b.Background(t.Accent.Alpha(0.12))
		} else {
			b.Background(colors(t).subtle)
		}
		b.Children(func() {
			if m.Text != "" {
				markdown(c, m.Text, md)
			}
			if len(m.Tools) > 0 {
				toolCalls(c, m.Tools, i, toolsOpen, inputOpen)
			}
			if m.Interrupted {
				muted(c, "已被用户中断").FontSize(12).Italic()
			}
		})
	})
}

func toolCalls(c *ui.Context, tools []agentlog.ToolCall, i int, toolsOpen map[int]bool, inputOpen map[[2]int]bool) {
	t := c.Theme()
	open := toolsOpen[i]
	ui.Column(c).Gap(4).Unselectable().Children(func() {
		head := ui.Row(c.Key("tools")).Gap(6).Cursor(ui.CursorPointer).Children(func() {
			rot := float32(0)
			if open {
				rot = 90
			}
			ui.Icon(c, iconChevron).FontSize(12).Rotate(rot).TextColor(t.TextMuted)
			ui.Icon(c, iconWrench).FontSize(12).TextColor(t.TextMuted)
			muted(c, fmt.Sprintf("%d 个工具调用  %s", len(tools), toolDigest(tools))).FontSize(12).SingleLine().Shrink(1)
			if n := countStatus(tools, agentlog.ToolFailed); n > 0 {
				ui.Text(c, fmt.Sprintf("%d 个失败", n)).FontSize(12).TextColor(t.Danger).SingleLine().Shrink(0)
			}
		})
		if head.Clicked() {
			toolsOpen[i] = !open
		}
		if !open {
			return
		}
		ui.Column(c).Gap(2).Padding(2, 0, 0, 18).Children(func() {
			for j, tc := range tools {
				k := [2]int{i, j}
				line := ui.Row(c.Key(j)).Gap(8).Cursor(ui.CursorPointer).Children(func() {
					ui.Text(c, tc.Name).FontSize(12).Bold().SingleLine().Shrink(0)
					muted(c, tc.Summary).Font("monospace").FontSize(11).SingleLine().Shrink(1)
					if label := statusLabel(tc.Status); label != "" {
						chip(c, label, t.Danger, t.Danger.Alpha(0.1)).Shrink(0)
					}
				})
				if line.Clicked() && (tc.Input != "" || tc.Output != "") {
					inputOpen[k] = !inputOpen[k]
				}
				if inputOpen[k] {
					ui.Column(c.Key(fmt.Sprint("in", j))).Gap(6).Padding(8).Radius(6).Background(t.Background).Border(1, t.Border).Selectable().Children(func() {
						if tc.Input != "" {
							ui.Text(c, tc.Input).Font("monospace").FontSize(11)
						}
						if tc.Output != "" {
							color := t.TextMuted
							if tc.Status != agentlog.ToolOK {
								color = t.Danger
							}
							ui.Text(c, tc.Output).Font("monospace").FontSize(11).TextColor(color)
						}
					})
				}
			}
		})
	})
}

// statusLabel names how a call ended, when it did not go well.
func statusLabel(s agentlog.ToolStatus) string {
	switch s {
	case agentlog.ToolFailed:
		return "失败"
	case agentlog.ToolRejected:
		return "已拒绝"
	case agentlog.ToolInterrupted:
		return "已中断"
	}
	return ""
}

func countStatus(tools []agentlog.ToolCall, s agentlog.ToolStatus) int {
	n := 0
	for _, tc := range tools {
		if tc.Status == s {
			n++
		}
	}
	return n
}

// toolDigest names the tools called, the most used first: "Bash ×3 · Read".
func toolDigest(tools []agentlog.ToolCall) string {
	counts := map[string]int{}
	var names []string
	for _, tc := range tools {
		if counts[tc.Name] == 0 {
			names = append(names, tc.Name)
		}
		counts[tc.Name]++
	}
	sort.SliceStable(names, func(i, j int) bool { return counts[names[i]] > counts[names[j]] })
	var parts []string
	for _, n := range names[:min(len(names), 4)] {
		if counts[n] > 1 {
			parts = append(parts, fmt.Sprintf("%s ×%d", n, counts[n]))
		} else {
			parts = append(parts, n)
		}
	}
	if len(names) > 4 {
		parts = append(parts, "…")
	}
	return strings.Join(parts, " · ")
}
