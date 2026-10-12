package sources_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/sources"
)

const home = "../testdata/home"

func scan(t *testing.T, cache string) agentlog.ScanResult {
	t.Helper()
	ix := &agentlog.Index{Sources: sources.All(home), CachePath: cache}
	res, err := ix.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Errors) > 0 {
		t.Fatal(res.Errors)
	}
	return res
}

func byAgent(t *testing.T, res agentlog.ScanResult, a agentlog.Agent) agentlog.Session {
	t.Helper()
	for _, s := range res.Sessions {
		if s.Agent == a {
			return s
		}
	}
	t.Fatalf("no %s session", a)
	return agentlog.Session{}
}

func load(t *testing.T, s agentlog.Session) *agentlog.Transcript {
	t.Helper()
	tr, err := agentlog.Load(sources.All(home), s)
	if err != nil {
		t.Fatal(err)
	}
	return tr
}

type want struct {
	role  agentlog.Role
	text  string
	tools []string // summaries
}

func check(t *testing.T, tr *agentlog.Transcript, wants []want) {
	t.Helper()
	if len(tr.Messages) != len(wants) {
		for _, m := range tr.Messages {
			t.Logf("%s %q %v", m.Role, m.Text, m.Tools)
		}
		t.Fatalf("got %d messages, want %d", len(tr.Messages), len(wants))
	}
	for i, w := range wants {
		m := tr.Messages[i]
		if m.Role != w.role || m.Text != w.text {
			t.Errorf("message %d: got %s %q, want %s %q", i, m.Role, m.Text, w.role, w.text)
		}
		var got []string
		for _, tc := range m.Tools {
			got = append(got, tc.Summary)
		}
		if strings.Join(got, "|") != strings.Join(w.tools, "|") {
			t.Errorf("message %d tools: got %q, want %q", i, got, w.tools)
		}
	}
}

func TestClaude(t *testing.T) {
	s := byAgent(t, scan(t, ""), agentlog.Claude)
	if s.ID != "s1" || s.Title != "修复登录问题" || s.CWD != "/tmp/proj" || s.Messages != 4 {
		t.Fatalf("session: %+v", s)
	}
	if len(s.Models) != 1 || s.Models[0] != "claude-opus-5-5" {
		t.Errorf("models: %v", s.Models)
	}
	check(t, load(t, s), []want{
		{agentlog.User, "/model opus", nil},
		{agentlog.User, "修复登录 bug", nil},
		{agentlog.Assistant, "我先看看代码。", []string{"git status", "/tmp/proj/a.go"}},
		{agentlog.Assistant, "已修复。", nil},
	})
}

func TestCodex(t *testing.T) {
	s := byAgent(t, scan(t, ""), agentlog.Codex)
	if s.ID != "c1" || s.Title != "跑测试" || s.CWD != "/tmp/proj" {
		t.Fatalf("session: %+v", s)
	}
	check(t, load(t, s), []want{
		{agentlog.User, "跑一下测试", nil},
		{agentlog.Assistant, "", []string{"go test ./...", "patch /tmp/proj/a.go", "git diff --stat"}},
		{agentlog.Assistant, "测试通过。", nil},
	})
}

func TestPi(t *testing.T) {
	s := byAgent(t, scan(t, ""), agentlog.Pi)
	if s.ID != "p1" || s.Title != "读 README" {
		t.Fatalf("session: %+v", s)
	}
	check(t, load(t, s), []want{
		{agentlog.User, "读一下 README", nil},
		{agentlog.Assistant, "好的。", []string{"/tmp/proj/README.md"}},
		{agentlog.Assistant, "README 说明了安装步骤。", nil},
	})
}

func TestCache(t *testing.T) {
	cache := filepath.Join(t.TempDir(), "index.gob")
	if res := scan(t, cache); res.Parsed != 3 {
		t.Fatalf("first scan parsed %d", res.Parsed)
	}
	res := scan(t, cache)
	if res.Parsed != 0 || len(res.Sessions) != 3 {
		t.Fatalf("cached scan parsed %d of %d", res.Parsed, len(res.Sessions))
	}
}

func TestMarkdown(t *testing.T) {
	tr := load(t, byAgent(t, scan(t, ""), agentlog.Claude))
	var b strings.Builder
	if err := agentlog.WriteMarkdown(&b, tr, agentlog.MarkdownOptions{}); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	for _, s := range []string{"title: \"修复登录问题\"", "## User\n\n修复登录 bug", "- **Bash** `git status`"} {
		if !strings.Contains(md, s) {
			t.Errorf("markdown lacks %q:\n%s", s, md)
		}
	}
	for _, s := range []string{"HUGE OUTPUT", "secret", "sidechain", "ignore me"} {
		if strings.Contains(md, s) {
			t.Errorf("markdown has %q", s)
		}
	}
}

func TestTrash(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "proj", "s.jsonl")
	os.MkdirAll(filepath.Join(dir, "proj", "s", "subagents"), 0o755)
	os.WriteFile(log, []byte("{}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "proj", "s", "subagents", "a.jsonl"), []byte("{}\n"), 0o644)

	tr := agentlog.Trash{Dir: filepath.Join(dir, "trash"), Sources: sources.All(dir)}
	item, err := tr.Move(agentlog.Session{Agent: agentlog.Claude, ID: "s", Path: log})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("log still there")
	}
	if _, err := os.Stat(filepath.Join(dir, "proj", "s")); !os.IsNotExist(err) {
		t.Fatal("session dir still there")
	}
	items, _ := tr.List()
	if len(items) != 1 || len(items[0].Files) != 2 {
		t.Fatalf("items: %+v", items)
	}
	if err := tr.Restore(item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "proj", "s", "subagents", "a.jsonl")); err != nil {
		t.Fatal("not restored:", err)
	}
	if items, _ := tr.List(); len(items) != 0 {
		t.Fatalf("trash not empty: %+v", items)
	}
}
