package runner

import (
	"encoding/json"

	"github.com/M2tHane/Distill/agentlog/parse"
)

// pi runs Pi: `pi -p` takes the prompt as an argument and writes JSON lines.
type pi struct{}

func (pi) command(r Request) ([]string, string) {
	args := []string{"-p", "--mode", "json", "--session-id", r.SessionID}
	if r.Model != "" {
		args = append(args, "--model", r.Model)
	}
	if r.Effort != "" {
		args = append(args, "--thinking", r.Effort)
	}
	return append(args, "--", r.Prompt), ""
}

func (pi) event(line []byte, emit func(Event)) (string, bool) {
	return "", piEvent(line, emit)
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
