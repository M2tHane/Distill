// Package testutil helps tests that run the real agents.
package testutil

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/sources"
	"github.com/M2tHane/Distill/app/internal/env"
)

// AgentDir returns a working directory for an agent run under
// ~/.distill/test, which Distill hides from its session list. When the test
// ends it removes the directory and the sessions the agents logged there,
// so tests leave nothing in the user's history.
func AgentDir(t *testing.T) string {
	t.Helper()
	p := env.NewPaths()
	base := filepath.Join(p.Root, "test")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(base, strings.ReplaceAll(t.Name(), "/", "-")+"-")
	if err != nil {
		t.Fatal(err)
	}
	// Agents resolve symbolic links in their working directory.
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		dir = real
	}
	t.Cleanup(func() {
		os.RemoveAll(dir)
		res, _ := (&agentlog.Index{Sources: sources.All(p.Home)}).Scan(context.Background())
		for _, s := range res.Sessions {
			if s.CWD == dir || strings.HasPrefix(s.CWD, dir+"/") {
				os.Remove(s.Path)
				if s.Agent == agentlog.Claude {
					os.RemoveAll(strings.TrimSuffix(s.Path, ".jsonl"))
				}
				// The project's directory, once empty: Claude Code also makes
				// an empty memory directory there.
				os.Remove(filepath.Join(filepath.Dir(s.Path), "memory"))
				os.Remove(filepath.Dir(s.Path))
			}
		}
	})
	return dir
}
