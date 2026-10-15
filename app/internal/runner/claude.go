package runner

import (
	"encoding/json"

	"github.com/M2tHane/Distill/agentlog/parse"
)

// claude runs Claude Code: `claude -p` reads the prompt on stdin and writes
// stream-json.
type claude struct{}

// claudeTools are what Claude Code may do without asking: read anything,
// write and edit files, and a few harmless commands.
const claudeTools = "Read,Write,Edit,MultiEdit,Glob,Grep,LS,Skill,Bash(ls:*),Bash(mkdir:*),Bash(cat:*),Bash(wc:*),Bash(head:*)"

func (claude) command(r Request) ([]string, string) {
	args := []string{"-p", "--output-format", "stream-json", "--verbose",
		"--permission-mode", "acceptEdits", "--allowedTools", claudeTools}
	if r.Resume {
		args = append(args, "--resume", r.SessionID)
	} else {
		args = append(args, "--session-id", r.SessionID)
	}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	if r.Effort != "" {
		args = append(args, "--effort", r.Effort)
	}
	return args, r.Prompt
}

func (claude) event(line []byte, emit func(Event)) (string, bool) {
	return "", claudeEvent(line, emit)
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
