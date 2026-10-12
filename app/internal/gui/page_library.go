package gui

import (
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
)

type libraryState struct {
	items    []library.Item
	builtins []library.Item
	statuses map[string]library.ApplyStatus // by item id and agent
	loaded   bool

	kind  int    // 0: every kind, then kinds[kind-1]
	agent string // "": every agent, else the label of an agent it is applied to

	editing   *library.Item
	preview   int // 0: edit, 1: preview
	editOpen  bool
	editText  string
	editSaved string

	confirmRemove bool
	toRemove      library.Item
}

func (l *libraryState) init() {
	l.statuses = map[string]library.ApplyStatus{}
}

// refresh reads the library and where its resources are applied.
func (l *libraryState) refresh(a *app) {
	l.items = a.lib.Scan(a.p.Library, false)
	l.builtins = a.lib.Scan(a.p.Builtin, true)
	clear(l.statuses)
	for _, list := range [][]library.Item{l.items, l.builtins} {
		for _, it := range list {
			for _, ag := range env.Agents {
				l.statuses[it.ID()+"@"+string(ag)] = library.Status(a.p, it, ag)
			}
		}
	}
	l.loaded = true
}

func (l *libraryState) status(it library.Item, ag agentlog.Agent) library.ApplyStatus {
	return l.statuses[it.ID()+"@"+string(ag)]
}

func (a *app) libraryPage(c *ui.Context) {
	l := &a.library
	t := c.Theme()
	if !l.loaded {
		l.refresh(a)
	}
	header(c, "资源库", func() {
		if iconButton(c, iconRefresh, "刷新").Clicked() {
			l.refresh(a)
		}
		if ui.Button(c, "").Children(func() {
			ui.Icon(c, iconFolder).FontSize(13)
			ui.Text(c, "在 Finder 中打开").FontSize(13)
		}).Clicked() {
			mygo.Shell.OpenPath(a.p.Library)
		}
	})
	ui.Scroll(c).Grow(1).Padding(4, 24, 24).Gap(14).Children(func() {
		ui.Row(c).Gap(10).Children(func() {
			labels := []string{"全部"}
			for _, k := range library.Kinds {
				labels = append(labels, k.Label())
			}
			ui.Segmented(c, &l.kind, labels...).Label("类型")
			ui.Spacer(c)
			opts := []string{"按 Agent：全部"}
			for _, ag := range env.Agents {
				opts = append(opts, ag.Label())
			}
			sel := l.agent
			if sel == "" {
				sel = opts[0]
			}
			if ui.Select(c, &sel, opts).Width(170).Label("按 Agent").Changed() {
				l.agent = sel
				if sel == opts[0] {
					l.agent = ""
				}
			}
		})

		items := l.filter(l.items)
		muted(c, "资源库中").FontSize(12)
		ui.Column(c).Gap(10).Children(func() {
			if len(items) == 0 {
				ui.Column(c).Padding(28).Radius(12).Border(1, t.Border).AlignItems(ui.Center).Gap(6).Children(func() {
					muted(c, "资源库是空的")
					muted(c, "在「会话」页勾选会话，点「AI 提炼」生成 Skill、Rule、AGENTS.md 或 CLAUDE.md。").FontSize(12)
				})
			}
			for _, it := range items {
				a.itemCard(c, it)
			}
		})

		ui.Row(c).Gap(12).MarginY(6).Children(func() {
			ui.Text(c, "SKILLS 启用方式").FontSize(12).Bold().LetterSpacing(0.5)
			mode := 0
			if a.cfg.SkillsCopy {
				mode = 1
			}
			if ui.Segmented(c, &mode, "符号链接", "副本").FontSize(12).Label("启用方式").Changed() {
				a.cfg.SkillsCopy = mode == 1
				a.saveConfig()
			}
		})
		if a.cfg.SkillsCopy {
			muted(c, "副本是独立的文件：在 Distill 里编辑保存后会同步到各 Agent，在外面改资源库则不会。").FontSize(11)
		} else {
			muted(c, "符号链接跟着资源库里的 Skill 走：一更新，各 Agent 立刻就用上。Rule、AGENTS.md、CLAUDE.md 写入 Agent 的指令文件，保存时同步。").FontSize(11)
		}

		if builtins := l.filter(l.builtins); len(builtins) > 0 {
			muted(c, "内置").FontSize(12).MarginY(6)
			ui.Column(c).Gap(8).Children(func() {
				for _, it := range builtins {
					a.itemCard(c, it)
				}
			})
		}
	})
	a.editorDialog(c)
	if ui.AlertDialog(c, &l.confirmRemove, "删除「"+l.toRemove.Name+"」？", "会先从所有已应用的 Agent 中取消，再从资源库删除。", "取消", "删除") == 1 {
		if err := a.lib.Remove(l.toRemove); err != nil {
			c.AddToast(ui.Toast{Title: "删除失败", Description: err.Error(), Type: "error"})
		}
		l.refresh(a)
	}
}

func (l *libraryState) filter(list []library.Item) []library.Item {
	var out []library.Item
	for _, it := range list {
		if l.kind > 0 && it.Kind != library.Kinds[l.kind-1] {
			continue
		}
		if l.agent != "" {
			found := false
			for _, ag := range env.Agents {
				if ag.Label() == l.agent && l.status(it, ag).Applied {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func (a *app) itemCard(c *ui.Context, it library.Item) {
	l := &a.library
	t := c.Theme()
	card := ui.Row(c.Key(it.ID())).Padding(10, 14).Gap(12).Radius(12).Border(1, t.Border).Background(t.Background)
	if card.Hovered() {
		card.Shadow(0, 2, 8, 0, ui.RGBA(0, 0, 0, 0.06))
	}
	card.Children(func() {
		svg := iconFile
		if it.Kind == library.KindSkill {
			svg = iconSkill
		}
		ui.Box(c).Size(30, 30).Radius(7).Background(ui.Hex("#F4F4F5")).Center().Children(func() {
			ui.Icon(c, svg).FontSize(14).TextColor(t.TextMuted)
		})
		body := ui.Column(c).Grow(1).MinWidth(0).Gap(3).Cursor(ui.CursorPointer).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, it.Name).FontSize(13).Bold().SingleLine().Shrink(1)
				chip(c, it.Kind.Label(), t.TextMuted, ui.Hex("#F4F4F5"))
			})
			muted(c, it.Description).FontSize(11).SingleLine()
		})
		if body.Clicked() {
			a.openEditor(it)
		}
		if !it.Builtin && iconButton(c, iconTrash, "删除").Clicked() {
			l.toRemove = it
			l.confirmRemove = true
		}

		// Apply to every installed agent, or take it off them all.
		var installed []agentlog.Agent
		all := true
		for _, ag := range env.Agents {
			if a.infos[ag].Installed() {
				installed = append(installed, ag)
				if !l.status(it, ag).Applied {
					all = false
				}
			}
		}
		if ui.Button(c, "全部").FontSize(12).Padding(3, 10).Disabled(len(installed) == 0).Tooltip("应用到所有已安装的 Agent，再点一次全部取消").Clicked() {
			for _, ag := range installed {
				a.toggleApply(c, it, ag, !all)
			}
			l.refresh(a)
		}
		ui.Row(c).Gap(6).Children(func() {
			for _, ag := range env.Agents {
				st := l.status(it, ag)
				tip := ag.Label() + "：未应用，点击应用"
				switch {
				case !a.infos[ag].Installed() && !a.infos[ag].Detecting:
					tip = ag.Label() + "：未安装"
				case st.Conflict != "":
					tip = ag.Label() + "：" + st.Conflict + " 已被占用"
				case st.Applied && st.Linked:
					tip = ag.Label() + "：已应用（符号链接），点击取消"
				case st.Applied:
					tip = ag.Label() + "：已应用，点击取消"
				}
				b := ui.ButtonBase(c.Key(ag)).Label(tip).Tooltip(tip).Padding(4).Radius(7).Cursor(ui.CursorPointer)
				if st.Applied {
					b.Border(1, agentColors[ag].Alpha(0.5))
				} else {
					b.Border(1, t.Border).Opacity(0.7)
				}
				b.Children(func() { agentMark(c, ag, 18, st.Applied) })
				if b.Clicked() && a.infos[ag].Installed() {
					a.toggleApply(c, it, ag, !st.Applied)
					l.refresh(a)
				}
			}
		})
	})
}

func (a *app) toggleApply(c *ui.Context, it library.Item, ag agentlog.Agent, on bool) {
	var err error
	if on {
		if a.library.status(it, ag).Applied {
			return
		}
		err = library.Apply(a.p, it, ag, a.cfg.SkillsCopy)
	} else {
		err = library.Unapply(a.p, it, ag)
	}
	if err != nil {
		c.AddToast(ui.Toast{Title: ag.Label() + "：操作失败", Description: err.Error(), Type: "error"})
	}
}

func (a *app) openEditor(it library.Item) {
	l := &a.library
	data, err := os.ReadFile(it.File)
	if err != nil {
		a.toast = &ui.Toast{Title: "打不开", Description: err.Error(), Type: "error"}
		return
	}
	l.editing = &it
	l.editText = string(data)
	l.editSaved = l.editText
	l.preview = 0
	l.editOpen = true
}

func (a *app) editorDialog(c *ui.Context) {
	l := &a.library
	t := c.Theme()
	if !l.editOpen || l.editing == nil {
		return
	}
	it := *l.editing
	dirty := l.editText != l.editSaved
	ui.DialogBase(c, &l.editOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.3))
		panel.WidthPercent(76).MaxWidth(940).HeightPercent(84).Padding(18, 20).Gap(12).Radius(14).Background(t.Background).
			Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.28))
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, it.Name).FontSize(17).Bold()
			chip(c, it.Kind.Label(), t.TextMuted, ui.Hex("#F4F4F5"))
			if it.Builtin {
				chip(c, "内置 · 只读", t.TextMuted, ui.Hex("#F4F4F5"))
			}
			ui.Spacer(c)
			if iconButton(c, iconFolder, "在 Finder 中显示").Clicked() {
				mygo.Shell.ShowItemInFolder(it.File)
			}
			if iconButton(c, iconX, "关闭").Clicked() {
				l.editOpen = false
			}
		})
		muted(c, it.File).Font("monospace").FontSize(11).SingleLine()
		ui.Segmented(c, &l.preview, "编辑", "预览").FontSize(12).Label("编辑或预览").AlignSelf(ui.Start)
		if l.preview == 1 {
			ui.Scroll(c).Grow(1).Padding(12, 16).Radius(8).Border(1, t.Border).Selectable().Children(func() {
				meta, body := library.FrontMatter(l.editText)
				if len(meta) > 0 {
					ui.Column(c).Gap(2).Padding(8, 10).Margin(0, 0, 8, 0).Radius(6).Background(t.Surface).Children(func() {
						for _, k := range []string{"name", "description", "paths"} {
							if v := meta[k]; v != "" {
								ui.RichText(c, ui.Span{Text: k + "：", Weight: 700}, ui.Span{Text: v}).FontSize(12)
							}
						}
					})
				}
				markdown(c, body, a.md)
			})
		} else {
			ui.TextArea(c, &l.editText).Grow(1).Font("monospace").FontSize(13).ReadOnly(it.Builtin)
		}
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if dirty {
				muted(c, "未保存").FontSize(12)
			}
			ui.Spacer(c)
			if ui.Button(c, "关闭").Clicked() {
				l.editOpen = false
			}
			if !it.Builtin && (ui.PrimaryButton(c, "保存").Disabled(!dirty).Clicked() || (dirty && c.Shortcut(ui.Cmd, ui.KeyS))) {
				if err := a.lib.Save(it, l.editText, a.cfg.SkillsCopy); err != nil {
					c.AddToast(ui.Toast{Title: "保存失败", Description: err.Error(), Type: "error"})
				} else {
					l.editSaved = l.editText
					l.refresh(a)
					c.Toast("已保存")
				}
			}
		})
	})
}
