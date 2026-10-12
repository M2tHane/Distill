package gui

import (
	"context"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/sources"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
)

// TestSnapshots draws the pages with this computer's real sessions and
// writes them as PNG files to $DISTILL_SNAPSHOTS, to look at the layout.
//
//	DISTILL_SNAPSHOTS=/tmp/shots go test -run Snapshots
func TestSnapshots(t *testing.T) {
	out := os.Getenv("DISTILL_SNAPSHOTS")
	if out == "" {
		t.Skip("DISTILL_SNAPSHOTS not set")
	}
	os.MkdirAll(out, 0o755)
	p := env.NewPaths()
	a := &app{
		p:     p,
		cfg:   env.LoadConfig(p),
		srcs:  sources.All(p.Home),
		lib:   library.Library{P: p},
		page:  pageSessions,
		infos: map[agentlog.Agent]*env.AgentInfo{},
		md:    newMDView(),
	}
	a.ix = &agentlog.Index{Sources: a.srcs, CachePath: p.Index}
	a.trash = agentlog.Trash{Dir: p.Trash, Sources: a.srcs}
	a.sessions.init()
	a.library.init()
	env.LoginPath()
	for _, ag := range env.Agents {
		info := env.Detect(p, ag)
		a.infos[ag] = &info
	}
	res, err := a.ix.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Sessions {
		if !strings.HasPrefix(s.CWD+"/", p.Root+"/") && s.Messages > 0 {
			a.sessions.all = append(a.sessions.all, s)
		}
	}
	a.sessions.loaded = true
	first := a.sessions.all[0]
	a.sessions.expanded[first.CWD] = true

	tt := ui.NewTester(a.view, 1180, 780)
	shot := func(name string) {
		tt.Frame()
		tt.Frame()
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, tt.Image()); err != nil {
			t.Fatal(err)
		}
	}
	shot("1-sessions")

	a.sessions.selected[first.Path] = first
	tr, err := agentlog.Load(a.srcs, first)
	if err != nil {
		t.Fatal(err)
	}
	a.sessions.view.session = &first
	a.sessions.view.transcript = tr
	a.sessions.view.toolsOpen = map[int]bool{2: true}
	a.sessions.view.inputOpen = map[[2]int]bool{}
	a.sessions.view.split = 560
	shot("2-transcript")

	a.sessions.tab = 0
	refs := a.sessions.all[:7]
	a.openDistill(refs)
	shot("3-distill")
	if err := tt.Click("选择 Agent"); err == nil {
		shot("3b-distill-agents")
		tt.Key(0, ui.KeyEscape)
	}
	a.distill.open = false

	a.page = pageAgents
	shot("4-agents")
	if err := tt.Click("模型"); err == nil {
		shot("4b-agents-models")
		tt.Key(0, ui.KeyEscape)
	}
	a.page = pageLibrary
	shot("5-library")
}
