package runner

import (
	"encoding/json"
	"strings"

	"github.com/M2tHane/Distill/agentlog"
)

// codex runs Codex: `codex exec` reads the prompt on stdin and writes JSON
// lines. Its session, a thread, gets its ID only as it starts.
type codex struct{}

func (codex) command(r Request) ([]string, string) {
	args := []string{"exec"}
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
	if r.Effort != "" {
		args = append(args, "-c", `model_reasoning_effort="`+r.Effort+`"`)
	}
	if r.Resume {
		args = append(args, r.SessionID)
	}
	return append(args, "-"), r.Prompt
}

func (codex) event(line []byte, emit func(Event)) (string, bool) {
	return codexEvent(line, emit)
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
