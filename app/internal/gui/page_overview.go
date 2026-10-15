package gui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
)

// heatWeeks is how many weeks the activity map shows at most.
const heatWeeks = 53

// overviewState is what the overview page remembers between frames.
type overviewState struct {
	period int    // the projects card's: 0 today, 1 this week, 2 all time
	hover  string // the day under the pointer in the activity map
}

// stats sums up the sessions for the overview.
type stats struct {
	today    time.Time // midnight, local
	total    int
	messages int
	size     int64
	week     map[agentlog.Agent]int // sessions of the last 7 days, by agent
	weekAll  int
	daily    map[time.Time]int // sessions by the local day they were last active
	maxDaily int               // the most sessions of a day in the map's span
}

func dayOf(t time.Time) time.Time {
	y, m, d := t.Local().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

func sumStats(all []agentlog.Session, now time.Time) stats {
	s := stats{today: dayOf(now), week: map[agentlog.Agent]int{}, daily: map[time.Time]int{}}
	weekStart := s.today.AddDate(0, 0, -6)
	heatStart := s.today.AddDate(0, 0, -7*heatWeeks)
	for _, x := range all {
		s.total++
		s.messages += x.Messages
		s.size += x.Size
		d := dayOf(x.Updated)
		if !d.Before(weekStart) {
			s.week[x.Agent]++
			s.weekAll++
		}
		s.daily[d]++
		if !d.Before(heatStart) {
			s.maxDaily = max(s.maxDaily, s.daily[d])
		}
	}
	return s
}

// heatLevel is the shade, 0 to 4, of a day of n sessions.
func heatLevel(n, most int) int {
	if n <= 0 || most <= 0 {
		return 0
	}
	return min(4, (4*n+most-1)/most)
}

// projects counts the projects active since a day (any day when zero), and
// finds the one with the most sessions.
func projects(all []agentlog.Session, since time.Time) (n int, top string, topN int) {
	byDir := map[string]int{}
	for _, x := range all {
		if since.IsZero() || !x.Updated.Before(since) {
			byDir[x.CWD]++
		}
	}
	for dir, k := range byDir {
		if k > topN || (k == topN && dir < top) {
			top, topN = dir, k
		}
	}
	return len(byDir), top, topN
}

func (a *app) overviewPage(c *ui.Context) {
	if !a.library.loaded {
		a.library.refresh(a)
	}
	st := sumStats(a.sessions.all, time.Now())
	header(c, "概览", func() {
		if a.sessions.scanning {
			ui.Spinner(c).Size(14, 14)
		}
	})
	ui.Scroll(c).Grow(1).Padding(4, 28, 28).Gap(18).Children(func() {
		ui.Grid(c).Columns(4).Gap(18).Children(func() {
			a.sessionsCard(c, st)
			a.agentsCard(c)
			a.messagesCard(c, st)
			a.libraryCard(c)
		})
		ui.Grid(c).Columns(2).Gap(18).Children(func() {
			a.weekCard(c, st)
			a.projectsCard(c, st)
		})
		a.activityCard(c, st)
	})
}

// statCard is a card of the first row: a caption, then what fill draws,
// then a line at the bottom.
func statCard(c *ui.Context, title string, fill func(), bottom func()) ui.Element {
	return card(c).Height(156).Children(func() {
		caption(c, title)
		fill()
		ui.Spacer(c)
		if bottom != nil {
			bottom()
		}
	})
}

func (a *app) sessionsCard(c *ui.Context, st stats) {
	t := c.Theme()
	// The last 30 days, oldest first.
	days := make([]int, 30)
	for i := range days {
		days[i] = st.daily[st.today.AddDate(0, 0, i-29)]
	}
	card(c).Height(156).Padding(0).Clip().Children(func() {
		ui.Column(c).Padding(18, 20, 0).Gap(6).Children(func() {
			caption(c, "会话")
			figure(c, strconv.Itoa(st.total), "个", 34)
		})
		ui.Box(c).Grow(1).Draw(func(p *ui.Painter, r ui.Rect) {
			sparkline(p, r, days, t.Accent)
		})
	}).Tooltip("近 30 天每天活跃的会话")
}

// sparkline draws counts as a line along the bottom of r, over a faint
// fill.
func sparkline(p *ui.Painter, r ui.Rect, counts []int, color ui.Color) {
	most := 1
	for _, n := range counts {
		most = max(most, n)
	}
	top, bottom := r.Y+6, r.Y+r.H-3
	x := func(i int) float32 { return r.X + r.W*float32(i)/float32(len(counts)-1) }
	y := func(n int) float32 { return bottom - (bottom-top)*float32(n)/float32(most) }
	var line, area ui.Path
	line.MoveTo(x(0), y(counts[0]))
	area.MoveTo(x(0), r.Y+r.H).LineTo(x(0), y(counts[0]))
	for i := 1; i < len(counts); i++ {
		line.LineTo(x(i), y(counts[i]))
		area.LineTo(x(i), y(counts[i]))
	}
	area.LineTo(x(len(counts)-1), r.Y+r.H).Close()
	p.FillPathGradient(&area, ui.LinearGradient{From: color.Alpha(0.16), To: color.Alpha(0.02), Angle: 180})
	p.StrokePath(&line, 2, color)
}

func (a *app) agentsCard(c *ui.Context) {
	t := c.Theme()
	p := colors(t)
	on := 0
	// The tiles fit the card, a quarter of the page less the gaps and the
	// card's padding.
	w, _ := c.Size()
	cardW := (w - sidebarWidth - 56 - 3*18) / 4
	n := float32(len(env.Agents))
	tileW := max(20, min(44, (cardW-40-8*(n-1))/n))
	b := statCard(c, "AGENT", func() {
		ui.Row(c).Gap(8).MarginY(6).Children(func() {
			for _, ag := range env.Agents {
				enabled := a.infos[ag].Installed() && a.cfg.Agents[ag].Enabled
				if enabled {
					on++
				}
				tile := ui.Box(c.Key(ag)).Size(tileW, tileW).Radius(tileW * 0.27).Center().Tooltip(ag.Label())
				if enabled {
					tile.Background(agentColors[ag].Alpha(0.12))
				} else {
					tile.Background(p.subtle)
				}
				tile.Children(func() { agentMark(c, ag, tileW*0.55, enabled) })
			}
		})
	}, func() {
		ui.Text(c, fmt.Sprintf("%d 个已启用", on)).FontWeight(600)
	})
	b.Cursor(ui.CursorPointer)
	if b.Clicked() {
		a.page = pageAgents
	}
}

func (a *app) messagesCard(c *ui.Context, st stats) {
	statCard(c, "消息", func() {
		figure(c, compact(st.messages), "条", 34)
	}, func() {
		muted(c, "会话记录共 "+humanSize(st.size)).SingleLine()
	})
}

// compact writes a large count short: 1234 is 1,234, 123456 is 12.3 万.
func compact(n int) string {
	switch {
	case n >= 100000:
		return strconv.FormatFloat(float64(n)/10000, 'f', 1, 64) + " 万"
	case n >= 1000:
		return fmt.Sprintf("%d,%03d", n/1000, n%1000)
	}
	return strconv.Itoa(n)
}

func (a *app) libraryCard(c *ui.Context) {
	l := &a.library
	b := statCard(c, "资源库", func() {
		figure(c, strconv.Itoa(len(l.items)), "项", 34)
	}, func() {
		var parts []string
		for _, k := range library.Kinds {
			n := 0
			for _, it := range l.items {
				if it.Kind == k {
					n++
				}
			}
			if n > 0 {
				parts = append(parts, fmt.Sprintf("%s %d", k.Label(), n))
			}
		}
		line := "还没有提炼的资源"
		if len(parts) > 0 {
			line = strings.Join(parts, " · ")
		}
		muted(c, line).SingleLine()
	})
	b.Cursor(ui.CursorPointer)
	if b.Clicked() {
		a.page = pageLibrary
	}
}

func (a *app) weekCard(c *ui.Context, st stats) {
	t := c.Theme()
	card(c).Height(190).Children(func() {
		ui.Row(c).Children(func() {
			caption(c, "近 7 天")
			ui.Spacer(c)
			if st.daily[st.today] > 0 {
				ui.Box(c).Size(10, 10).Radius(5).Background(t.Accent).Tooltip("今天有活跃的会话")
			}
		})
		figure(c, strconv.Itoa(st.weekAll), "个会话", 40)
		ui.Spacer(c)
		ui.Row(c).Gap(18).Children(func() {
			for i, ag := range env.Agents {
				if i > 0 {
					ui.Box(c).Width(1).Height(40).Background(t.Border)
				}
				ui.Column(c).Gap(2).Children(func() {
					ui.Text(c, strconv.Itoa(st.week[ag])).FontSize(20).FontWeight(800)
					ui.Row(c).Gap(5).Children(func() {
						agentMark(c, ag, 13, true)
						caption(c, strings.ToUpper(ag.Label()))
					})
				})
			}
		})
	})
}

func (a *app) projectsCard(c *ui.Context, st stats) {
	o := &a.overview
	card(c).Height(190).Children(func() {
		ui.Row(c).Children(func() {
			caption(c, "活跃项目")
			ui.Spacer(c)
			textSegments(c, &o.period, "时间范围", "今天", "本周", "全部")
		})
		var since time.Time
		what := "今天"
		switch o.period {
		case 0:
			since = st.today
		case 1:
			since, what = st.today.AddDate(0, 0, -6), "近 7 天"
		default:
			what = "全部会话中"
		}
		n, top, topN := projects(a.sessions.all, since)
		figure(c, strconv.Itoa(n), "个", 40)
		ui.Spacer(c)
		line := what + "没有活跃的项目。"
		if n > 0 {
			name := filepath.Base(top)
			if top == "" {
				name = "（未知目录）"
			}
			line = fmt.Sprintf("%s最活跃：%s，%d 个会话。", what, name, topN)
		}
		muted(c, line).SingleLine()
	})
}

// heatLayout is where the activity map's cells go in a box of width w.
type heatLayout struct {
	weeks      int
	left, top  float32 // the first cell's corner, from the box's
	cell, step float32
}

const heatLabelW, heatMonthH = 34, 20

// newHeatLayout fills the width with up to a year of weeks, making the
// cells larger in a wide window and dropping the oldest weeks in a narrow
// one.
func newHeatLayout(w float32) heatLayout {
	step := min(18, max(13, (w-heatLabelW)/heatWeeks))
	weeks := max(1, min(heatWeeks, int((w-heatLabelW)/step)))
	return heatLayout{weeks: weeks, left: w - float32(weeks)*step, top: heatMonthH, cell: step - 3, step: step}
}

func (h heatLayout) height() float32 { return h.top + 7*h.step }

// day returns the day of the cell in column col and row row (Sunday 0),
// the last column being this week.
func (h heatLayout) day(today time.Time, col, row int) time.Time {
	sunday := today.AddDate(0, 0, -int(today.Weekday()))
	return sunday.AddDate(0, 0, 7*(col-h.weeks+1)+row)
}

func (a *app) activityCard(c *ui.Context, st stats) {
	t := c.Theme()
	p := colors(t)
	o := &a.overview
	card(c).Gap(12).Children(func() {
		ui.Row(c).Children(func() {
			caption(c, "活动")
			ui.Spacer(c)
			if o.hover != "" {
				muted(c, o.hover).FontSize(12)
			}
		})
		// Sized by the width it had in the previous frame.
		mapBox := ui.Box(c)
		mapBox.Height(newHeatLayout(mapBox.LayoutBounds().W).height())
		o.hover = ""
		if x, y, over := mapBox.PointerPosition(); over {
			h := newHeatLayout(mapBox.LayoutBounds().W)
			col, row := int((x-h.left)/h.step), int((y-h.top)/h.step)
			if x >= h.left && y >= h.top && col < h.weeks && row < 7 {
				if d := h.day(st.today, col, row); !d.After(st.today) {
					o.hover = fmt.Sprintf("%d年%d月%d日 · %d 个会话", d.Year(), d.Month(), d.Day(), st.daily[d])
				}
			}
		}
		mapBox.Draw(func(pt *ui.Painter, r ui.Rect) {
			h := newHeatLayout(r.W)
			for col := range h.weeks {
				x := r.X + h.left + float32(col)*h.step
				// A month's name over its first full week.
				if sunday := h.day(st.today, col, 0); sunday.Day() <= 7 {
					pt.Text(x, r.Y, fmt.Sprintf("%d月", sunday.Month()), 11, t.TextMuted)
				}
				for row := range 7 {
					d := h.day(st.today, col, row)
					if d.After(st.today) {
						continue
					}
					y := r.Y + h.top + float32(row)*h.step
					pt.Fill(ui.Rect{X: x, Y: y, W: h.cell, H: h.cell}, p.heat[heatLevel(st.daily[d], st.maxDaily)], 3)
				}
			}
			for row, name := range map[int]string{1: "一", 3: "三", 5: "五"} {
				pt.Text(r.X, r.Y+h.top+float32(row)*h.step+(h.cell-14)/2, "周"+name, 11, t.TextMuted)
			}
		})
		ui.Row(c).Gap(4).Justify(ui.End).Children(func() {
			muted(c, "少").FontSize(11).MarginX(4)
			for _, color := range p.heat {
				ui.Box(c).Size(11, 11).Radius(3).Background(color)
			}
			muted(c, "多").FontSize(11).MarginX(4)
		})
	})
}
