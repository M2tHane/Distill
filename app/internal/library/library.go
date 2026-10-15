// Package library is the library of resources (skills, rules, AGENTS.md and
// CLAUDE.md pieces) and applying them to agents, and of the reports the
// user reads, which are not applied.
package library

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/M2tHane/Distill/app/internal/env"
)

// Kind is a kind of resource in the library.
type Kind string

const (
	KindSkill  Kind = "skill"
	KindRule   Kind = "rule"
	KindAgents Kind = "agents"
	KindClaude Kind = "claude"
	// KindReport is a document for the user: a review of a project's
	// problems, advice on prompts, a check of instruction files.
	KindReport Kind = "report"
)

var Kinds = []Kind{KindSkill, KindRule, KindAgents, KindClaude, KindReport}

// Appliable reports whether resources of the kind go to agents.
func (k Kind) Appliable() bool { return k != KindReport }

func (k Kind) Label() string {
	switch k {
	case KindSkill:
		return "Skill"
	case KindRule:
		return "Rule"
	case KindAgents:
		return "AGENTS.md"
	case KindClaude:
		return "CLAUDE.md"
	case KindReport:
		return "报告"
	}
	return string(k)
}

// Dir is the kind's directory in the library.
func (k Kind) Dir() string {
	switch k {
	case KindSkill:
		return "skills"
	case KindRule:
		return "rules"
	case KindAgents:
		return "agents-md"
	case KindClaude:
		return "claude-md"
	case KindReport:
		return "reports"
	}
	return string(k)
}

// Item is a resource: a skill directory holding SKILL.md, or a Markdown
// file for the other kinds.
type Item struct {
	Kind        Kind
	Name        string
	Description string
	File        string // the Markdown file
	Root        string // what is linked or copied: the skill's directory, or File
	Builtin     bool
	ModTime     time.Time
}

func (it Item) ID() string { return string(it.Kind) + "/" + it.Name }

// Library lists the resources under its directories.
type Library struct {
	P env.Paths
}

func (l Library) List() []Item {
	var items []Item
	items = append(items, l.Scan(l.P.Builtin, true)...)
	items = append(items, l.Scan(l.P.Library, false)...)
	return items
}

func (l Library) Scan(root string, builtin bool) []Item {
	var items []Item
	for _, k := range Kinds {
		dir := filepath.Join(root, k.Dir())
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if strings.HasPrefix(name, ".") {
				continue
			}
			it := Item{Kind: k, Builtin: builtin}
			if k == KindSkill {
				if !e.IsDir() {
					continue
				}
				it.Name, it.Root = name, filepath.Join(dir, name)
				it.File = filepath.Join(it.Root, "SKILL.md")
			} else {
				if e.IsDir() || !strings.HasSuffix(name, ".md") {
					continue
				}
				it.Name = strings.TrimSuffix(name, ".md")
				it.File = filepath.Join(dir, name)
				it.Root = it.File
			}
			data, err := os.ReadFile(it.File)
			if err != nil {
				continue
			}
			if st, err := os.Stat(it.File); err == nil {
				it.ModTime = st.ModTime()
			}
			meta, body := FrontMatter(string(data))
			it.Description = meta["description"]
			if it.Description == "" {
				it.Description = firstLine(body)
			}
			items = append(items, it)
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].ModTime.After(items[j].ModTime) })
	return items
}

// Signature changes when the library changes, to tell what a run wrote.
func (l Library) Signature() map[string]time.Time {
	sig := map[string]time.Time{}
	for _, it := range l.Scan(l.P.Library, false) {
		sig[it.ID()] = it.ModTime
	}
	return sig
}

func (l Library) Remove(it Item) error {
	if it.Builtin {
		return errors.New("内置资源不能删除")
	}
	for _, a := range env.Agents {
		if st := Status(l.P, it, a); st.Applied {
			if err := Unapply(l.P, it, a); err != nil {
				return err
			}
		}
	}
	return os.RemoveAll(it.Root)
}

// Save writes a resource's file and refreshes the copies applied from it.
func (l Library) Save(it Item, content string, copyMode bool) error {
	if err := os.WriteFile(it.File, []byte(content), 0o644); err != nil {
		return err
	}
	for _, a := range env.Agents {
		st := Status(l.P, it, a)
		if st.Applied && !st.Linked {
			if err := Apply(l.P, it, a, copyMode); err != nil {
				return err
			}
		}
	}
	return nil
}

var frontRe = regexp.MustCompile(`(?s)^---\r?\n(.*?)\r?\n---\r?\n?`)

// FrontMatter splits a Markdown file into its YAML front matter, read as
// one-line keys, and its body.
func FrontMatter(s string) (map[string]string, string) {
	meta := map[string]string{}
	m := frontRe.FindStringSubmatchIndex(s)
	if m == nil {
		return meta, s
	}
	for _, line := range strings.Split(s[m[2]:m[3]], "\n") {
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.HasPrefix(line, " ") {
			continue
		}
		v = strings.TrimSpace(v)
		v = strings.Trim(v, `"'`)
		meta[strings.TrimSpace(k)] = v
	}
	return meta, s[m[1]:]
}

func firstLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#>-* "))
		if line != "" {
			return line
		}
	}
	return ""
}

// Diff names the resources added or changed between two signatures.
func Diff(before, after map[string]time.Time) []string {
	var changed []string
	for id, mod := range after {
		if old, ok := before[id]; !ok || !old.Equal(mod) {
			changed = append(changed, id)
		}
	}
	return changed
}
