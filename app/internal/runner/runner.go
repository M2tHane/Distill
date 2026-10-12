// Package runner runs an agent's command (claude, codex or pi) non-interactively
// and reads what it does from its JSON output.
package runner

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/dade/agentlog"
	"github.com/dade/agentlog/parse"
	"github.com/dade/distill/internal/env"
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
	Dir       string
	Prompt    string
	SessionID string
	Resume    bool
}

// Run starts the agent and calls emit, from another goroutine, for what it
// does. It returns the session ID to continue with, which Codex gives only
// as it starts.
func Run(ctx context.Context, r Request, emit func(Event)) (string, error) {
	var args []string
	stdin := r.Prompt
	sessionID := r.SessionID
	switch r.Info.Agent {
	case agentlog.Claude:
		args = []string{"-p", "--output-format", "stream-json", "--verbose",
			"--permission-mode", "acceptEdits",
			"--allowedTools", "Read,Write,Edit,MultiEdit,Glob,Grep,LS,Skill,Bash(ls:*),Bash(mkdir:*),Bash(cat:*),Bash(wc:*),Bash(head:*)"}
		if r.Resume {
			args = append(args, "--resume", sessionID)
		} else {
			args = append(args, "--session-id", sessionID)
		}
		if r.Model != "" {
			args = append(args, "--model", r.Model)
		}
	case agentlog.Codex:
		args = []string{"exec"}
		if r.Resume {
			args = append(args, "resume")
		}
		args = append(args, "--json", "--skip-git-repo-check", "-c", `sandbox_mode="workspace-write"`)
		if !r.Resume {
			args = append(args, "-C", r.Dir)
		}
		if r.Model != "" {
			args = append(args, "-m", r.Model)
		}
		if r.Resume {
			args = append(args, sessionID)
		}
		args = append(args, "-")
	case agentlog.Pi:
		args = []string{"-p", "--mode", "json", "--session-id", sessionID}
		if r.Model != "" {
			args = append(args, "--model", r.Model)
		}
		args = append(args, "--", r.Prompt)
		stdin = ""
	}

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
		var id string
		var bad bool
		switch r.Info.Agent {
		case agentlog.Claude:
			bad = claudeEvent(line, emit)
		case agentlog.Codex:
			id, bad = codexEvent(line, emit)
		case agentlog.Pi:
			bad = piEvent(line, emit)
		}
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

func claudeEvent(line []byte, emit func(Event)) (failed bool) {
	var ev struct {
		Type    string `json:"type"`
		IsError bool   `json:"is_error"`
		Result  string `json:"result"`
		Message struct {
			Content []struct {
				Type  string          `json:"type"`
				Text  string          `json:"text"`
				Name  string          `json:"name"`
				Input json.RawMessage `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &ev) != nil {
		return false
	}
	switch ev.Type {
	case "assistant":
		for _, c := range ev.Message.Content {
			switch c.Type {
			case "text":
				emit(Event{Text: c.Text})
			case "tool_use":
				tc := parse.NewToolCall(c.Name, c.Input)
				emit(Event{Tool: &tc})
			}
		}
	case "result":
		if ev.IsError {
			emit(Event{Err: ev.Result})
			return true
		}
	}
	return false
}

func codexEvent(line []byte, emit func(Event)) (id string, failed bool) {
	var ev struct {
		Type     string `json:"type"`
		ThreadID string `json:"thread_id"`
		Error    struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
		Item    struct {
			Type    string `json:"type"`
			Text    string `json:"text"`
			Message string `json:"message"`
			Command string `json:"command"`
			Server  string `json:"server"`
			Tool    string `json:"tool"`
			Query   string `json:"query"`
			Changes []struct {
				Path string `json:"path"`
			} `json:"changes"`
		} `json:"item"`
	}
	if json.Unmarshal(line, &ev) != nil {
		return "", false
	}
	switch ev.Type {
	case "thread.started":
		return ev.ThreadID, false
	case "item.completed":
		it := ev.Item
		switch it.Type {
		case "agent_message":
			emit(Event{Text: it.Text})
		case "command_execution":
			emit(Event{Tool: &agentlog.ToolCall{Name: "shell", Summary: oneLine(it.Command), Input: it.Command}})
		case "file_change":
			var files []string
			for _, ch := range it.Changes {
				files = append(files, ch.Path)
			}
			s := strings.Join(files, "\n")
			emit(Event{Tool: &agentlog.ToolCall{Name: "apply_patch", Summary: oneLine(strings.Join(files, ", ")), Input: s}})
		case "mcp_tool_call":
			emit(Event{Tool: &agentlog.ToolCall{Name: it.Server + "." + it.Tool, Summary: it.Tool}})
		case "web_search":
			emit(Event{Tool: &agentlog.ToolCall{Name: "web_search", Summary: it.Query}})
		case "error":
			// Warnings about the user's configuration are not failures.
			if !strings.Contains(it.Message, "ignoring") {
				emit(Event{Err: it.Message})
			}
		}
	case "turn.failed":
		emit(Event{Err: ev.Error.Message})
		return "", true
	case "error":
		emit(Event{Err: ev.Message})
		return "", true
	}
	return "", false
}

func piEvent(line []byte, emit func(Event)) (failed bool) {
	var ev struct {
		Type    string `json:"type"`
		Message struct {
			Role         string `json:"role"`
			StopReason   string `json:"stopReason"`
			ErrorMessage string `json:"errorMessage"`
			Content      []struct {
				Type      string          `json:"type"`
				Text      string          `json:"text"`
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &ev) != nil || ev.Type != "message_end" || ev.Message.Role != "assistant" {
		return false
	}
	for _, c := range ev.Message.Content {
		switch c.Type {
		case "text":
			emit(Event{Text: c.Text})
		case "toolCall":
			tc := parse.NewToolCall(c.Name, c.Arguments)
			emit(Event{Tool: &tc})
		}
	}
	if ev.Message.StopReason == "error" || ev.Message.ErrorMessage != "" {
		emit(Event{Err: ev.Message.ErrorMessage})
		return true
	}
	return false
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
