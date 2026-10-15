// Package runner runs an agent's command (claude, codex or pi) non-interactively
// and reads what it does from its JSON output. Each agent has an adapter, in
// its own file, that builds the command and reads its output.
package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
)

// Event is something an agent did while it ran.
type Event struct {
	Text string
	Tool *agentlog.ToolCall
	Err  string
}

// Request runs an agent's command once, non-interactively, in dir.
// A conversation goes on in the same agent session: SessionID names it,
// Resume continues it.
type Request struct {
	Info      env.AgentInfo
	Model     string
	Effort    string // empty: the model's default
	Dir       string
	Prompt    string
	SessionID string
	Resume    bool
}

// adapter is what differs between agents.
type adapter interface {
	// command is the arguments to run the agent with, and its stdin.
	command(r Request) (args []string, stdin string)
	// event reads a JSON line of its output. It returns the session ID if
	// the line gives it, and whether the run failed.
	event(line []byte, emit func(Event)) (id string, failed bool)
}

var adapters = map[agentlog.Agent]adapter{
	agentlog.Claude: claude{},
	agentlog.Codex:  codex{},
	agentlog.Pi:     pi{},
}

// Run starts the agent and calls emit, from another goroutine, for what it
// does. It returns the session ID to continue with, which Codex gives only
// as it starts.
func Run(ctx context.Context, r Request, emit func(Event)) (string, error) {
	sessionID := r.SessionID
	ad := adapters[r.Info.Agent]
	if ad == nil {
		return sessionID, fmt.Errorf("unknown agent %q", r.Info.Agent)
	}
	args, stdin := ad.command(r)

	cmd := exec.CommandContext(ctx, r.Info.Bin, args...)
	cmd.Dir = r.Dir
	cmd.Stdin = strings.NewReader(stdin)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return sessionID, err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitWriter{w: &stderr, n: 8 << 10}
	if err := cmd.Start(); err != nil {
		return sessionID, err
	}

	failed := false
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		id, bad := ad.event(line, emit)
		if id != "" {
			sessionID = id
		}
		failed = failed || bad
	}
	err = cmd.Wait()
	if ctx.Err() != nil {
		return sessionID, ctx.Err()
	}
	if err != nil && !failed {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		emit(Event{Err: msg})
	}
	return sessionID, err
}

func NewUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		return string(r[:199]) + "…"
	}
	return s
}

// limitWriter keeps the first n bytes written to it.
type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}
