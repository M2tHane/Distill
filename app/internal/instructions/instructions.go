// Package instructions finds the instruction files that applied to the
// sessions being distilled (CLAUDE.md, AGENTS.md and the like), and copies
// them where the distilling agent reads them, with what tells whether they
// are out of date: when they last changed, what the project did since, and
// the paths they name that are gone.
package instructions

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Names are the instruction files an agent reads in a project directory.
var Names = []string{"CLAUDE.md", "CLAUDE.local.md", ".claude/CLAUDE.md", "AGENTS.md", "AGENTS.override.md"}

// UserFiles are the instruction files of the user, which apply to every
// project, relative to the home directory.
var UserFiles = []string{".claude/CLAUDE.md", ".codex/AGENTS.md", ".pi/agent/AGENTS.md"}

// File is an instruction file and what is known of its age.
type File struct {
	Path     string
	Project  string // the project it applies to; "" for the user's files
	Sessions int    // sessions referenced in the project
	ModTime  time.Time
	Git      *GitInfo // nil when the file is not in a repository's history
	Missing  []string // paths the file names that do not exist
}

// GitInfo is the history of a file and of its project since.
type GitInfo struct {
	Root       string
	LastCommit time.Time // when the file last changed in a commit
	Dirty      bool      // changed since that commit
	Since      int       // the project's commits after it
	Log        []string  // the latest of those, one line each
}

// Find returns the instruction files of the projects of dirs, each the
// working directory of a session, and the user's. A project's files are
// those of its directory and of the directories above it up to the root of
// its repository.
func Find(home string, dirs []string) []File {
	var files []File
	seen := map[string]bool{}
	add := func(path, project string, sessions int) {
		if seen[path] {
			for i := range files {
				if files[i].Path == path {
					files[i].Sessions += sessions
				}
			}
			return
		}
		st, err := os.Stat(path)
		if err != nil || st.IsDir() {
			return
		}
		seen[path] = true
		files = append(files, File{Path: path, Project: project, Sessions: sessions, ModTime: st.ModTime()})
	}
	count := map[string]int{}
	var order []string
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if count[d] == 0 {
			order = append(order, d)
		}
		count[d]++
	}
	for _, d := range order {
		root := gitRoot(d)
		for dir := d; ; dir = filepath.Dir(dir) {
			if dir == home || dir == filepath.Dir(dir) {
				break
			}
			for _, name := range Names {
				add(filepath.Join(dir, name), d, count[d])
			}
			if root == "" || dir == root {
				break
			}
		}
	}
	for _, rel := range UserFiles {
		add(filepath.Join(home, rel), "", 0)
	}
	for i := range files {
		f := &files[i]
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		root := gitRoot(filepath.Dir(f.Path))
		f.Missing = missingPaths(string(data), filepath.Dir(f.Path), root, home, trackedNames(root))
		f.Git = history(f.Path)
	}
	return files
}

func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...).Output()
	return strings.TrimSpace(string(out)), err
}

func gitRoot(dir string) string {
	if _, err := os.Stat(dir); err != nil {
		return ""
	}
	root, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return root
}

// logLines is how many of the commits since a file changed are listed.
const logLines = 20

func history(path string) *GitInfo {
	dir := filepath.Dir(path)
	root := gitRoot(dir)
	if root == "" {
		return nil
	}
	last, err := git(dir, "log", "-1", "--format=%H %cI", "--", path)
	if err != nil || last == "" {
		return nil
	}
	hash, date, _ := strings.Cut(last, " ")
	g := &GitInfo{Root: root}
	g.LastCommit, _ = time.Parse(time.RFC3339, date)
	if n, err := git(dir, "rev-list", "--count", hash+"..HEAD"); err == nil {
		g.Since, _ = strconv.Atoi(n)
	}
	if g.Since > 0 {
		if out, err := git(dir, "log", "--format=%cs %s", "-n", strconv.Itoa(logLines), hash+"..HEAD"); err == nil && out != "" {
			g.Log = strings.Split(out, "\n")
		}
	}
	if out, err := git(dir, "status", "--porcelain", "--", path); err == nil && out != "" {
		g.Dirty = true
	}
	return g
}

var (
	// A path is named in backquotes or as a link's target.
	codeRe = regexp.MustCompile("`([^`\\s]+)`")
	linkRe = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	extRe  = regexp.MustCompile(`\.[A-Za-z][A-Za-z0-9]{0,5}$`)
)

// trackedNames are the base names of the files in a repository, by which
// a file named without its directory is found.
func trackedNames(root string) map[string]bool {
	if root == "" {
		return nil
	}
	out, err := git(root, "ls-files")
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, p := range strings.Split(out, "\n") {
		names[filepath.Base(p)] = true
	}
	return names
}

// sourceExts are the extensions of files a name without a directory is
// taken for.
var sourceExts = map[string]bool{
	".md": true, ".ts": true, ".tsx": true, ".js": true, ".jsx": true, ".mjs": true, ".cjs": true, ".go": true,
	".py": true, ".rs": true, ".java": true, ".kt": true, ".swift": true, ".rb": true, ".php": true, ".c": true,
	".h": true, ".cpp": true, ".sh": true, ".yaml": true, ".yml": true, ".json": true, ".toml": true, ".sql": true,
	".css": true, ".scss": true, ".html": true, ".vue": true, ".svelte": true,
}

// missingPaths returns the paths a file names that exist neither from its
// directory, nor from its repository's root, nor from the home directory.
// A name without a directory, as config.ts, is missing only when no file
// of the repository has it.
func missingPaths(text, dir, root, home string, tracked map[string]bool) []string {
	var missing []string
	seen := map[string]bool{}
	var names []string
	for _, m := range codeRe.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
	}
	for _, m := range linkRe.FindAllStringSubmatch(text, -1) {
		names = append(names, m[1])
	}
	for _, name := range names {
		p := strings.TrimSuffix(strings.TrimPrefix(name, "./"), "/")
		if !looksLikePath(p) || seen[p] {
			continue
		}
		// An absolute path outside the home directory is more often a
		// route, as /api/users, than a file.
		if filepath.IsAbs(p) && !strings.HasPrefix(p, home+"/") {
			continue
		}
		seen[p] = true
		if exists(p, dir, root, home) {
			continue
		}
		if !strings.Contains(p, "/") && (tracked == nil || tracked[p] || !sourceExts[filepath.Ext(p)]) {
			continue
		}
		// Without an extension, a name with a slash may be no path at all,
		// as owner/repo or application/json: it is a missing path only
		// where its first directory is there.
		if !extRe.MatchString(p) && !filepath.IsAbs(p) && !strings.HasPrefix(p, "~/") {
			first, _, _ := strings.Cut(p, "/")
			if !exists(first, dir, root, home) {
				continue
			}
		}
		missing = append(missing, name)
	}
	sort.Strings(missing)
	return missing
}

func looksLikePath(p string) bool {
	if p == "" || strings.Contains(p, "://") || strings.HasPrefix(p, "#") || strings.HasPrefix(p, "-") ||
		strings.HasPrefix(p, "$") || strings.HasPrefix(p, "@") || strings.ContainsAny(p, "*?<>{}[]|=$\"'(),;") ||
		strings.Contains(p, "...") || strings.Contains(p, "…") {
		return false
	}
	// A path has a directory in it, or an extension; dotted names that are
	// not files, as versions or Go identifiers, have neither a slash nor a
	// lower-case extension. A first directory with a dot is a domain, as
	// in github.com/x/y.
	if first, _, ok := strings.Cut(p, "/"); ok {
		return !strings.Contains(first, ".") || strings.HasPrefix(first, ".") || first == "~"
	}
	return extRe.MatchString(p) && strings.ToLower(filepath.Ext(p)) == filepath.Ext(p) && !strings.Contains(strings.TrimSuffix(p, filepath.Ext(p)), ".")
}

func exists(p, dir, root, home string) bool {
	var candidates []string
	switch {
	case strings.HasPrefix(p, "~/"):
		candidates = []string{filepath.Join(home, p[2:])}
	case filepath.IsAbs(p):
		candidates = []string{p}
	default:
		candidates = []string{filepath.Join(dir, p)}
		if root != "" {
			candidates = append(candidates, filepath.Join(root, p))
		}
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return true
		}
	}
	return false
}

// Snapshot copies the files into dir, with a README.md that says where
// each came from and how old it is. It returns the README's path, or ""
// when there are no files.
func Snapshot(dir string, files []File, now time.Time) (string, error) {
	if len(files) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# 指令文件快照\n\n")
	fmt.Fprintf(&b, "导出于 %s。下面是这些会话所在项目的指令文件，以及用户级的指令文件（对所有项目生效）。副本在本目录，原文件不要改。\n", now.Format("2006-01-02 15:04"))
	for i, f := range files {
		name := fmt.Sprintf("%02d-%s", i+1, copyName(f))
		data, err := os.ReadFile(f.Path)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "\n## %s\n\n", name)
		fmt.Fprintf(&b, "- 原文件：`%s`\n", f.Path)
		if f.Project == "" {
			b.WriteString("- 适用于：用户的所有项目\n")
		} else {
			fmt.Fprintf(&b, "- 适用于：项目 `%s`（引用了其中 %d 个会话）\n", f.Project, f.Sessions)
		}
		fmt.Fprintf(&b, "- 文件最后修改：%s（%s）\n", f.ModTime.Format("2006-01-02"), age(f.ModTime, now))
		if g := f.Git; g != nil {
			fmt.Fprintf(&b, "- git 中最后一次修改它的提交：%s（%s）；之后项目又有 %d 次提交", g.LastCommit.Format("2006-01-02"), age(g.LastCommit, now), g.Since)
			if g.Dirty {
				b.WriteString("；工作区中它还有未提交的改动")
			}
			b.WriteString("\n")
			if len(g.Log) > 0 {
				fmt.Fprintf(&b, "- 之后的提交（最近 %d 条）：\n", len(g.Log))
				for _, l := range g.Log {
					fmt.Fprintf(&b, "  - %s\n", l)
				}
			}
		}
		if len(f.Missing) > 0 {
			b.WriteString("- 文中提到、但现在找不到的路径：")
			for j, m := range f.Missing {
				if j > 0 {
					b.WriteString("、")
				}
				fmt.Fprintf(&b, "`%s`", m)
			}
			b.WriteString("\n")
		}
	}
	readme := filepath.Join(dir, "README.md")
	return readme, os.WriteFile(readme, []byte(b.String()), 0o644)
}

// copyName names a file's copy after its project and itself:
// AskData-CLAUDE.md, user-codex-AGENTS.md.
func copyName(f File) string {
	base := strings.ReplaceAll(filepath.Base(f.Path), string(filepath.Separator), "-")
	if strings.Contains(f.Path, string(filepath.Separator)+".claude"+string(filepath.Separator)) && f.Project != "" {
		base = "dot-claude-" + base
	}
	if f.Project == "" {
		// ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md, ~/.pi/agent/AGENTS.md
		agent := filepath.Base(filepath.Dir(f.Path))
		if agent == "agent" {
			agent = filepath.Base(filepath.Dir(filepath.Dir(f.Path)))
		}
		return "user-" + strings.TrimPrefix(agent, ".") + "-" + base
	}
	rel := filepath.Base(f.Project)
	if d := filepath.Dir(f.Path); d != f.Project && filepath.Base(d) != ".claude" {
		rel = filepath.Base(d)
	}
	return rel + "-" + base
}

func age(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < 24*time.Hour:
		return "今天"
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%d 天前", int(d.Hours()/24))
	}
	return fmt.Sprintf("约 %d 个月前", int(d.Hours()/24/30))
}
