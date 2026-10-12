// Package agentlog reads the local session logs of coding agents and turns
// them into one compact transcript format: what the user typed, what the
// assistant replied and which tools it called. Tool results, reasoning, token
// accounting and injected context are dropped.
//
// Each agent's logs are read by a Source, in a package of its own under
// sources/; sources.All returns them all. Package parse holds what sources
// share. To support another agent, add a package under sources/ and list it
// in sources.All.
package agentlog

import (
	"os"
	"sync"
	"time"
)

// Agent names a coding agent whose sessions can be read.
type Agent string

// The agents of the sources in this module.
const (
	Claude Agent = "claude"
	Codex  Agent = "codex"
	Pi     Agent = "pi"
)

var (
	labelsMu sync.RWMutex
	labels   = map[Agent]string{}
)

// RegisterLabel names an agent for display. Each source registers its own.
func RegisterLabel(a Agent, label string) {
	labelsMu.Lock()
	defer labelsMu.Unlock()
	labels[a] = label
}

// Label is the agent's display name, as its source registered it.
func (a Agent) Label() string {
	labelsMu.RLock()
	defer labelsMu.RUnlock()
	if l, ok := labels[a]; ok {
		return l
	}
	return string(a)
}

// Session describes one session log without its messages.
type Session struct {
	Agent    Agent
	ID       string
	Path     string
	Title    string
	CWD      string
	Models   []string
	Started  time.Time
	Updated  time.Time
	Size     int64
	ModTime  time.Time
	Messages int  // user and assistant messages kept in the transcript
	Archived bool // Codex archived_sessions
}

// Role is who wrote a message.
type Role string

const (
	User      Role = "user"
	Assistant Role = "assistant"
)

// Message is one bubble of the transcript. An assistant message holds its
// text and the tools it called after that text.
type Message struct {
	Role  Role
	Text  string
	Tools []ToolCall
	Time  time.Time
}

// ToolCall is a tool invocation without its result.
type ToolCall struct {
	Name    string // Bash, Edit, exec_command, mcp__server__tool …
	Summary string // one line: the command, the file, the pattern …
	Input   string // the full arguments, for display on demand
}

// Transcript is a session with its messages.
type Transcript struct {
	Session
	Messages []Message
}

// Source reads the sessions of one agent.
type Source interface {
	Agent() Agent
	// Files lists the session logs, newest layout first; order is not significant.
	Files() ([]string, error)
	// Parse reads one session log.
	Parse(path string) (*Transcript, error)
}

// Decorator is a source that refreshes metadata kept outside the session
// file, such as Codex thread names, on sessions served from the cache.
type Decorator interface {
	Decorate(*Session)
}

// Relater is a source whose sessions have files besides their log, such as
// Claude Code's subagent logs, which go with the log to the trash.
type Relater interface {
	Related(path string) []string
}

// SourceFor returns the source of an agent among srcs, or nil.
func SourceFor(srcs []Source, a Agent) Source {
	for _, s := range srcs {
		if s.Agent() == a {
			return s
		}
	}
	return nil
}

// Load parses a session's log with the source of its agent.
func Load(srcs []Source, s Session) (*Transcript, error) {
	src := SourceFor(srcs, s.Agent)
	if src == nil {
		return nil, os.ErrNotExist
	}
	return src.Parse(s.Path)
}
