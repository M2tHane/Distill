package gui

import (
	"embed"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
)

// paneHeader is the height of the rows atop the panes beside each other,
// so that they line up.
const paneHeader = 46

// icon makes a Lucide-style icon of SVG path elements.
func icon(paths string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`))
}

var (
	iconDashboard = icon(`<rect x="3" y="3" width="7" height="9" rx="1.5"/><rect x="14" y="3" width="7" height="5" rx="1.5"/><rect x="14" y="12" width="7" height="9" rx="1.5"/><rect x="3" y="16" width="7" height="5" rx="1.5"/>`)
	iconTerminal  = icon(`<path d="m4 17 6-6-6-6"/><path d="M12 19h8"/>`)
	iconChat      = icon(`<path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/>`)
	iconLibrary   = icon(`<path d="M4 19.5v-15A2.5 2.5 0 0 1 6.5 2H20v20H6.5a2.5 2.5 0 0 1 0-5H20"/>`)
	iconTrash     = icon(`<path d="M3 6h18"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	iconDownload  = icon(`<path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/><path d="M12 15V3"/>`)
	iconX         = icon(`<path d="M18 6 6 18"/><path d="m6 6 12 12"/>`)
	iconChevron   = icon(`<path d="m9 18 6-6-6-6"/>`)
	iconSparkles  = icon(`<path d="M12 3l1.9 5.1L19 10l-5.1 1.9L12 17l-1.9-5.1L5 10l5.1-1.9z"/><path d="M19 3v4M17 5h4"/>`)
	iconRefresh   = icon(`<path d="M21 12a9 9 0 1 1-3-6.7L21 8"/><path d="M21 3v5h-5"/>`)
	iconFile      = icon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M8 13h8M8 17h5"/>`)
	iconRestore   = icon(`<path d="M3 12a9 9 0 1 0 3-6.7L3 8"/><path d="M3 3v5h5"/>`)
	iconFolder    = icon(`<path d="M3 7a2 2 0 0 1 2-2h4l2 2h8a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/>`)
	iconExpand    = icon(`<path d="M15 3h6v6M9 21H3v-6M21 3l-7 7M3 21l7-7"/>`)
	iconWrench    = icon(`<path d="M14.7 6.3a4 4 0 0 0 5 5L21 13l-8 8-3-3 8-8z" transform="translate(-2 0)"/><path d="M3 21l6-6"/>`)
	iconSend      = icon(`<path d="M22 2 11 13"/><path d="M22 2 15 22l-4-9-9-4z"/>`)
	iconStop      = icon(`<rect x="6" y="6" width="12" height="12" rx="2"/>`)
	iconCopy      = icon(`<rect x="9" y="9" width="12" height="12" rx="2"/><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"/>`)
	iconReport    = icon(`<path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"/><path d="M14 2v6h6"/><path d="M8 18v-3M12 18v-6M16 18v-4"/>`)
	iconSkill     = icon(`<path d="M12 2 2 7l10 5 10-5z"/><path d="m2 17 10 5 10-5"/><path d="m2 12 10 5 10-5"/>`)
)

// Colors of the agents, for counts and borders beside their logos.
var agentColors = map[agentlog.Agent]ui.Color{
	agentlog.Claude: ui.Hex("#D97757"),
	agentlog.Codex:  ui.Hex("#5B6CF0"),
	agentlog.Pi:     ui.Hex("#18181B"),
}

//go:embed icons/*.svg
var iconFiles embed.FS

// agentLogos are the agents' logos, from yetone/magpie (see icons/README.md).
var agentLogos = func() map[agentlog.Agent]*ui.SVG {
	logos := map[agentlog.Agent]*ui.SVG{}
	for _, a := range []agentlog.Agent{agentlog.Claude, agentlog.Codex, agentlog.Pi} {
		data, err := iconFiles.ReadFile("icons/" + string(a) + ".svg")
		if err == nil {
			logos[a] = ui.MustParseSVG(data)
		}
	}
	return logos
}()

// agentMark draws an agent's logo, grayed when off. An agent without a logo
// shows its initial.
func agentMark(c *ui.Context, a agentlog.Agent, size float32, on bool) ui.Element {
	t := c.Theme()
	logo := agentLogos[a]
	if logo == nil {
		return ui.Box(c).Size(size, size).Radius(size * 0.28).Background(t.TextMuted).Center().Children(func() {
			ui.Text(c, strings.ToUpper(string(a)[:1])).FontSize(size * 0.6).Bold().TextColor(ui.Hex("#FFFFFF"))
		})
	}
	img := ui.Image(c, logo).Size(size, size).TextColor(t.Text)
	if !on {
		img.Grayscale().Opacity(0.35)
	}
	return img
}

// iconButton is a borderless button showing an icon, which a hover shades.
func iconButton(c *ui.Context, svg *ui.SVG, label string) ui.Element {
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Padding(6).Radius(6).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(15).TextColor(t.TextMuted) })
	return b
}

// chip is a small rounded label.
func chip(c *ui.Context, text string, fg, bg ui.Color) ui.Element {
	return ui.Box(c).Padding(1, 6).Radius(4).Background(bg).Children(func() {
		ui.Text(c, text).FontSize(11).TextColor(fg).SingleLine()
	})
}

func muted(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).TextColor(c.Theme().TextMuted)
}

// ago is a date as list rows show it: 刚刚, 5 分钟前, 3 小时前, 昨天, 10月3日.
func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return fmt.Sprintf("%d 分钟前", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d 小时前", int(d.Hours()))
	}
	lt, ln := t.Local(), now.Local()
	y, m, dd := ln.Date()
	yesterday := time.Date(y, m, dd-1, 0, 0, 0, 0, time.Local)
	if !lt.Before(yesterday) {
		return "昨天"
	}
	if lt.Year() != ln.Year() {
		return fmt.Sprintf("%d年%d月%d日", lt.Year(), lt.Month(), lt.Day())
	}
	return fmt.Sprintf("%d月%d日", lt.Month(), lt.Day())
}

func humanSize(n int64) string {
	switch {
	case n < 1<<20:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	}
}

func shortID(id string) string {
	id = strings.ReplaceAll(id, "-", "")
	if len(id) > 8 {
		// Time-ordered IDs (UUIDv7) share their start; their end tells them apart.
		if strings.HasPrefix(id, "01") || strings.HasPrefix(id, "019") {
			return id[len(id)-8:]
		}
		return id[:8]
	}
	return id
}

// fileName makes a title safe as a file name.
func fileName(s string) string {
	s = strings.Map(func(r rune) rune {
		switch r {
		case '/', '\\', ':', '*', '?', '"', '<', '>', '|', '\n', '\r', '\t':
			return ' '
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) > 60 {
		s = string([]rune(s)[:60])
	}
	if s == "" {
		s = "session"
	}
	return s
}

var iconSearch = icon(`<circle cx="11" cy="11" r="7"/><path d="m20 20-3.5-3.5"/>`)

// searchBox is a search field with a placeholder of its own, which the
// toolkit's field does not take; Escape clears it.
func searchBox(c *ui.Context, query *string, placeholder string) ui.Element {
	t := c.Theme()
	box := ui.Row(c).Gap(6).Padding(0, 4, 0, 10).Height(32).Radius(10).Background(colors(t).card).Border(1, t.Border)
	box.Children(func() {
		ui.Icon(c, iconSearch).FontSize(13).TextColor(t.TextMuted)
		in := ui.TextInput(c.Key("q"), query).Placeholder(placeholder).Label(placeholder).Grow(1).
			Background(ui.Transparent).Border(0, ui.Transparent).Padding(0)
		if *query != "" && (iconButton(c, iconX, "清除").Padding(3).Clicked() || (in.Focused() && c.Shortcut(0, ui.KeyEscape))) {
			*query = ""
		}
	})
	return box
}

// scrollSelect is a drop-down of many options whose list scrolls instead of
// running off the window, width wide (0 fills its container). It reports
// a new choice, in the frame after the user made it: the select sets
// *value after the frame that built it, so value must outlive the frame,
// as a field of the app's state does and a local variable does not. leading, when not nil, draws what goes before an option, as an icon.
func scrollSelect(c *ui.Context, value *string, options []string, width float32, label string, leading func(string)) bool {
	t := c.Theme()
	sel := ui.SelectBase(c, value)
	trig := sel.Trigger.Height(30).Padding(0, 10).Gap(6).Radius(8).Background(t.Surface).Border(1, t.Border).Label(label)
	if trig.Hovered() {
		trig.Background(t.SurfaceHover)
	}
	if width > 0 {
		trig.Width(width)
	} else {
		trig.FillWidth()
		// The list is as wide as the trigger was.
		width = trig.LayoutBounds().W
	}
	trig.Children(func() {
		if leading != nil {
			leading(*value)
		}
		ui.Text(c, *value).SingleLine().Grow(1).MinWidth(0)
		ui.Icon(c, iconChevron).FontSize(12).Rotate(90).TextColor(t.TextMuted)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).MinWidth(width).MaxWidth(max(width, 360)).Padding(4).Radius(10).Background(t.Background).Border(1, t.Border).
			Shadow(0, 8, 24, 0, ui.RGBA(0, 0, 0, 0.15))
		ui.Scroll(c).MaxHeight(320).Gap(1).Children(func() {
			for _, o := range options {
				it := sel.Item(o).Padding(6, 10).Gap(8).Radius(6)
				if it.Highlighted() {
					it.Background(t.Accent).TextColor(t.AccentText)
				}
				it.Children(func() {
					if leading != nil {
						leading(o)
					}
					ui.Text(c, o).SingleLine()
				})
			}
		})
	})
	return trig.Changed()
}
