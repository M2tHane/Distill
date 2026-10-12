package pi

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/dade/agentlog"
	"github.com/dade/agentlog/parse"
)

// PiSource reads Pi sessions: <Root>/--<cwd>--/<time>_<id>.jsonl. Deeper
// files are the runs of subagents and are skipped.
type Source struct{ Root string }

func (s *Source) Agent() agentlog.Agent { return agentlog.Pi }

func (s *Source) Files() ([]string, error) {
	return filepath.Glob(filepath.Join(s.Root, "*", "*.jsonl"))
}

type piRecord struct {
	Type      string `json:"type"`
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	CWD       string `json:"cwd"`
	Name      string `json:"name"`
	Message   *struct {
		Role    string          `json:"role"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

type piBlock struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

var (
	piSession    = []byte(`"type":"session"`)
	piInfo       = []byte(`"type":"session_info"`)
	piMessage    = []byte(`"type":"message"`)
	piToolResult = []byte(`"role":"toolResult"`)
)

func (s *Source) Parse(path string) (*agentlog.Transcript, error) {
	b, f, err := parse.Open(path, agentlog.Pi)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var name string
	err = parse.Lines(f, func(line []byte) {
		h := parse.Head(line, 200)
		if !bytes.Contains(h, piSession) && !bytes.Contains(h, piInfo) && !bytes.Contains(h, piMessage) {
			return
		}
		if bytes.Contains(h, piToolResult) {
			return
		}
		var r piRecord
		if json.Unmarshal(line, &r) != nil {
			return
		}
		ts := parse.Time(r.Timestamp)
		switch r.Type {
		case "session":
			if b.T.ID == "" {
				b.T.ID, b.T.CWD = r.ID, r.CWD
			}
			b.Stamp(ts)
			return
		case "session_info":
			name = r.Name
			return
		case "message":
		default:
			return
		}
		if r.Message == nil {
			return
		}
		b.Stamp(ts)
		content := r.Message.Content
		var blocks []piBlock
		if len(content) > 0 && content[0] == '"' {
			var text string
			if json.Unmarshal(content, &text) != nil {
				return
			}
			blocks = []piBlock{{Type: "text", Text: text}}
		} else if json.Unmarshal(content, &blocks) != nil {
			return
		}
		switch r.Message.Role {
		case "user":
			var parts []string
			for _, bl := range blocks {
				switch bl.Type {
				case "text":
					if t := parse.StripReminders(bl.Text); t != "" && !parse.Injected(t) {
						parts = append(parts, t)
					}
				case "image":
					parts = append(parts, "[image]")
				}
			}
			b.User(strings.Join(parts, "\n\n"), ts)
		case "assistant":
			b.Model(r.Message.Model)
			for _, bl := range blocks {
				switch bl.Type {
				case "text":
					b.AssistantText(bl.Text, ts)
				case "toolCall":
					b.Tool(parse.NewToolCall(bl.Name, bl.Arguments), ts)
				}
			}
		}
	})
	if err != nil {
		return nil, err
	}
	b.T.Title = strings.TrimSpace(name)
	base := strings.TrimSuffix(filepath.Base(path), ".jsonl")
	if i := strings.LastIndexByte(base, '_'); i >= 0 {
		base = base[i+1:]
	}
	return b.Finish(base), nil
}

func init() { agentlog.RegisterLabel(agentlog.Pi, "Pi") }
