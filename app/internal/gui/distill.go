package gui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
	"github.com/M2tHane/Distill/app/internal/runner"
)

// distillState is the conversation with an agent about the sessions
// referenced, which it turns into resources of the library.
type distillState struct {
	open      bool
	refs      []agentlog.Session
	runner    string // the label of the agent chosen
	msgs      []agentlog.Message
	errs      map[int]string // errors under messages
	draft     string
	running   bool
	cancel    context.CancelFunc
	sessionID string
	started   bool   // the agent session exists: later messages resume it
	runDir    string // runs/<id>, where the transcripts are
	refFiles  []string
	list      ui.ListState
	toolsOpen map[int]bool
	inputOpen map[[2]int]bool
	changed   []string // resources the last run wrote
}

// quickPrompts start a request for each kind of resource.
var quickPrompts = []struct{ label, text string }{
	{"生成 Skill", "请使用 distill-skill，根据这些会话提炼可复用的 Skill。"},
	{"提炼 Rules", "请使用 distill-rule，从这些会话中提炼我的偏好、约束和纠正，写成 Rule。"},
	{"写 AGENTS.md", "请使用 distill-instructions，根据这些会话写一份 AGENTS.md 片段。"},
	{"写 CLAUDE.md", "请使用 distill-instructions，根据这些会话写一份 CLAUDE.md 片段。"},
}

func (a *app) openDistill(refs []agentlog.Session) {
	d := &a.distill
	if d.running {
		d.open = true
		return
	}
	*d = distillState{
		open:      true,
		refs:      refs,
		errs:      map[int]string{},
		toolsOpen: map[int]bool{},
		inputOpen: map[[2]int]bool{},
		sessionID: runner.NewUUID(),
		runner:    a.runnerLabel(a.cfg.Runner),
	}
	d.list.FollowEnd = true
}

// enabledRunners are the agents that can distill: installed and enabled.
func (a *app) enabledRunners() []agentlog.Agent {
	var list []agentlog.Agent
	for _, ag := range env.Agents {
		if a.infos[ag].Installed() && a.cfg.Agents[ag].Enabled {
			list = append(list, ag)
		}
	}
	return list
}

func (a *app) runnerLabel(ag agentlog.Agent) string {
	if ag == "" || a.cfg.Agents[ag] == nil {
		return ""
	}
	m := a.cfg.Agents[ag].Model
	if m == "" {
		m = defaultModel
	}
	return ag.Label() + " · " + m
}

func (a *app) distillDialog(c *ui.Context) {
	d := &a.distill
	t := c.Theme()
	if !d.open {
		return
	}
	ui.DialogBase(c, &d.open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.3))
		f := &a.distillFrame
		f.fit(c.Size())
		panel.Width(f.w).Height(f.h).Translate(f.dx, f.dy).Radius(14).Background(t.Background).Clip().
			Shadow(0, 16, 48, 0, ui.RGBA(0, 0, 0, 0.28))
		// The edges and the corner resize it, built last to be on top.
		defer f.handles(c)

		runners := a.enabledRunners()
		var labels []string
		for _, ag := range runners {
			labels = append(labels, a.runnerLabel(ag))
		}
		if len(labels) > 0 && !contains(labels, d.runner) {
			d.runner = labels[0]
		}

		// Title, and the agent that distills; dragging it moves the dialog.
		title := ui.Row(c).Padding(14, 14, 12, 20).Gap(10).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Children(func() {
			ui.Icon(c, iconSparkles).FontSize(16).TextColor(t.Accent)
			ui.Text(c, "AI 提炼").FontSize(16).Bold()
			ui.Spacer(c)
			if len(labels) > 0 {
				muted(c, "使用").FontSize(12)
				mark := func(label string) {
					for _, ag := range runners {
						if a.runnerLabel(ag) == label {
							agentMark(c, ag, 18, true)
						}
					}
				}
				if d.running || d.started {
					// The conversation goes on with the agent it started with.
					ui.Row(c).Gap(8).Padding(0, 10).Height(30).Children(func() {
						mark(d.runner)
						ui.Text(c, d.runner).SingleLine()
					})
				} else if scrollSelect(c, &d.runner, labels, 280, "选择 Agent", mark) {
					for _, ag := range runners {
						if a.runnerLabel(ag) == d.runner {
							a.cfg.Runner = ag
							a.saveConfig()
						}
					}
				}
			}
			if iconButton(c, iconX, "关闭").Clicked() {
				d.open = false
			}
		})
		if dx, dy, ok := title.Dragged(); ok {
			f.dx, f.dy = f.dx+dx, f.dy+dy
		}

		if len(labels) == 0 {
			ui.Column(c).Grow(1).Center().Gap(12).Children(func() {
				muted(c, "还没有启用的 Agent。提炼需要调用本机已安装的 Claude Code、Codex 或 Pi。")
				if ui.PrimaryButton(c, "去 Agent 页启用").Clicked() {
					d.open = false
					a.page = pageAgents
				}
			})
			return
		}

		a.refList(c)

		// The conversation.
		ui.List(c, &d.list, len(d.msgs), func(i int) {
			bubble(c, d.msgs[i], i, d.toolsOpen, d.inputOpen, a.md)
			if e := d.errs[i]; e != "" {
				ui.Text(c, e).TextColor(t.Danger).FontSize(12).Selectable().Padding(4, 4)
			}
		}).Grow(1).Padding(16, 20).Gap(12).Children(func() {
			if len(d.msgs) == 0 {
				ui.Column(c).Padding(40, 24).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(c, "告诉 AI 你想从这些会话里得到什么").FontSize(15).Bold()
					muted(c, "它会读取引用的会话记录，用内置 Skill 生成 Skill、Rule、AGENTS.md 或 CLAUDE.md，写进资源库。").TextAlign(ui.Center)
				})
			}
		})
		if d.running {
			ui.Row(c).Padding(0, 20, 8).Gap(8).Children(func() {
				ui.Spinner(c).Size(12, 12)
				muted(c, "正在运行…").FontSize(12)
			})
		}
		if len(d.changed) > 0 && !d.running {
			ui.Row(c).Margin(0, 20, 8).Padding(8, 12).Gap(8).Radius(8).Background(t.Success.Alpha(0.1)).Children(func() {
				ui.Text(c, "资源库有更新："+strings.Join(d.changed, "、")).FontSize(12).Grow(1).MaxLines(2)
				if ui.Button(c, "查看资源库").Clicked() {
					d.open = false
					a.page = pageLibrary
					a.library.refresh(a)
				}
			})
		}

		// The references and the message.
		ui.Column(c).Padding(10, 20, 16).Gap(8).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
			if !d.started {
				ui.Row(c).Gap(6).Wrap().Children(func() {
					for _, q := range quickPrompts {
						if ui.Button(c.Key(q.label), q.label).FontSize(12).Padding(3, 10).Radius(999).Clicked() {
							d.draft = q.text
						}
					}
				})
			}
			ui.Row(c).Gap(8).AlignItems(ui.End).Children(func() {
				in := ui.TextArea(c, &d.draft).Lines(2, 8).Grow(1).Placeholder("例如：把我做前端改版的流程整理成一个 Skill；或者总结我反复强调的代码规范。⌘↩ 发送").Disabled(d.running)
				send := c.Shortcut(ui.Cmd, ui.KeyEnter) && in.FocusWithin()
				if d.running {
					if ui.Button(c, "").Children(func() {
						ui.Icon(c, iconStop).FontSize(13)
						ui.Text(c, "停止")
					}).Clicked() && d.cancel != nil {
						d.cancel()
					}
				} else if ui.PrimaryButton(c, "").Disabled(strings.TrimSpace(d.draft) == "" || len(d.refs) == 0).Children(func() {
					ui.Icon(c, iconSend).FontSize(13)
					ui.Text(c, "发送")
				}).Clicked() || send {
					a.send(runners)
				}
			})
		})
	})
}

// refRow is the height of a reference in the list; it shows five before
// it scrolls.
const refRow = 30

// refList shows the sessions referenced, one per row, each with a button
// taking it out until the conversation starts.
func (a *app) refList(c *ui.Context) {
	d := &a.distill
	t := c.Theme()
	now := time.Now()
	ui.Column(c).Padding(10, 20, 10).Gap(6).BorderWidth(0, 0, 1, 0).BorderColor(t.Border).Shrink(0).Children(func() {
		ui.Row(c).Gap(6).Children(func() {
			ui.Text(c, fmt.Sprintf("引用 %d 个会话", len(d.refs))).FontSize(12).Bold()
			if d.started {
				muted(c, "· 已发送给 Agent").FontSize(12)
			}
		})
		if len(d.refs) == 0 {
			muted(c, "没有引用的会话：回到会话页勾选后再来。").FontSize(12)
			return
		}
		ui.Scroll(c).MaxHeight(refRow * 5).Children(func() {
			for i, x := range d.refs {
				ui.Row(c.Key(x.Path)).Height(refRow).Gap(8).Padding(0, 4, 0, 6).Radius(6).Children(func() {
					agentMark(c, x.Agent, 16, true)
					ui.Text(c, x.Title).FontSize(13).SingleLine().Shrink(1).MinWidth(0)
					muted(c, fmt.Sprintf("%s · %s · %s", filepath.Base(x.CWD), ago(x.Updated, now), humanSize(x.Size))).FontSize(11).SingleLine().Shrink(0)
					ui.Spacer(c)
					if !d.started && !d.running && iconButton(c, iconX, "取消引用").Padding(3).Clicked() {
						d.refs = append(d.refs[:i:i], d.refs[i+1:]...)
					}
				})
			}
		})
	})
}

// send gives the draft to the agent: the first message with the
// references and where to write, the next ones as they are.
func (a *app) send(runners []agentlog.Agent) {
	d := &a.distill
	text := strings.TrimSpace(d.draft)
	if text == "" || d.running || len(d.refs) == 0 {
		return
	}
	var ag agentlog.Agent
	for _, r := range runners {
		if a.runnerLabel(r) == d.runner {
			ag = r
		}
	}
	if ag == "" {
		return
	}
	d.draft = ""
	d.msgs = append(d.msgs, agentlog.Message{Role: agentlog.User, Text: text, Time: time.Now()})
	d.running = true
	d.changed = nil
	ctx, cancel := context.WithCancel(context.Background())
	d.cancel = cancel
	req := runner.Request{
		Info:      *a.infos[ag],
		Model:     a.cfg.Agents[ag].Model,
		Dir:       a.p.Root,
		SessionID: d.sessionID,
		Resume:    d.started,
	}
	first := !d.started
	refs := d.refs
	go func() {
		defer cancel()
		if first {
			files, dir, err := a.exportRefs(refs)
			if err != nil {
				a.win.Update(func() {
					d.running = false
					d.errs[len(d.msgs)-1] = "导出会话记录失败：" + err.Error()
				})
				return
			}
			a.win.Update(func() { d.refFiles, d.runDir = files, dir })
			req.Prompt = firstPrompt(a.p, files, text)
		} else {
			req.Prompt = text
		}
		before := a.lib.Signature()
		a.win.Update(func() { d.started = true })
		id, err := runner.Run(ctx, req, func(ev runner.Event) {
			a.win.Update(func() { d.event(ev) })
		})
		changed := library.Diff(before, a.lib.Signature())
		a.win.Update(func() {
			d.running = false
			d.cancel = nil
			if id != "" {
				d.sessionID = id
			}
			if err == context.Canceled {
				d.errs[len(d.msgs)-1] = "已停止"
			}
			d.changed = changed
			if len(changed) > 0 {
				a.library.refresh(a)
				a.toast = &ui.Toast{Title: fmt.Sprintf("资源库更新了 %d 项", len(changed)), Description: strings.Join(changed, "、")}
			}
		})
	}()
}

// event adds what the agent did to the conversation.
func (d *distillState) event(ev runner.Event) {
	last := len(d.msgs) - 1
	switch {
	case ev.Text != "":
		if d.msgs[last].Role == agentlog.Assistant && len(d.msgs[last].Tools) == 0 {
			d.msgs[last].Text = strings.TrimSpace(d.msgs[last].Text + "\n\n" + ev.Text)
			return
		}
		d.msgs = append(d.msgs, agentlog.Message{Role: agentlog.Assistant, Text: ev.Text, Time: time.Now()})
	case ev.Tool != nil:
		if d.msgs[last].Role != agentlog.Assistant {
			d.msgs = append(d.msgs, agentlog.Message{Role: agentlog.Assistant, Time: time.Now()})
			last++
		}
		d.msgs[last].Tools = append(d.msgs[last].Tools, *ev.Tool)
	case ev.Err != "":
		d.errs[last] = strings.TrimSpace(d.errs[last] + "\n" + ev.Err)
	}
}

// exportRefs writes the referenced sessions as Markdown under runs/, where
// the agent reads them.
func (a *app) exportRefs(refs []agentlog.Session) ([]string, string, error) {
	dir := filepath.Join(a.p.Runs, time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	var files []string
	for i, x := range refs {
		name := fmt.Sprintf("%02d-%s-%s.md", i+1, x.Agent, fileName(x.Title))
		if err := a.writeMarkdown(x, filepath.Join(dir, name)); err != nil {
			return nil, "", err
		}
		rel, _ := filepath.Rel(a.p.Root, filepath.Join(dir, name))
		files = append(files, rel)
	}
	return files, dir, nil
}

func firstPrompt(p env.Paths, files []string, request string) string {
	var b strings.Builder
	b.WriteString("你在 Distill 中工作，工作目录是 " + p.Root + "。\n\n")
	b.WriteString("用户引用了以下会话记录（Markdown，只含用户输入、助手回复和工具调用摘要，没有工具输出）：\n")
	for _, f := range files {
		b.WriteString("- " + f + "\n")
	}
	b.WriteString(`
资源库在 library/，格式见 library/README.md：
- Skill：library/skills/<name>/SKILL.md
- Rule：library/rules/<name>.md
- AGENTS.md 片段：library/env.Agents-md/<name>.md
- CLAUDE.md 片段：library/claude-md/<name>.md

按用户的需求，使用对应的内置 Skill（distill-skill、distill-rule、distill-instructions）；如果这些 Skill 不可用，就按 library/README.md 的格式直接写。
只在 library/ 中创建或修改文件，不要改动其它位置。完成后列出写入的文件。

用户的需求：
`)
	b.WriteString(request)
	return b.String()
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// panelFrame is where a dialog the user moves and resizes is: its size,
// and its offset from the center of the window. It lasts while the app
// runs, so the dialog opens again where it was left.
type panelFrame struct {
	dx, dy, w, h float32
}

const (
	panelMinW = 560
	panelMinH = 420
	panelEdge = 6 // the width of the edges that resize it
)

// fit sizes a new frame for the window and keeps it within the window.
func (f *panelFrame) fit(ww, wh float32) {
	if f.w == 0 {
		f.w, f.h = min(980, ww*0.78), wh*0.86
	}
	f.w = max(min(f.w, ww-24), min(panelMinW, ww-24))
	f.h = max(min(f.h, wh-24), min(panelMinH, wh-24))
	limX, limY := (ww-f.w)/2, (wh-f.h)/2
	f.dx = max(-limX, min(f.dx, limX))
	f.dy = max(-limY, min(f.dy, limY))
}

// handles are the edges and the corner that resize the dialog; the side
// opposite the one dragged stays where it is.
func (f *panelFrame) handles(c *ui.Context) {
	edge := func(key string, cursor ui.Cursor, place func(ui.Element) ui.Element, resize func(dx, dy float32)) {
		e := place(ui.Box(c.Key(key)).Absolute().Cursor(cursor))
		if dx, dy, ok := e.Dragged(); ok {
			resize(dx, dy)
		}
	}
	edge("r", ui.CursorResizeEW, func(e ui.Element) ui.Element { return e.Top(0).Bottom(0).Right(0).Width(panelEdge) },
		func(dx, _ float32) { f.w += dx; f.dx += dx / 2 })
	edge("l", ui.CursorResizeEW, func(e ui.Element) ui.Element { return e.Top(0).Bottom(0).Left(0).Width(panelEdge) },
		func(dx, _ float32) { f.w -= dx; f.dx += dx / 2 })
	edge("b", ui.CursorResizeNS, func(e ui.Element) ui.Element { return e.Left(0).Right(0).Bottom(0).Height(panelEdge) },
		func(_, dy float32) { f.h += dy; f.dy += dy / 2 })
	edge("t", ui.CursorResizeNS, func(e ui.Element) ui.Element { return e.Left(0).Right(0).Top(0).Height(panelEdge) },
		func(_, dy float32) { f.h -= dy; f.dy += dy / 2 })
	edge("br", ui.CursorResizeNWSE, func(e ui.Element) ui.Element { return e.Right(0).Bottom(0).Size(16, 16) },
		func(dx, dy float32) { f.w += dx; f.dx += dx / 2; f.h += dy; f.dy += dy / 2 })
}
