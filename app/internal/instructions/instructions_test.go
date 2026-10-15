package instructions

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFindAndSnapshot(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	home := t.TempDir()
	repo := filepath.Join(home, "proj")
	sub := filepath.Join(repo, "web")
	write := func(path, s string) {
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	write(filepath.Join(repo, "CLAUDE.md"), "# 项目\n\n- 用 `npm run dev` 启动，入口是 `src/main.ts`，旧代码在 `src/legacy/` 和 `scripts/old.sh`。\n- 依赖 `github.com/a/b`，接口 `/api/users`，类型 `application/json`，见 [文档](docs/arch.md)。\n- 版本 `v1.2.3`，函数 `fmt.Println`。\n")
	write(filepath.Join(repo, "src", "main.ts"), "")
	write(filepath.Join(repo, "docs", "arch.md"), "")
	write(filepath.Join(sub, "AGENTS.md"), "- 前端在 `web/`。\n")
	write(filepath.Join(home, ".codex", "AGENTS.md"), "- 中文回复。\n")
	run("init", "-q")
	run("add", ".")
	run("commit", "-q", "-m", "init")
	write(filepath.Join(repo, "src", "x.ts"), "")
	run("add", ".")
	run("commit", "-q", "-m", "add x")

	files := Find(home, []string{sub, sub, repo})
	var names []string
	for _, f := range files {
		names = append(names, strings.TrimPrefix(f.Path, home+"/"))
	}
	if got := strings.Join(names, " "); got != "proj/web/AGENTS.md proj/CLAUDE.md .codex/AGENTS.md" {
		t.Fatalf("files: %s", got)
	}
	claude := files[1]
	if claude.Sessions != 3 {
		t.Errorf("sessions: %d", claude.Sessions)
	}
	if got := strings.Join(claude.Missing, " "); got != "scripts/old.sh src/legacy/" {
		t.Errorf("missing: %q", got)
	}
	if g := claude.Git; g == nil || g.Since != 1 || len(g.Log) != 1 || !strings.HasSuffix(g.Log[0], "add x") || g.Dirty {
		t.Errorf("git: %+v", claude.Git)
	}
	if files[2].Git != nil || files[2].Project != "" {
		t.Errorf("user file: %+v", files[2])
	}

	dir := filepath.Join(t.TempDir(), "instructions")
	readme, err := Snapshot(dir, files, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(readme)
	for _, want := range []string{"## 01-web-AGENTS.md", "## 02-proj-CLAUDE.md", "## 03-user-codex-AGENTS.md", "之后项目又有 1 次提交", "add x", "`scripts/old.sh`、`src/legacy/`", "用户的所有项目"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("README lacks %q:\n%s", want, data)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "02-proj-CLAUDE.md")); err != nil {
		t.Error(err)
	}
}
