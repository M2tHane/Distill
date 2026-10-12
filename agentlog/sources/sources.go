// Package sources lists the agents whose sessions agentlog reads.
package sources

import (
	"os"
	"path/filepath"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/sources/claude"
	"github.com/M2tHane/Distill/agentlog/sources/codex"
	"github.com/M2tHane/Distill/agentlog/sources/pi"
)

// All returns a source for every supported agent, reading under home (the
// user's home directory when empty). Add new agents here.
func All(home string) []agentlog.Source {
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return []agentlog.Source{
		&claude.Source{Root: filepath.Join(home, ".claude", "projects")},
		&codex.Source{Root: filepath.Join(home, ".codex")},
		&pi.Source{Root: filepath.Join(home, ".pi", "agent", "sessions")},
	}
}
