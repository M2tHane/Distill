package gui

import (
	"github.com/egoist/mygo/ui"
)

// palette is the look of the window: a light blue wash behind white cards.
type palette struct {
	washFrom, washTo ui.Color // the window's background, top left to bottom right
	card             ui.Color
	shadow           ui.Color
	subtle           ui.Color // fills inside a card: icon tiles, chips, project rows
	navActive        ui.Color // the sidebar's chosen item, and a segmented control's track
	navHover         ui.Color
	// heat colors the activity map, from no session to the most.
	heat [5]ui.Color
}

func colors(t *ui.Theme) palette {
	if t.Dark {
		return palette{
			washFrom:  ui.Hex("#16232D"),
			washTo:    ui.Hex("#121519"),
			card:      ui.Hex("#1D2228"),
			shadow:    ui.RGBA(0, 0, 0, 0.35),
			subtle:    ui.Hex("#272D34"),
			navActive: ui.RGBA(255, 255, 255, 0.09),
			navHover:  ui.RGBA(255, 255, 255, 0.05),
			heat:      [5]ui.Color{ui.Hex("#2A3038"), ui.Hex("#11506A"), ui.Hex("#13729A"), ui.Hex("#1796C9"), ui.Hex("#2BB8EE")},
		}
	}
	return palette{
		washFrom:  ui.Hex("#D9EEF8"),
		washTo:    ui.Hex("#F1F6F9"),
		card:      ui.Hex("#FFFFFF"),
		shadow:    ui.RGBA(30, 60, 90, 0.08),
		subtle:    ui.Hex("#F2F5F8"),
		navActive: ui.RGBA(30, 50, 70, 0.08),
		navHover:  ui.RGBA(30, 50, 70, 0.04),
		heat:      [5]ui.Color{ui.Hex("#EEF1F4"), ui.Hex("#BCE6F5"), ui.Hex("#86D2EE"), ui.Hex("#4ABCE4"), ui.Hex("#13A4D8")},
	}
}

// card is a white panel with round corners and a soft shadow.
func card(c *ui.Context) ui.Element {
	p := colors(c.Theme())
	return ui.Column(c).Padding(18, 20).Gap(6).Radius(16).Background(p.card).Shadow(0, 2, 10, 0, p.shadow)
}

// caption is a card's or a section's small title, spaced out.
func caption(c *ui.Context, s string) ui.Element {
	return ui.Text(c, s).FontSize(11).Bold().LetterSpacing(1.2).TextColor(c.Theme().TextMuted).SingleLine()
}

// figure is a large number, then its unit in small type on the same line.
func figure(c *ui.Context, value, unit string, size float32) ui.Element {
	spans := []ui.Span{{Text: value, Size: size, Weight: 800}}
	if unit != "" {
		spans = append(spans, ui.Span{Text: " " + unit, Size: max(12, size*0.38), Weight: 600})
	}
	return ui.RichText(c, spans...).SingleLine()
}

// segments is a segmented control drawn as a tinted track whose chosen
// segment is a raised white pill. labels name the segments; item draws
// segment i.
func segments(c *ui.Context, selected *int, label string, labels []string, item func(i int)) ui.Element {
	p := colors(c.Theme())
	seg := ui.SegmentedBase(c, selected, len(labels))
	seg.Track.Padding(3).Gap(2).Radius(10).Background(p.navActive).Shrink(0).Label(label).Children(func() {
		for i := range labels {
			b := seg.Segment(i).Padding(4, 10).Radius(8).Gap(6).Label(labels[i]).Cursor(ui.CursorPointer)
			if i == *selected {
				b.Background(p.card).Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.12))
			} else if b.Hovered() {
				b.Background(p.navHover)
			}
			b.Children(func() { item(i) })
		}
	})
	return seg.Track
}

// textSegments is segments of plain labels.
func textSegments(c *ui.Context, selected *int, label string, labels ...string) ui.Element {
	return segments(c, selected, label, labels, func(i int) {
		ui.Text(c, labels[i]).FontSize(13).SingleLine()
	})
}
