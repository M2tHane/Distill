package agentlog

import (
	"context"
	"encoding/gob"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
)

// cacheVersion changes whenever parsing changes what a Session holds.
const cacheVersion = 4

// Index lists sessions, reparsing only the logs that changed since the last
// scan. Its cache file keeps the sessions between runs.
type Index struct {
	Sources   []Source
	CachePath string // empty: no cache file

	mu      sync.Mutex
	entries map[string]cacheEntry
	loaded  bool
}

type cacheEntry struct {
	Size    int64
	ModTime int64
	Session Session
}

type cacheFile struct {
	Version int
	Entries map[string]cacheEntry
}

// ScanResult is the outcome of a scan.
type ScanResult struct {
	Sessions []Session // newest first
	Errors   []error   // logs that could not be read
	Parsed   int       // logs parsed in this scan, not served from the cache
}

// Scan lists the sessions of every source.
func (ix *Index) Scan(ctx context.Context) (ScanResult, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if !ix.loaded {
		ix.load()
		ix.loaded = true
	}

	type job struct {
		src  Source
		path string
		st   os.FileInfo
	}
	var jobs []job
	var res ScanResult
	seen := map[string]bool{}
	for _, src := range ix.Sources {
		files, err := src.Files()
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				res.Errors = append(res.Errors, err)
			}
			continue
		}
		for _, p := range files {
			st, err := os.Stat(p)
			if err != nil {
				continue
			}
			seen[p] = true
			if e, ok := ix.entries[p]; ok && e.Size == st.Size() && e.ModTime == st.ModTime().UnixNano() {
				s := e.Session
				if d, ok := src.(Decorator); ok {
					d.Decorate(&s)
				}
				res.Sessions = append(res.Sessions, s)
				continue
			}
			jobs = append(jobs, job{src, p, st})
		}
	}

	var mu sync.Mutex
	var wg sync.WaitGroup
	ch := make(chan job)
	for range runtime.GOMAXPROCS(0) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range ch {
				var t *Transcript
				var err error
				if sm, ok := j.src.(Summarizer); ok {
					t, err = sm.Summarize(j.path)
				} else {
					t, err = j.src.Parse(j.path)
				}
				mu.Lock()
				if err != nil {
					res.Errors = append(res.Errors, err)
				} else {
					s := t.Session
					ix.entries[j.path] = cacheEntry{j.st.Size(), j.st.ModTime().UnixNano(), s}
					res.Sessions = append(res.Sessions, s)
					res.Parsed++
				}
				mu.Unlock()
			}
		}()
	}
	for _, j := range jobs {
		if ctx.Err() != nil {
			break
		}
		ch <- j
	}
	close(ch)
	wg.Wait()

	for p := range ix.entries {
		if !seen[p] {
			delete(ix.entries, p)
		}
	}
	sort.Slice(res.Sessions, func(i, j int) bool {
		return res.Sessions[i].Updated.After(res.Sessions[j].Updated)
	})
	if res.Parsed > 0 || len(ix.entries) != len(seen) {
		ix.save()
	}
	return res, ctx.Err()
}

// Forget drops a log from the cache, as after it is moved to the trash.
func (ix *Index) Forget(path string) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	delete(ix.entries, path)
}

func (ix *Index) load() {
	ix.entries = map[string]cacheEntry{}
	if ix.CachePath == "" {
		return
	}
	f, err := os.Open(ix.CachePath)
	if err != nil {
		return
	}
	defer f.Close()
	var c cacheFile
	if gob.NewDecoder(f).Decode(&c) == nil && c.Version == cacheVersion && c.Entries != nil {
		ix.entries = c.Entries
	}
}

func (ix *Index) save() {
	if ix.CachePath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(ix.CachePath), 0o755); err != nil {
		return
	}
	tmp := ix.CachePath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return
	}
	err = gob.NewEncoder(f).Encode(cacheFile{cacheVersion, ix.entries})
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return
	}
	os.Rename(tmp, ix.CachePath)
}
