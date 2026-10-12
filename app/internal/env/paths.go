// Package env is where Distill keeps its data, which agents are installed
// on this computer, and what the user chose for them.
package env

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"time"
)

// Paths are where Distill keeps its data, under ~/.distill so the agents it
// runs can read the transcripts and write the library from their working
// directory.
type Paths struct {
	Home    string
	Root    string // ~/.distill: the agents' working directory
	Library string
	Builtin string
	Runs    string
	Trash   string
	Index   string
	Config  string
}

func NewPaths() Paths {
	home, _ := os.UserHomeDir()
	root := filepath.Join(home, ".distill")
	return Paths{
		Home:    home,
		Root:    root,
		Library: filepath.Join(root, "library"),
		Builtin: filepath.Join(root, "builtin"),
		Runs:    filepath.Join(root, "runs"),
		Trash:   filepath.Join(root, "trash"),
		Index:   filepath.Join(root, "index.gob"),
		Config:  filepath.Join(root, "config.json"),
	}
}

// LoginPath gives this process the PATH of the user's login shell. An app
// opened from Finder gets a bare PATH, without Homebrew or nvm, where the
// agents' commands are not found.
func LoginPath() {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, shell, "-l", "-i", "-c", "printf '\\n__PATH__%s__PATH__\\n' \"$PATH\"").Output()
	if err != nil && len(out) == 0 {
		return
	}
	if m := regexp.MustCompile(`__PATH__(.*)__PATH__`).FindSubmatch(out); m != nil && len(m[1]) > 0 {
		os.Setenv("PATH", string(m[1]))
	}
}
