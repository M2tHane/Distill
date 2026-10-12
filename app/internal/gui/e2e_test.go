package gui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dade/agentlog"
	"github.com/dade/agentlog/sources"
	"github.com/dade/distill/internal/env"
	"github.com/dade/distill/internal/library"
	"github.com/dade/distill/internal/runner"
	"github.com/dade/distill/internal/testutil"
)

// TestDistillE2E distills a real session into a temporary library with
// Claude Code, as the app does: transcripts under runs/, the first prompt,
// and a resource the agent writes.
//
//	DISTILL_E2E=1 go test -run E2E -v
func TestDistillE2E(t *testing.T) {
	if os.Getenv("DISTILL_E2E") == "" {
		t.Skip("DISTILL_E2E not set")
	}
	env.LoginPath()
	real := env.NewPaths()
	root := testutil.AgentDir(t)
	p := real
	p.Root, p.Library, p.Builtin, p.Runs = root, filepath.Join(root, "library"), filepath.Join(root, "builtin"), filepath.Join(root, "runs")
	a := &app{p: p, srcs: sources.All(real.Home), lib: library.Library{P: p}}
	if err := a.writeBuiltin(); err != nil {
		t.Fatal(err)
	}

	// A short session of the user's, with a few messages.
	res, err := (&agentlog.Index{Sources: a.srcs, CachePath: real.Index}).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	var ref agentlog.Session
	for _, s := range res.Sessions {
		if s.Agent == agentlog.Claude && s.Messages >= 6 && s.Messages <= 40 && s.CWD != real.Root {
			ref = s
			break
		}
	}
	if ref.Path == "" {
		t.Skip("no short Claude Code session")
	}
	files, _, err := a.exportRefs([]agentlog.Session{ref})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("ref %q → %v", ref.Title, files)

	info := env.Detect(p, agentlog.Claude)
	before := a.lib.Signature()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	var log strings.Builder
	_, err = runner.Run(ctx, runner.Request{
		Info: info, Model: "haiku", Dir: root, SessionID: runner.NewUUID(),
		Prompt: firstPrompt(p, files, "从这个会话提炼 1 条 Rule（我对助手的要求或纠正），写成一个 Rule 文件。只写一个文件。"),
	}, func(ev runner.Event) {
		switch {
		case ev.Tool != nil:
			log.WriteString("[" + ev.Tool.Name + "] " + ev.Tool.Summary + "\n")
		case ev.Err != "":
			log.WriteString("ERR " + ev.Err + "\n")
		default:
			log.WriteString(ev.Text + "\n")
		}
	})
	t.Log(log.String())
	if err != nil {
		t.Fatal(err)
	}
	changed := library.Diff(before, a.lib.Signature())
	t.Logf("changed: %v", changed)
	if len(changed) == 0 {
		t.Fatal("the agent wrote nothing to the library")
	}
	for _, it := range a.lib.Scan(p.Library, false) {
		data, _ := os.ReadFile(it.File)
		t.Logf("%s (%s): %s\n%s", it.ID(), it.Description, it.File, data)
	}
}
