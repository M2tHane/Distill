package gui

import (
	"fmt"
	"slices"
	"strconv"

	"github.com/egoist/mygo/ui"

	"github.com/dade/agentlog"
	"github.com/dade/distill/internal/env"
	"github.com/dade/distill/internal/library"
)

const defaultModel = "默认模型"

// modelSelect chooses an agent's model: its default, or one it offers.
func (a *app) modelSelect(c *ui.Context, ag agentlog.Agent) {
	cfg := a.cfg.Agents[ag]
	options := append([]string{defaultModel}, a.infos[ag].Models...)
	model := cfg.Model
	if model == "" || !slices.Contains(options, model) {
		// A model no longer offered, as after the settings changed, is the default.
		model = defaultModel
	}
	if scrollSelect(c, &model, options, 240, "模型", nil) {
		if model == defaultModel {
			model = ""
		}
		cfg.Model = model
		a.saveConfig()
	}
}

func (a *app) agentsPage(c *ui.Context) {
	t := c.Theme()
	header(c, "Agent", func() {
		if iconButton(c, iconRefresh, "重新检测").Clicked() {
			for _, ag := range env.Agents {
				a.infos[ag].Detecting = true
			}
			go a.detectAgents()
		}
	})
	ui.Scroll(c).Grow(1).Padding(4, 24, 24).Gap(14).Children(func() {
		muted(c, "启用的 Agent 才能在「提炼」时选择，所选模型就是提炼时使用的模型。内置 Skill 会安装到对应 Agent 的 skills 目录。")
		ui.Column(c).Radius(12).Border(1, t.Border).Clip().Dividers(1, t.Border).Children(func() {
			for _, ag := range env.Agents {
				a.agentRow(c, ag)
			}
		})
	})
}

func (a *app) agentRow(c *ui.Context, ag agentlog.Agent) {
	t := c.Theme()
	info := a.infos[ag]
	cfg := a.cfg.Agents[ag]
	ui.Row(c.Key(ag)).Padding(14, 18).Gap(14).Children(func() {
		agentMark(c, ag, 34, info.Installed())
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Row(c).Gap(8).Children(func() {
				ui.Text(c, ag.Label()).FontSize(15).Bold()
				if info.Version != "" {
					muted(c, info.Version)
				}
			})
			ui.Row(c).Gap(6).Children(func() {
				switch {
				case info.Detecting && !info.Installed():
					ui.Spinner(c).Size(10, 10)
					muted(c, "检测中…").FontSize(12)
				case !info.Installed():
					dot(c, t.TextMuted)
					muted(c, "未安装：PATH 中找不到 "+env.Commands[ag]).FontSize(12)
				default:
					color := t.TextMuted
					state := "未启用"
					if cfg.Enabled {
						color, state = t.Success, "已启用"
					}
					dot(c, color)
					line := fmt.Sprintf("%s · %s · %d 个模型", state, info.Bin, len(info.Models))
					muted(c, line).FontSize(12).SingleLine()
				}
			})
		})
		if !info.Installed() {
			return
		}
		a.builtinButton(c, ag)
		a.modelSelect(c, ag)
		if ui.Switch(c, &cfg.Enabled).Label("启用").Changed() {
			a.saveConfig()
		}
	})
}

func dot(c *ui.Context, color ui.Color) {
	ui.Box(c).Size(7, 7).Radius(4).Background(color)
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
	b := ui.Button(c, "").Tooltip("distill-skill、distill-rule、distill-instructions").Children(func() {
		ui.Icon(c, iconSkill).FontSize(13)
		ui.Text(c, label).FontSize(12).SingleLine()
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
