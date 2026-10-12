package library

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
)

func testLibrary(t *testing.T) (env.Paths, Library) {
	t.Helper()
	home := t.TempDir()
	root := filepath.Join(home, ".distill")
	p := env.Paths{Home: home, Root: root, Library: filepath.Join(root, "library"), Builtin: filepath.Join(root, "builtin")}
	write := func(rel, s string) {
		path := filepath.Join(p.Library, rel)
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("skills/deploy/SKILL.md", "---\nname: deploy\ndescription: Deploy the app.\n---\n\nSteps.\n")
	write("rules/git.md", "---\nname: git\ndescription: Git rules.\n---\n\n- Branch first.\n")
	write("agents-md/style.md", "---\nname: style\ndescription: Style.\n---\n\n- Be brief.\n")
	return p, Library{P: p}
}

func find(t *testing.T, l Library, id string) Item {
	t.Helper()
	for _, it := range l.List() {
		if it.ID() == id {
			return it
		}
	}
	t.Fatalf("no %s", id)
	return Item{}
}

func TestLibraryList(t *testing.T) {
	_, l := testLibrary(t)
	it := find(t, l, "skill/deploy")
	if it.Description != "Deploy the app." {
		t.Errorf("description %q", it.Description)
	}
	if len(l.List()) != 3 {
		t.Errorf("got %d items", len(l.List()))
	}
}

func TestApplySkillLink(t *testing.T) {
	p, l := testLibrary(t)
	it := find(t, l, "skill/deploy")
	if err := Apply(p, it, agentlog.Claude, false); err != nil {
		t.Fatal(err)
	}
	st := Status(p, it, agentlog.Claude)
	if !st.Applied || !st.Linked {
		t.Fatalf("status %+v", st)
	}
	if _, err := os.Stat(filepath.Join(p.Home, ".claude", "skills", "deploy", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if err := Unapply(p, it, agentlog.Claude); err != nil {
		t.Fatal(err)
	}
	if Status(p, it, agentlog.Claude).Applied {
		t.Fatal("still applied")
	}
}

func TestApplySkillCopyAndConflict(t *testing.T) {
	p, l := testLibrary(t)
	it := find(t, l, "skill/deploy")
	if err := Apply(p, it, agentlog.Codex, true); err != nil {
		t.Fatal(err)
	}
	if st := Status(p, it, agentlog.Codex); !st.Applied || st.Linked {
		t.Fatalf("status %+v", st)
	}
	// Saving refreshes the copy.
	if err := l.Save(it, "---\nname: deploy\ndescription: New.\n---\n\nNew steps.\n", true); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(filepath.Join(p.Home, ".codex", "skills", "deploy", "SKILL.md"))
	if !strings.Contains(string(data), "New steps.") {
		t.Fatalf("copy not refreshed: %s", data)
	}

	// A skill of the user's with the same name is left alone.
	other := filepath.Join(p.Home, ".pi", "agent", "skills", "deploy")
	os.MkdirAll(other, 0o755)
	if err := Apply(p, it, agentlog.Pi, false); err == nil {
		t.Fatal("applied over the user's skill")
	}
	if st := Status(p, it, agentlog.Pi); st.Applied || st.Conflict == "" {
		t.Fatalf("status %+v", st)
	}
}

func TestApplyBlocks(t *testing.T) {
	p, l := testLibrary(t)
	file := filepath.Join(p.Home, ".codex", "AGENTS.md")
	os.MkdirAll(filepath.Dir(file), 0o755)
	os.WriteFile(file, []byte("# Mine\n\nKeep this.\n"), 0o644)

	rule, style := find(t, l, "rule/git"), find(t, l, "agents/style")
	for _, it := range []Item{rule, style} {
		if err := Apply(p, it, agentlog.Codex, false); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(file)
	s := string(data)
	for _, want := range []string{"# Mine\n\nKeep this.", "- Branch first.", "- Be brief.", blockBegin(rule), blockEnd(style)} {
		if !strings.Contains(s, want) {
			t.Errorf("AGENTS.md lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "name: git") {
		t.Errorf("front matter written:\n%s", s)
	}
	if _, err := os.Stat(file + ".distill-bak"); err != nil {
		t.Error("no backup")
	}
	// Applying again replaces the block instead of adding one.
	Apply(p, rule, agentlog.Codex, false)
	data, _ = os.ReadFile(file)
	if strings.Count(string(data), blockBegin(rule)) != 1 {
		t.Errorf("block twice:\n%s", data)
	}

	for _, it := range []Item{rule, style} {
		if err := Unapply(p, it, agentlog.Codex); err != nil {
			t.Fatal(err)
		}
	}
	data, _ = os.ReadFile(file)
	if string(data) != "# Mine\n\nKeep this.\n" {
		t.Errorf("not restored: %q", data)
	}

	// A rule is a file of its own for Claude Code.
	if err := Apply(p, rule, agentlog.Claude, false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(p.Home, ".claude", "rules", "git.md")); err != nil {
		t.Fatal(err)
	}
}
