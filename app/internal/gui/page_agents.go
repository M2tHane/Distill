package gui

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
)

const defaultModel = "默认模型"

// modelPick is an agent's model as its select shows it: value, which the
// select sets after the frame, and shown, what the configuration gave.
type modelPick struct{ value, shown string }

// modelSelect chooses an agent's model: its default, or one it offers.
func (a *app) modelSelect(c *ui.Context, ag agentlog.Agent, width float32) {
	cfg := a.cfg.Agents[ag]
	options := append([]string{defaultModel}, a.infos[ag].Models...)
	model := cfg.Model
	if model == "" || !slices.Contains(options, model) {
		// A model no longer offered, as after the settings changed, or not
		// yet listed while the agent is detected, shows as the default.
		model = defaultModel
	}
	if a.modelPicks == nil {
		a.modelPicks = map[agentlog.Agent]*modelPick{}
	}
	p := a.modelPicks[ag]
	if p == nil {
		p = &modelPick{model, model}
		a.modelPicks[ag] = p
	}
	switch {
	case p.value != p.shown:
		// Chosen since the last frame.
		cfg.Model = p.value
		if p.value == defaultModel {
			cfg.Model = ""
		}
		a.saveConfig()
		p.shown = p.value
	case model != p.shown:
		p.value, p.shown = model, model
	}
	scrollSelect(c, &p.value, options, width, "模型", nil)
}

func (a *app) agentsPage(c *ui.Context) {
	header(c, "Agent", func() {
		if iconButton(c, iconRefresh, "重新检测").Clicked() {
			for _, ag := range env.Agents {
				a.infos[ag].Detecting = true
			}
			go a.detectAgents()
		}
	})
	ui.Scroll(c).Grow(1).Padding(4, 28, 28).Gap(18).Children(func() {
		muted(c, "启用的 Agent 才能在「提炼」时选择，所选模型就是提炼时使用的模型。内置 Skill 会安装到对应 Agent 的 skills 目录。")
		// The model select narrows with the window.
		w, _ := c.Size()
		selectW := min(260, max(170, (w-860)/2+170))
		card(c).Padding(0).Gap(0).Children(func() {
			for i, ag := range env.Agents {
				a.agentRow(c, ag, selectW, i == len(env.Agents)-1)
			}
		})
	})
}

// agentRow shows an agent in a row of the list: its logo, name and version,
// whether it is installed and on, then its built-in skills, its model and
// its switch.
func (a *app) agentRow(c *ui.Context, ag agentlog.Agent, selectW float32, last bool) {
	t := c.Theme()
	info := a.infos[ag]
	cfg := a.cfg.Agents[ag]
	row := ui.Row(c.Key(ag)).Padding(14, 20).Gap(14).AlignItems(ui.Center)
	if !last {
		row.BorderWidth(0, 0, 1, 0).BorderColor(t.Border)
	}
	row.Children(func() {
		agentMark(c, ag, 30, info.Installed()).Shrink(0)
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, ag.Label()).FontSize(15).FontWeight(600).SingleLine()
				if info.Version != "" {
					muted(c, info.Version).FontSize(13).SingleLine().Tooltip(info.Bin)
				}
			})
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				var state string
				color := t.TextMuted
				switch {
				case info.Detecting && !info.Installed():
					ui.Spinner(c).Size(10, 10)
					state = "正在查找 " + env.Commands[ag]
				case !info.Installed():
					state = "未安装 · PATH 中找不到 " + env.Commands[ag]
				case cfg.Enabled:
					color = t.Success
					state = fmt.Sprintf("已启用 · %d 个模型", len(info.Models))
				default:
					state = fmt.Sprintf("未启用 · %d 个模型", len(info.Models))
				}
				if !(info.Detecting && !info.Installed()) {
					ui.Box(c).Size(7, 7).Radius(3.5).Background(color).Shrink(0)
				}
				muted(c, state).FontSize(12).SingleLine().Shrink(1).MinWidth(0)
			})
		})
		if !info.Installed() {
			return
		}
		a.builtinButton(c, ag)
		a.modelSelect(c, ag, selectW)
		if ui.Switch(c, &cfg.Enabled).Label("启用 " + ag.Label()).Changed() {
			a.saveConfig()
		}
	})
}

// builtinButton shows whether the built-in skills are in an agent's skills,
// and puts them there or takes them out.
func (a *app) builtinButton(c *ui.Context, ag agentlog.Agent) {
	if !a.library.loaded {
		a.library.refresh(a)
	}
	builtins := a.library.builtins
	n := 0
	for _, it := range builtins {
		if a.library.status(it, ag).Applied {
			n++
		}
	}
	label := "安装内置 Skill"
	if n == len(builtins) && n > 0 {
		label = "内置 Skill 已安装"
	} else if n > 0 {
		label = "内置 Skill " + strconv.Itoa(n) + "/" + strconv.Itoa(len(builtins))
	}
	t := c.Theme()
	b := ui.ButtonBase(c).Label(label).Tooltip(builtinNames(builtins)).Padding(5, 8).Gap(6).Radius(8).Shrink(0).Cursor(ui.CursorPointer)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, iconSkill).FontSize(13).TextColor(t.TextMuted)
		ui.Text(c, label).FontSize(13).SingleLine()
	})
	if b.Clicked() {
		install := n < len(builtins)
		if err := a.setBuiltin(ag, install); err != nil {
			c.AddToast(ui.Toast{Title: "操作失败", Description: err.Error(), Type: "error"})
		}
		a.library.refresh(a)
	}
}

func (a *app) setBuiltin(ag agentlog.Agent, install bool) error {
	for _, it := range a.lib.Scan(a.p.Builtin, true) {
		var err error
		if install {
			if !library.Status(a.p, it, ag).Applied {
				err = library.Apply(a.p, it, ag, a.cfg.SkillsCopy)
			}
		} else {
			err = library.Unapply(a.p, it, ag)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// builtinDialog asks, once, where to install the built-in skills.
func (a *app) builtinDialog(c *ui.Context) {
	t := c.Theme()
	if !a.builtinPrompt {
		return
	}
	ui.DialogBase(c, &a.builtinPrompt, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.3))
		panel.Width(460).Padding(22).Gap(14).Radius(14).Background(t.Background).Shadow(0, 12, 40, 0, ui.RGBA(0, 0, 0, 0.25))
		ui.Text(c, "安装内置 Skill").FontSize(17).Bold()
		muted(c, "Distill 提炼资源时，会让 Agent 使用这些 Skill。选择要安装到哪些 Agent（之后可在 Agent 页更改）：")
		ui.Column(c).Gap(6).Padding(4, 0).Children(func() {
			for _, it := range a.lib.Scan(a.p.Builtin, true) {
				ui.Row(c).Gap(8).Children(func() {
					ui.Icon(c, iconSkill).FontSize(13).TextColor(t.TextMuted)
					ui.Text(c, it.Name).Font("monospace").FontSize(12)
				})
			}
		})
		ui.Row(c).Gap(16).Children(func() {
			for _, ag := range env.Agents {
				if !a.infos[ag].Installed() {
					continue
				}
				v := a.builtinPick[ag]
				if ui.Checkbox(c.Key(ag), &v, ag.Label()).Changed() {
					a.builtinPick[ag] = v
				}
			}
		})
		ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
			if ui.Button(c, "以后再说").Clicked() {
				a.builtinPrompt = false
				a.cfg.BuiltinAsked = true
				a.saveConfig()
			}
			if ui.PrimaryButton(c, "安装").Clicked() {
				for _, ag := range env.Agents {
					if a.builtinPick[ag] {
						if err := a.setBuiltin(ag, true); err != nil {
							c.AddToast(ui.Toast{Title: "安装失败", Description: err.Error(), Type: "error"})
						}
					}
				}
				a.builtinPrompt = false
				a.cfg.BuiltinAsked = true
				a.saveConfig()
				a.library.refresh(a)
				c.Toast("内置 Skill 已安装")
			}
		})
	})
}

// builtinNames lists the built-in skills by name.
func builtinNames(items []library.Item) string {
	var names []string
	for _, it := range items {
		names = append(names, it.Name)
	}
	return strings.Join(names, "、")
}
