// Package builtin holds what Distill installs: the skills an agent uses to
// distill sessions, and the README of the library.
package builtin

import "embed"

//go:embed LIBRARY.md skills
var FS embed.FS
