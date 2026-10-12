package gui

import (
	"testing"

	"github.com/dade/agentlog"
	"github.com/dade/distill/internal/env"
	"github.com/egoist/mygo/ui"
)

// TestDistillDialogFrame drags the dialog's title and its corner: the first
// moves it, the second resizes it about its top left corner.
func TestDistillDialogFrame(t *testing.T) {
	p := env.Paths{Home: t.TempDir()}
	a := &app{p: p, cfg: env.LoadConfig(p), infos: map[agentlog.Agent]*env.AgentInfo{}, md: newMDView()}
	for _, ag := range env.Agents {
		a.infos[ag] = &env.AgentInfo{Agent: ag}
	}
	a.sessions.init()
	a.library.init()
	a.openDistill([]agentlog.Session{{Agent: agentlog.Claude, Title: "一个会话", Path: "/x.jsonl"}})
	tt := ui.NewTester(func(c *ui.Context) { a.distillDialog(c) }, 1200, 800)
	tt.Frame()
	f := &a.distillFrame
	w0, h0 := f.w, f.h

	title, ok := tt.Find("AI 提炼")
	if !ok {
		t.Fatal("no title")
	}
	x, y := title.X+title.W+40, title.Y+title.H/2
	tt.Press(x, y)
	tt.Move(x+30, y+10)
	tt.Move(x+60, y+20)
	tt.Release(x+60, y+20)
	if f.dx != 60 || f.dy != 20 {
		t.Fatalf("after moving: offset %v,%v", f.dx, f.dy)
	}

	// The panel's bottom right corner, in the window.
	left, top := (1200-f.w)/2+f.dx, (800-f.h)/2+f.dy
	cx, cy := left+f.w-9, top+f.h-9
	tt.Press(cx, cy)
	tt.Move(cx+50, cy+30)
	tt.Release(cx+50, cy+30)
	if f.w != w0+50 || f.h != h0+30 {
		t.Fatalf("after resizing: %vx%v, want %vx%v", f.w, f.h, w0+50, h0+30)
	}
	if l, tp := (1200-f.w)/2+f.dx, (800-f.h)/2+f.dy; l != left || tp != top {
		t.Errorf("the top left corner moved from %v,%v to %v,%v", left, top, l, tp)
	}
}
