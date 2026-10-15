package gui

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
)

// TestChooseModel picks a model on the Agent page: the configuration
// takes it.
func TestChooseModel(t *testing.T) {
	p := env.Paths{Home: t.TempDir()}
	p.Root = p.Home + "/.distill"
	p.Config = p.Root + "/config.json"
	a := &app{p: p, cfg: env.LoadConfig(p), page: pageAgents, infos: map[agentlog.Agent]*env.AgentInfo{}, md: newMDView()}
	for _, ag := range env.Agents {
		a.infos[ag] = &env.AgentInfo{Agent: ag}
	}
	a.infos[agentlog.Claude] = &env.AgentInfo{Agent: agentlog.Claude, Bin: "/bin/claude", Version: "1", Models: []string{"opus", "sonnet"}}
	a.sessions.init()
	a.library.init()
	a.library.loaded = true
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Frame()
	if err := tt.Click("模型"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("sonnet"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if got := a.cfg.Agents[agentlog.Claude].Model; got != "sonnet" {
		t.Fatalf("model %q, want sonnet", got)
	}
	if got := env.LoadConfig(p).Agents[agentlog.Claude].Model; got != "sonnet" {
		t.Fatalf("saved model %q, want sonnet", got)
	}

	// Started again: the models are listed only once the agent is detected,
	// which must not take the saved model for a new choice.
	a.cfg = env.LoadConfig(p)
	a.modelPicks = nil
	models := a.infos[agentlog.Claude].Models
	a.infos[agentlog.Claude].Models = nil
	tt.Frame()
	a.infos[agentlog.Claude].Models = models
	tt.Frame()
	tt.Frame()
	if got := a.cfg.Agents[agentlog.Claude].Model; got != "sonnet" {
		t.Fatalf("after detection, model %q, want sonnet", got)
	}
}

// TestChooseSkillsMode switches the library to copies: the configuration
// takes it.
func TestChooseSkillsMode(t *testing.T) {
	p := env.Paths{Home: t.TempDir()}
	p.Root = p.Home + "/.distill"
	p.Config = p.Root + "/config.json"
	a := &app{p: p, cfg: env.LoadConfig(p), page: pageLibrary, infos: map[agentlog.Agent]*env.AgentInfo{}, md: newMDView()}
	for _, ag := range env.Agents {
		a.infos[ag] = &env.AgentInfo{Agent: ag}
	}
	a.sessions.init()
	a.library.init()
	a.library.loaded = true
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Frame()
	if err := tt.Click("副本"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if !a.cfg.SkillsCopy || !env.LoadConfig(p).SkillsCopy {
		t.Fatal("not copies")
	}
	if err := tt.Click("符号链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if a.cfg.SkillsCopy {
		t.Fatal("still copies")
	}
}

func TestElapsed(t *testing.T) {
	for d, want := range map[time.Duration]string{
		8 * time.Second:               "8 秒",
		65 * time.Second:              "1 分 05 秒",
		time.Hour + 2*time.Minute + 9: "1 小时 02 分",
	} {
		if got := elapsed(d); got != want {
			t.Errorf("elapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

// TestChooseEffort picks an effort in the distill dialog: the agent's
// configuration takes it, and a model without levels has no select.
func TestChooseEffort(t *testing.T) {
	p := env.Paths{Home: t.TempDir()}
	p.Root = p.Home + "/.distill"
	p.Config = p.Root + "/config.json"
	a := &app{p: p, cfg: env.LoadConfig(p), page: pageSessions, infos: map[agentlog.Agent]*env.AgentInfo{}, md: newMDView()}
	for _, ag := range env.Agents {
		a.infos[ag] = &env.AgentInfo{Agent: ag}
	}
	a.infos[agentlog.Claude] = &env.AgentInfo{Agent: agentlog.Claude, Bin: "/bin/claude", Version: "1", Models: []string{"opus", "claude-haiku-4-5"}}
	a.cfg.Agents[agentlog.Claude].Enabled = true
	a.sessions.init()
	a.library.init()
	a.library.loaded = true
	a.openDistill([]agentlog.Session{{Agent: agentlog.Claude, Title: "t"}})
	tt := ui.NewTester(a.view, 1180, 780)
	tt.Frame()
	if err := tt.Click("选择 Effort"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if err := tt.Click("xhigh"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	tt.Frame()
	if got := env.LoadConfig(p).Agents[agentlog.Claude].Effort; got != "xhigh" {
		t.Fatalf("saved effort %q, want xhigh", got)
	}
	if got := a.effort(agentlog.Claude); got != "xhigh" {
		t.Fatalf("effort %q, want xhigh", got)
	}

	a.cfg.Agents[agentlog.Claude].Model = "claude-haiku-4-5"
	tt.Frame()
	tt.Frame()
	if err := tt.Click("选择 Effort"); err == nil {
		t.Fatal("a model without effort levels has an effort select")
	}
	if got := a.effort(agentlog.Claude); got != "" {
		t.Fatalf("effort %q for a model without levels", got)
	}
}
