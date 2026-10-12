package claude

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/parse"
)

// ClaudeSource reads Claude Code sessions: <Root>/<project>/<session>.jsonl.
// Subagent logs, under <Root>/<project>/<session>/subagents/, are not sessions
// of their own and are skipped.
type Source struct{ Root string }

func (s *Source) Agent() agentlog.Agent { return agentlog.Claude }

func (s *Source) Files() ([]string, error) {
	files, err := filepath.Glob(filepath.Join(s.Root, "*", "*.jsonl"))
	if err != nil {
		return nil, err
	}
	return files, nil
}

type ccRecord struct {
	Type             string `json:"type"`
	IsMeta           bool   `json:"isMeta"`
	IsSidechain      bool   `json:"isSidechain"`
	IsCompactSummary bool   `json:"isCompactSummary"`
	IsTranscriptOnly bool   `json:"isVisibleInTranscriptOnly"`
	CWD              string `json:"cwd"`
	SessionID        string `json:"sessionId"`
	Timestamp        string `json:"timestamp"`
	AITitle          string `json:"aiTitle"`
	CustomTitle      string `json:"customTitle"`
	Summary          string `json:"summary"`
	Message          *struct {
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type ccBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

var (
	ccUser      = []byte(`"type":"user"`)
	ccAssistant = []byte(`"type":"assistant"`)
	ccAITitle   = []byte(`"type":"ai-title"`)
	ccCustom    = []byte(`"type":"custom-title"`)
	ccSummary   = []byte(`"type":"summary"`)
	// Tool results are the bulk of a log. Their lines start their content with
	// a tool_result block, which we can tell without decoding the line.
	ccToolResult  = []byte(`"content":[{"tool_use_id"`)
	ccToolResult2 = []byte(`"content":[{"type":"tool_result"`)
)

func (s *Source) Parse(path string) (*agentlog.Transcript, error) {
	b, f, err := parse.Open(path, agentlog.Claude)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var title, aiTitle, summary string
	err = parse.Lines(f, func(line []byte) {
		user := bytes.Contains(line, ccUser)
		if !user && !bytes.Contains(line, ccAssistant) && !bytes.Contains(line, ccAITitle) &&
			!bytes.Contains(line, ccCustom) && !bytes.Contains(line, ccSummary) {
			return
		}
		if user && (bytes.Contains(line, ccToolResult) || bytes.Contains(line, ccToolResult2)) {
			return
		}
		var r ccRecord
		if json.Unmarshal(line, &r) != nil {
			return
		}
		switch r.Type {
		case "ai-title":
			aiTitle = r.AITitle
			return
		case "custom-title":
			title = r.CustomTitle
			return
		case "summary":
			if summary == "" {
				summary = r.Summary
			}
			return
		case "user", "assistant":
		default:
			return
		}
		if r.IsSidechain || r.IsMeta || r.IsCompactSummary || r.IsTranscriptOnly || r.Message == nil {
			return
		}
		if b.T.ID == "" {
			b.T.ID = r.SessionID
		}
		if b.T.CWD == "" {
			b.T.CWD = r.CWD
		}
		ts := parse.Time(r.Timestamp)
		b.Stamp(ts)
		content := r.Message.Content
		if r.Type == "user" {
			if len(content) > 0 && content[0] == '"' {
				var text string
				if json.Unmarshal(content, &text) == nil {
					b.User(cleanClaudeUser(text), ts)
				}
				return
			}
			var blocks []ccBlock
			if json.Unmarshal(content, &blocks) != nil {
				return
			}
			var parts []string
			for _, bl := range blocks {
				switch bl.Type {
				case "text":
					if t := cleanClaudeUser(bl.Text); t != "" {
						parts = append(parts, t)
					}
				case "image":
					parts = append(parts, "[image]")
				}
			}
			b.User(strings.Join(parts, "\n\n"), ts)
			return
		}
		b.Model(r.Message.Model)
		var blocks []ccBlock
		if json.Unmarshal(content, &blocks) != nil {
			return
		}
		for _, bl := range blocks {
			switch bl.Type {
			case "text":
				b.AssistantText(bl.Text, ts)
			case "tool_use":
				b.Tool(parse.NewToolCall(bl.Name, bl.Input), ts)
			}
		}
	})
	if err != nil {
		return nil, err
	}
	b.T.Title = firstNonEmpty(title, aiTitle, summary)
	return b.Finish(strings.TrimSuffix(filepath.Base(path), ".jsonl")), nil
}

var (
	ccCommandRe = regexp.MustCompile(`(?s)<command-name>(.*?)</command-name>`)
	ccArgsRe    = regexp.MustCompile(`(?s)<command-args>(.*?)</command-args>`)
	ccBashRe    = regexp.MustCompile(`(?s)<bash-input>(.*?)</bash-input>`)
)

// cleanClaudeUser keeps what the user typed of a user message: slash commands
// and ! commands in short, and nothing of command output or reminders.
func cleanClaudeUser(text string) string {
	text = parse.StripReminders(text)
	for _, p := range []string{"<local-command-caveat>", "<local-command-stdout>", "<local-command-stderr>",
		"<bash-stdout>", "<bash-stderr>", "<task-notification>", "<user-memory-input>"} {
		if strings.HasPrefix(text, p) {
			return ""
		}
	}
	if m := ccCommandRe.FindStringSubmatch(text); m != nil {
		cmd := strings.TrimSpace(m[1])
		if a := ccArgsRe.FindStringSubmatch(text); a != nil && strings.TrimSpace(a[1]) != "" {
			cmd += " " + strings.TrimSpace(a[1])
		}
		return cmd
	}
	if m := ccBashRe.FindStringSubmatch(text); m != nil {
		return "! " + strings.TrimSpace(m[1])
	}
	switch text {
	case "[Request interrupted by user]", "[Request interrupted by user for tool use]":
		return ""
	}
	if parse.Injected(text) {
		return ""
	}
	return text
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if v = strings.TrimSpace(v); v != "" {
			return v
		}
	}
	return ""
}

// Related returns the directory beside a session log that holds its
// subagents and tool outputs, which go with it to the trash.
func (s *Source) Related(path string) []string {
	dir := strings.TrimSuffix(path, ".jsonl")
	if st, err := os.Stat(dir); err == nil && st.IsDir() {
		return []string{dir}
	}
	return nil
}

func init() { agentlog.RegisterLabel(agentlog.Claude, "Claude Code") }
