package gui

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

// Markdown is drawn with native elements: headings, paragraphs of styled
// spans, lists, quotes, code blocks and tables. Font sizes stay between
// mdMin and mdMax whatever the heading level.
const (
	mdMin  = 11
	mdBody = 13
	mdCode = 12
	mdMax  = 16
	// mdLong is the length past which a text shows its start until expanded.
	mdLong = 20000
)

// mdSize is the font size of a heading level, within bounds.
func mdSize(level int) float32 {
	return min(mdMax, max(mdMin, float32(mdMax+1-level)))
}

var mdParser = goldmark.New(goldmark.WithExtensions(extension.GFM)).Parser()

// mdDoc is a parsed text. Views are rebuilt every frame, so a text is parsed
// once and kept by its hash.
type mdDoc struct {
	src  []byte
	root ast.Node
	cut  bool // only the start of a long text was parsed
}

// mdView keeps the parsed texts, and which long ones show whole.
type mdView struct {
	docs map[[2]uint64]*mdDoc
	full map[uint64]bool
}

func newMDView() *mdView {
	return &mdView{docs: map[[2]uint64]*mdDoc{}, full: map[uint64]bool{}}
}

func mdHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

func (v *mdView) doc(s string, h uint64) *mdDoc {
	full := v.full[h] || len(s) <= mdLong
	key := [2]uint64{h, map[bool]uint64{true: 1}[full]}
	if d := v.docs[key]; d != nil {
		return d
	}
	if len(v.docs) > 4000 {
		clear(v.docs)
	}
	d := &mdDoc{}
	if !full {
		cut := mdLong
		if i := strings.LastIndexByte(s[:cut], '\n'); i > mdLong/2 {
			cut = i
		}
		s, d.cut = s[:cut], true
	}
	d.src = []byte(s)
	d.root = mdParser.Parse(text.NewReader(d.src))
	v.docs[key] = d
	return d
}

// markdown draws a text as Markdown, and the rest of a long text on demand.
func markdown(c *ui.Context, s string, v *mdView) {
	t := c.Theme()
	h := mdHash(s)
	ui.Column(c.Key(h)).Gap(8).MinWidth(0).Children(func() {
		d := v.doc(s, h)
		r := mdRenderer{c: c, src: d.src, t: t}
		r.blocks(d.root)
		if d.cut {
			if ui.Button(c, fmt.Sprintf("展开全部（共 %s）", humanSize(int64(len(s))))).FontSize(12).Clicked() {
				v.full[h] = true
			}
		}
	})
}

type mdRenderer struct {
	c   *ui.Context
	src []byte
	t   *ui.Theme
}

// blocks draws the block children of n, one under the other.
func (r mdRenderer) blocks(n ast.Node) {
	for b := n.FirstChild(); b != nil; b = b.NextSibling() {
		r.block(b)
	}
}

func (r mdRenderer) block(n ast.Node) {
	c, t := r.c, r.t
	switch n := n.(type) {
	case *ast.Heading:
		r.paragraph(n, ui.Span{Size: mdSize(n.Level), Weight: 700}).MarginY(2)
	case *ast.Paragraph, *ast.TextBlock:
		r.paragraph(n, ui.Span{})
	case *ast.ThematicBreak:
		ui.Divider(c).MarginY(4)
	case *ast.Blockquote:
		ui.Row(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
			ui.Box(c).Width(3).Radius(2).Background(t.Border).Shrink(0)
			ui.Column(c).Gap(6).Grow(1).MinWidth(0).TextColor(t.TextMuted).Children(func() { r.blocks(n) })
		})
	case *ast.List:
		r.list(n)
	case *ast.FencedCodeBlock, *ast.CodeBlock:
		r.code(n)
	case *ast.HTMLBlock:
		ui.Text(c, r.lines(n)).Font("monospace").FontSize(mdCode).TextColor(t.TextMuted).MinWidth(0)
	case *east.Table:
		r.table(n)
	default:
		r.blocks(n)
	}
}

func (r mdRenderer) lines(n ast.Node) string {
	var b strings.Builder
	l := n.Lines()
	for i := 0; i < l.Len(); i++ {
		seg := l.At(i)
		b.Write(seg.Value(r.src))
	}
	return strings.TrimRight(b.String(), "\n")
}

func (r mdRenderer) code(n ast.Node) {
	c, t := r.c, r.t
	src := r.lines(n)
	lang := ""
	if f, ok := n.(*ast.FencedCodeBlock); ok && f.Info != nil {
		lang = strings.Fields(string(f.Info.Value(r.src)) + " ")[0]
	}
	ui.Column(c).Radius(6).Background(t.Background).Border(1, t.Border).Clip().MinWidth(0).Children(func() {
		ui.Row(c).Padding(2, 4, 0, 10).Unselectable().Children(func() {
			ui.Text(c, lang).FontSize(mdMin).TextColor(t.TextMuted)
			ui.Spacer(c)
			if iconButton(c, iconCopy, "复制代码").Padding(3).Clicked() {
				c.WriteClipboard(src)
				c.Toast("已复制")
			}
		})
		ui.ScrollHorizontal(c).Padding(0, 10, 8).Children(func() {
			ui.Text(c, src).Font("monospace").FontSize(mdCode).NoWrap()
		})
	})
}

func (r mdRenderer) list(n *ast.List) {
	c, t := r.c, r.t
	ui.Column(c).Gap(4).MinWidth(0).Children(func() {
		i := n.Start
		for item := n.FirstChild(); item != nil; item = item.NextSibling() {
			marker := "•"
			if n.IsOrdered() {
				marker = strconv.Itoa(i) + "."
				i++
			}
			// A task's check box replaces its marker.
			if p := item.FirstChild(); p != nil {
				if cb, ok := p.FirstChild().(*east.TaskCheckBox); ok {
					marker = map[bool]string{true: "☑", false: "☐"}[cb.IsChecked]
				}
			}
			ui.Row(c).Gap(6).AlignItems(ui.Start).Children(func() {
				ui.Text(c, marker).FontSize(mdBody).TextColor(t.TextMuted).MinWidth(14).Shrink(0).Unselectable()
				ui.Column(c).Gap(4).Grow(1).MinWidth(0).Children(func() { r.blocks(item) })
			})
		}
	})
}

func (r mdRenderer) table(n *east.Table) {
	c, t := r.c, r.t
	cols := 0
	for row := n.FirstChild(); row != nil; row = row.NextSibling() {
		cols = max(cols, row.ChildCount())
	}
	if cols == 0 {
		return
	}
	ui.ScrollHorizontal(c).MinWidth(0).Children(func() {
		ui.Grid(c).Columns(cols).Border(1, t.Border).Radius(6).Clip().Children(func() {
			for row := n.FirstChild(); row != nil; row = row.NextSibling() {
				_, head := row.(*east.TableHeader)
				for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
					box := ui.Box(c).Padding(4, 8).MinWidth(60).BorderWidth(0, 1, 1, 0).BorderColor(t.Border)
					style := ui.Span{}
					if head {
						box.Background(t.Surface)
						style.Weight = 700
					}
					box.Children(func() { r.paragraph(cell, style) })
				}
			}
		})
	})
}

// paragraph draws the inline children of n as one paragraph of spans in
// the given style.
func (r mdRenderer) paragraph(n ast.Node, style ui.Span) ui.Element {
	var spans []ui.Span
	r.inline(n, style, &spans)
	if len(spans) == 0 {
		return ui.Box(r.c)
	}
	size := style.Size
	if size == 0 {
		size = mdBody
	}
	return ui.RichText(r.c, spans...).FontSize(size).LineHeight(1.45).MinWidth(0)
}

func (r mdRenderer) inline(n ast.Node, style ui.Span, spans *[]ui.Span) {
	add := func(s string, st ui.Span) {
		if s == "" {
			return
		}
		st.Text = s
		*spans = append(*spans, st)
	}
	for x := n.FirstChild(); x != nil; x = x.NextSibling() {
		switch x := x.(type) {
		case *ast.Text:
			add(string(x.Segment.Value(r.src)), style)
			if x.HardLineBreak() || x.SoftLineBreak() {
				// Line breaks stay, as the author typed them.
				add("\n", style)
			}
		case *ast.String:
			add(string(x.Value), style)
		case *ast.CodeSpan:
			st := style
			st.Font, st.Size, st.Background = "monospace", mdCode, r.t.Surface
			var b strings.Builder
			for t := x.FirstChild(); t != nil; t = t.NextSibling() {
				if tx, ok := t.(*ast.Text); ok {
					b.Write(tx.Segment.Value(r.src))
				}
			}
			add(b.String(), st)
		case *ast.Emphasis:
			st := style
			if x.Level >= 2 {
				st.Weight = 700
			} else {
				st.Italic = true
			}
			r.inline(x, st, spans)
		case *east.Strikethrough:
			st := style
			st.Strikethrough = true
			r.inline(x, st, spans)
		case *ast.Link:
			st := style
			st.Color, st.Underline = r.t.Accent, true
			r.inline(x, st, spans)
		case *ast.AutoLink:
			st := style
			st.Color, st.Underline = r.t.Accent, true
			add(string(x.Label(r.src)), st)
		case *ast.Image:
			st := style
			st.Color = r.t.TextMuted
			var alt []ui.Span
			r.inline(x, st, &alt)
			label := "[图片]"
			if len(alt) > 0 {
				label = "[图片：" + alt[0].Text + "]"
			}
			add(label, st)
		case *ast.RawHTML:
			st := style
			st.Font, st.Size, st.Color = "monospace", mdCode, r.t.TextMuted
			l := x.Segments
			for i := 0; i < l.Len(); i++ {
				seg := l.At(i)
				add(string(seg.Value(r.src)), st)
			}
		case *east.TaskCheckBox:
			// Drawn as the list item's marker.
		default:
			r.inline(x, style, spans)
		}
	}
}
