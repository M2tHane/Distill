package agentlog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Trash keeps deleted sessions so they can be restored to where they were.
// Each item is a directory under Dir holding the moved files and item.json.
// Sources give the files that go with a session's log (see Relater).
type Trash struct {
	Dir     string
	Sources []Source
}

// TrashItem is a session in the trash.
type TrashItem struct {
	ID        string
	Session   Session
	Files     []TrashFile
	DeletedAt time.Time
}

// TrashFile is a file or directory moved to the trash, and where it was.
type TrashFile struct {
	Original string
	Stored   string // relative to the item's directory
}

// Move moves a session's log, and the files that belong to it, to the trash.
func (t Trash) Move(s Session) (TrashItem, error) {
	now := time.Now()
	item := TrashItem{
		ID:        fmt.Sprintf("%s-%s-%d", s.Agent, sanitize(s.ID), now.UnixNano()),
		Session:   s,
		DeletedAt: now,
	}
	dir := filepath.Join(t.Dir, item.ID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return item, err
	}
	paths := []string{s.Path}
	if r, ok := SourceFor(t.Sources, s.Agent).(Relater); ok {
		paths = append(paths, r.Related(s.Path)...)
	}
	for i, p := range paths {
		stored := fmt.Sprintf("%d-%s", i, filepath.Base(p))
		if err := move(p, filepath.Join(dir, stored)); err != nil {
			// Put back what already moved, so a session is never half deleted.
			for _, f := range item.Files {
				move(filepath.Join(dir, f.Stored), f.Original)
			}
			os.RemoveAll(dir)
			return item, err
		}
		item.Files = append(item.Files, TrashFile{Original: p, Stored: stored})
	}
	data, _ := json.MarshalIndent(item, "", "  ")
	return item, os.WriteFile(filepath.Join(dir, "item.json"), data, 0o644)
}

// List returns the items in the trash, the latest deleted first.
func (t Trash) List() ([]TrashItem, error) {
	entries, err := os.ReadDir(t.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []TrashItem
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(t.Dir, e.Name(), "item.json"))
		if err != nil {
			continue
		}
		var it TrashItem
		if json.Unmarshal(data, &it) == nil {
			items = append(items, it)
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].DeletedAt.After(items[j].DeletedAt) })
	return items, nil
}

// Restore moves an item's files back where they were.
func (t Trash) Restore(id string) error {
	dir := filepath.Join(t.Dir, sanitize(id))
	data, err := os.ReadFile(filepath.Join(dir, "item.json"))
	if err != nil {
		return err
	}
	var it TrashItem
	if err := json.Unmarshal(data, &it); err != nil {
		return err
	}
	for _, f := range it.Files {
		if _, err := os.Lstat(f.Original); err == nil {
			return fmt.Errorf("restore %s: a file is already there", f.Original)
		}
	}
	for _, f := range it.Files {
		if err := os.MkdirAll(filepath.Dir(f.Original), 0o755); err != nil {
			return err
		}
		if err := move(filepath.Join(dir, f.Stored), f.Original); err != nil {
			return err
		}
	}
	return os.RemoveAll(dir)
}

// Purge deletes an item for good.
func (t Trash) Purge(id string) error {
	return os.RemoveAll(filepath.Join(t.Dir, sanitize(id)))
}

// PurgeOlder deletes the items deleted before the given age.
func (t Trash) PurgeOlder(age time.Duration) error {
	items, err := t.List()
	if err != nil {
		return err
	}
	for _, it := range items {
		if time.Since(it.DeletedAt) > age {
			if err := t.Purge(it.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

func sanitize(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c == '/' || c == '\\' || c == ':' || c == 0 {
			b[i] = '_'
		}
	}
	if s == "." || s == ".." {
		return "_"
	}
	return string(b)
}

// move renames, or copies and removes across file systems.
func move(from, to string) error {
	err := os.Rename(from, to)
	if err == nil {
		return nil
	}
	var le *os.LinkError
	if !errors.As(err, &le) {
		return err
	}
	if err := copyTree(from, to); err != nil {
		os.RemoveAll(to)
		return err
	}
	return os.RemoveAll(from)
}

func copyTree(from, to string) error {
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
