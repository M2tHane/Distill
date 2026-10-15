package codex

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/agentlog/parse"
)

// CodexSource reads Codex rollouts: <Root>/sessions/YYYY/MM/DD/rollout-*.jsonl
// and <Root>/archived_sessions/rollout-*.jsonl. Thread names come from
// <Root>/session_index.jsonl.
type Source struct {
	Root string

	mu       sync.Mutex
	names    map[string]string
	namesMod time.Time
}

func (s *Source) Agent() agentlog.Agent { return agentlog.Codex }

func (s *Source) Files() ([]string, error) {
	var files []string
	err := filepath.WalkDir(filepath.Join(s.Root, "sessions"), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == filepath.Join(s.Root, "sessions") {
				return err
			}
			return nil
		}
		if !d.IsDir() && strings.HasPrefix(d.Name(), "rollout-") && strings.HasSuffix(d.Name(), ".jsonl") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	archived, _ := filepath.Glob(filepath.Join(s.Root, "archived_sessions", "rollout-*.jsonl"))
	return append(files, archived...), nil
}

type cxRecord struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type cxItem struct {
	Type    string `json:"type"`
	Role    string `json:"role"`
	Name    string `json:"name"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Arguments string          `json:"arguments"`
	Input     string          `json:"input"`
	Action    json.RawMessage `json:"action"`
	CallID    string          `json:"call_id"`
	Output    json.RawMessage `json:"output"`
	Reason    string          `json:"reason"` // turn_aborted
	// session_meta and turn_context
	ID    string `json:"id"`
	CWD   string `json:"cwd"`
	Model string `json:"model"`
}

var (
	cxMeta     = []byte(`"type":"session_meta"`)
	cxTurn     = []byte(`"type":"turn_context"`)
	cxResponse = []byte(`"type":"response_item"`)
	cxAborted  = []byte(`"payload":{"type":"turn_aborted"`)
	cxSkip     = [][]byte{
		[]byte(`"payload":{"type":"reasoning"`),
	}
	cxOutputs = [][]byte{
		[]byte(`"payload":{"type":"function_call_output"`),
		[]byte(`"payload":{"type":"custom_tool_call_output"`),
		[]byte(`"payload":{"type":"local_shell_call_output"`),
	}
)

func (s *Source) Parse(path string) (*agentlog.Transcript, error) { return s.parse(path, true) }

// Summarize reads a session without its tool results.
func (s *Source) Summarize(path string) (*agentlog.Transcript, error) { return s.parse(path, false) }

func (s *Source) parse(path string, results bool) (*agentlog.Transcript, error) {
	b, f, err := parse.Open(path, agentlog.Codex)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b.T.Archived = strings.Contains(path, string(filepath.Separator)+"archived_sessions"+string(filepath.Separator))
	err = parse.Lines(f, func(line []byte) {
		h := parse.Head(line, 160)
		meta, turn := bytes.Contains(h, cxMeta), bytes.Contains(h, cxTurn)
		aborted := bytes.Contains(h, cxAborted)
		if !meta && !turn && !aborted && !bytes.Contains(h, cxResponse) {
			return
		}
		for _, skip := range cxSkip {
			if bytes.Contains(h, skip) {
				return
			}
		}
		if !results {
			for _, skip := range cxOutputs {
				if bytes.Contains(h, skip) {
					return
				}
			}
		}
		var r cxRecord
		if json.Unmarshal(line, &r) != nil {
			return
		}
		var it cxItem
		if json.Unmarshal(r.Payload, &it) != nil {
			return
		}
		ts := parse.Time(r.Timestamp)
		switch r.Type {
		case "session_meta":
			if b.T.ID == "" {
				b.T.ID, b.T.CWD = it.ID, it.CWD
			}
			b.Stamp(ts)
			return
		case "turn_context":
			b.Model(it.Model)
			if b.T.CWD == "" {
				b.T.CWD = it.CWD
			}
			return
		case "event_msg":
			if it.Type == "turn_aborted" && it.Reason == "interrupted" {
				b.Interrupted(ts)
			}
			return
		case "response_item":
		default:
			return
		}
		b.Stamp(ts)
		switch it.Type {
		case "message":
			var parts []string
			for _, c := range it.Content {
				switch c.Type {
				case "input_text", "output_text", "text":
					parts = append(parts, c.Text)
				case "input_image":
					parts = append(parts, "[image]")
				}
			}
			text := strings.Join(parts, "\n\n")
			switch it.Role {
			case "user":
				if !parse.Injected(text) {
					b.User(text, ts)
				}
			case "assistant":
				b.AssistantText(text, ts)
			}
		case "function_call":
			b.Tool(withID(parse.NewToolCall(it.Name, json.RawMessage(it.Arguments)), it.CallID), ts)
		case "custom_tool_call":
			b.Tool(withID(parse.TextToolCall(it.Name, it.Input), it.CallID), ts)
		case "local_shell_call":
			b.Tool(withID(parse.NewToolCall("shell", it.Action), it.CallID), ts)
		case "web_search_call":
			b.Tool(parse.NewToolCall("web_search", it.Action), ts)
		case "function_call_output", "custom_tool_call_output", "local_shell_call_output":
			b.Result(it.CallID, outputText(it.Output), false)
		}
	})
	if err != nil {
		return nil, err
	}
	t := b.Finish(strings.TrimSuffix(filepath.Base(path), ".jsonl"))
	s.Decorate(&t.Session)
	return t, nil
}

func withID(tc agentlog.ToolCall, id string) agentlog.ToolCall {
	tc.ID = id
	return tc
}

// outputText is the text of a call's output: a string, or parts of text.
func outputText(raw json.RawMessage) string {
	if len(raw) > 0 && raw[0] == '"' {
		var s string
		json.Unmarshal(raw, &s)
		return s
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &parts) != nil {
		return ""
	}
	var texts []string
	for _, p := range parts {
		if p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	return strings.Join(texts, "\n")
}

// Decorate gives a session its thread name, which Codex keeps apart from the
// rollout and may change without touching it.
func (s *Source) Decorate(sess *agentlog.Session) {
	if name := s.threadName(sess.ID); name != "" {
		sess.Title = name
	}
}

func (s *Source) threadName(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := filepath.Join(s.Root, "session_index.jsonl")
	st, err := os.Stat(p)
	if err != nil {
		return ""
	}
	if s.names == nil || !st.ModTime().Equal(s.namesMod) {
		s.names, s.namesMod = map[string]string{}, st.ModTime()
		if f, err := os.Open(p); err == nil {
			parse.Lines(f, func(line []byte) {
				var e struct {
					ID   string `json:"id"`
					Name string `json:"thread_name"`
				}
				if json.Unmarshal(line, &e) == nil && e.Name != "" {
					s.names[e.ID] = e.Name
				}
			})
			f.Close()
		}
	}
	return s.names[id]
}

func init() { agentlog.RegisterLabel(agentlog.Codex, "Codex") }
