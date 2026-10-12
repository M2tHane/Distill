// Distill reads the sessions of local coding agents (Claude Code, Codex, Pi),
// lets an agent turn them into skills, rules and instruction files, and
// applies those to the agents.
package main

import (
	_ "embed"

	"github.com/dade/distill/internal/gui"
)

// icon is the app icon; resources/icon.svg is its source.
//
//go:embed resources/icon.png
var icon []byte

func main() {
	gui.Run(icon)
}
