package library

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
)

// Applying a resource to an agent puts it where the agent reads it:
//
//   - a skill as a directory of the agent's skills;
//   - a rule, for Claude Code, as a file of ~/.claude/rules;
//   - anything else as a block, between markers, of the agent's
//     instructions file: ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md or
//     ~/.pi/agent/AGENTS.md. The rest of the file is left as it is.
//
// Skills and Claude Code rules are symbolic links to the library, or copies
// that name their source in a marker.

const copyMarker = ".distill-source"

type target struct {
	path  string
	block bool // a block in an instructions file
}

func skillsDir(p env.Paths, a agentlog.Agent) string {
	switch a {
	case agentlog.Claude:
		return filepath.Join(p.Home, ".claude", "skills")
	case agentlog.Codex:
		return filepath.Join(p.Home, ".codex", "skills")
	default:
		return filepath.Join(p.Home, ".pi", "agent", "skills")
	}
}

func instructionsFile(p env.Paths, a agentlog.Agent) string {
	switch a {
	case agentlog.Claude:
		return filepath.Join(p.Home, ".claude", "CLAUDE.md")
	case agentlog.Codex:
		return filepath.Join(p.Home, ".codex", "AGENTS.md")
	default:
		return filepath.Join(p.Home, ".pi", "agent", "AGENTS.md")
	}
}

func targetOf(p env.Paths, it Item, a agentlog.Agent) target {
	switch {
	case it.Kind == KindSkill:
		return target{path: filepath.Join(skillsDir(p, a), it.Name)}
	case it.Kind == KindRule && a == agentlog.Claude:
		return target{path: filepath.Join(p.Home, ".claude", "rules", it.Name+".md")}
	}
	return target{path: instructionsFile(p, a), block: true}
}

type ApplyStatus struct {
	Applied  bool
	Linked   bool   // a symbolic link, which follows the library
	Conflict string // something else is where the resource would go
}

func Status(p env.Paths, it Item, a agentlog.Agent) ApplyStatus {
	if !it.Kind.Appliable() {
		return ApplyStatus{}
	}
	t := targetOf(p, it, a)
	if t.block {
		data, err := os.ReadFile(t.path)
		return ApplyStatus{Applied: err == nil && bytes.Contains(data, []byte(blockBegin(it)))}
	}
	st, err := os.Lstat(t.path)
	if err != nil {
		return ApplyStatus{}
	}
	if st.Mode()&os.ModeSymlink != 0 {
		if dest, err := os.Readlink(t.path); err == nil && dest == it.Root {
			return ApplyStatus{Applied: true, Linked: true}
		}
		return ApplyStatus{Conflict: t.path}
	}
	if ours(t.path, it) {
		return ApplyStatus{Applied: true}
	}
	return ApplyStatus{Conflict: t.path}
}

// ours reports whether a copy at path came from the resource.
func ours(path string, it Item) bool {
	if it.Kind == KindSkill {
		data, err := os.ReadFile(filepath.Join(path, copyMarker))
		return err == nil && strings.TrimSpace(string(data)) == it.Root
	}
	data, err := os.ReadFile(path)
	return err == nil && bytes.Contains(data, []byte(fileMarker(it)))
}

func fileMarker(it Item) string { return "<!-- distill:source " + it.Root + " -->" }
func blockBegin(it Item) string { return "<!-- distill:begin " + it.ID() + " -->" }
func blockEnd(it Item) string   { return "<!-- distill:end " + it.ID() + " -->" }

func Apply(p env.Paths, it Item, a agentlog.Agent, copyMode bool) error {
	if !it.Kind.Appliable() {
		return fmt.Errorf("%s 不能应用到 Agent", it.Kind.Label())
	}
	t := targetOf(p, it, a)
	if t.block {
		data, err := os.ReadFile(it.File)
		if err != nil {
			return err
		}
		_, body := FrontMatter(string(data))
		return writeBlock(t.path, it, strings.TrimSpace(body))
	}
	st := Status(p, it, a)
	if st.Conflict != "" {
		return fmt.Errorf("%s 已存在，且不是 Distill 放的", st.Conflict)
	}
	if st.Applied {
		if err := os.RemoveAll(t.path); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(t.path), 0o755); err != nil {
		return err
	}
	if !copyMode {
		return os.Symlink(it.Root, t.path)
	}
	if it.Kind == KindSkill {
		if err := copyDir(it.Root, t.path); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(t.path, copyMarker), []byte(it.Root+"\n"), 0o644)
	}
	data, err := os.ReadFile(it.File)
	if err != nil {
		return err
	}
	data = append(bytes.TrimRight(data, "\n"), []byte("\n\n"+fileMarker(it)+"\n")...)
	return os.WriteFile(t.path, data, 0o644)
}

func Unapply(p env.Paths, it Item, a agentlog.Agent) error {
	if !it.Kind.Appliable() {
		return nil
	}
	t := targetOf(p, it, a)
	if t.block {
		return writeBlock(t.path, it, "")
	}
	if st := Status(p, it, a); !st.Applied {
		return nil
	}
	return os.RemoveAll(t.path)
}

// writeBlock puts the resource's block in an instructions file, replacing
// the one there, or takes it out when body is empty. The file is backed up
// once, before Distill first changes it.
func writeBlock(path string, it Item, body string) error {
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err == nil {
		bak := path + ".distill-bak"
		if _, err := os.Stat(bak); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(bak, data, 0o644); err != nil {
				return err
			}
		}
	}
	s := string(data)
	begin, end := blockBegin(it), blockEnd(it)
	if i := strings.Index(s, begin); i >= 0 {
		if j := strings.Index(s[i:], end); j >= 0 {
			rest := strings.TrimLeft(s[i+j+len(end):], "\n")
			head := strings.TrimRight(s[:i], "\n")
			if head != "" && rest != "" {
				head += "\n\n"
			}
			s = head + rest
		}
	}
	if body != "" {
		s = strings.TrimRight(s, "\n")
		if s != "" {
			s += "\n\n"
		}
		s += begin + "\n" + body + "\n" + end
	}
	if s != "" {
		s = strings.TrimRight(s, "\n") + "\n"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".distill-tmp"
	if err := os.WriteFile(tmp, []byte(s), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func copyDir(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		dst := filepath.Join(to, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		out, err := os.Create(dst)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, src); err != nil {
			out.Close()
			return err
		}
		return out.Close()
	})
}
