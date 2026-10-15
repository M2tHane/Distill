package gui

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/builtin"
	"github.com/M2tHane/Distill/app/internal/env"
	"github.com/M2tHane/Distill/app/internal/library"
)

// TestQuickPromptsNameBuiltins checks that every skill the quick prompts and
// the first prompt name is built in.
func TestQuickPromptsNameBuiltins(t *testing.T) {
	re := regexp.MustCompile(`distill-[a-z-]+`)
	prompt := firstPrompt(env.Paths{Root: "/r"}, []string{"runs/1/01.md"}, "runs/1/instructions/README.md", "x")
	var texts []string
	for _, q := range quickPrompts {
		texts = append(texts, q.text)
	}
	for _, name := range re.FindAllString(strings.Join(append(texts, prompt), "\n"), -1) {
		if _, err := builtin.FS.ReadFile("skills/" + name + "/SKILL.md"); err != nil {
			t.Errorf("%s is not built in", name)
		}
	}
	for _, want := range []string{"runs/1/instructions/README.md", "library/agents-md/", "library/reports/"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("the first prompt lacks %q", want)
		}
	}
}

// TestWriteBuiltinAddsNew gives an agent that has one built-in skill the
// others.
func TestWriteBuiltinAddsNew(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(home, ".distill")
	p := env.Paths{Home: home, Root: root, Library: filepath.Join(root, "library"), Builtin: filepath.Join(root, "builtin")}
	a := &app{p: p, lib: library.Library{P: p}, cfg: env.LoadConfig(p)}
	if err := a.writeBuiltin(); err != nil {
		t.Fatal(err)
	}
	builtins := a.lib.Scan(p.Builtin, true)
	if len(builtins) < 6 {
		t.Fatalf("%d built-in skills", len(builtins))
	}
	if err := library.Apply(p, builtins[0], agentlog.Claude, false); err != nil {
		t.Fatal(err)
	}
	if err := a.writeBuiltin(); err != nil {
		t.Fatal(err)
	}
	for _, it := range builtins {
		if !library.Status(p, it, agentlog.Claude).Applied {
			t.Errorf("%s not given to Claude Code", it.Name)
		}
		if library.Status(p, it, agentlog.Codex).Applied {
			t.Errorf("%s given to Codex, which had none", it.Name)
		}
	}
}
