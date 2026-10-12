package parse

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dade/agentlog"
)

// Builder assembles a transcript from the records of a log, in order: a
// source calls User, AssistantText and Tool as it reads them, then Finish.
type Builder struct {
	T   *agentlog.Transcript // what was read so far, and the session's metadata
	cur *agentlog.Message    // the assistant message being built
}

func Open(path string, agent agentlog.Agent) (*Builder, *os.File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	t := &agentlog.Transcript{Session: agentlog.Session{Agent: agent, Path: path, Size: st.Size(), ModTime: st.ModTime()}}
	return &Builder{T: t}, f, nil
}

func (b *Builder) Stamp(ts time.Time) {
	if ts.IsZero() {
		return
	}
	if b.T.Started.IsZero() || ts.Before(b.T.Started) {
		b.T.Started = ts
	}
	if ts.After(b.T.Updated) {
		b.T.Updated = ts
	}
}

func (b *Builder) Model(m string) {
	if m == "" || m == "<synthetic>" || slices.Contains(b.T.Models, m) {
		return
	}
	b.T.Models = append(b.T.Models, m)
}

func (b *Builder) User(text string, ts time.Time) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	b.cur = nil
	b.T.Messages = append(b.T.Messages, agentlog.Message{Role: agentlog.User, Text: text, Time: ts})
}

func (b *Builder) AssistantText(text string, ts time.Time) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	if b.cur != nil && len(b.cur.Tools) == 0 {
		b.cur.Text += "\n\n" + text
		return
	}
	b.newAssistant(ts).Text = text
}

func (b *Builder) Tool(tc agentlog.ToolCall, ts time.Time) {
	if b.cur == nil {
		b.newAssistant(ts)
	}
	b.cur.Tools = append(b.cur.Tools, tc)
}

func (b *Builder) newAssistant(ts time.Time) *agentlog.Message {
	b.T.Messages = append(b.T.Messages, agentlog.Message{Role: agentlog.Assistant, Time: ts})
	b.cur = &b.T.Messages[len(b.T.Messages)-1]
	return b.cur
}

// Finish fills what the records did not give.
func (b *Builder) Finish(fallbackID string) *agentlog.Transcript {
	t := b.T
	// b.cur points into Messages; it must not outlive appends, so drop it.
	b.cur = nil
	if t.ID == "" {
		t.ID = fallbackID
	}
	if t.Title == "" {
		for _, m := range t.Messages {
			if m.Role == agentlog.User {
				t.Title = TitleFrom(m.Text)
				break
			}
		}
	}
	if t.Updated.IsZero() {
		t.Updated = t.ModTime
	}
	if t.Started.IsZero() {
		t.Started = t.Updated
	}
	t.Session.Messages = len(t.Messages)
	return t
}

// Lines calls fn with every non-empty line of r. The slice is only valid
// during the call.
func Lines(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 1<<20)
	var long []byte
	for {
		line, err := br.ReadSlice('\n')
		if err == bufio.ErrBufferFull {
			long = append(long[:0], line...)
			for err == bufio.ErrBufferFull {
				line, err = br.ReadSlice('\n')
				long = append(long, line...)
			}
			line = long
		}
		if line = bytes.TrimSpace(line); len(line) > 0 {
			fn(line)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func Time(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

// Head returns the first n bytes of line, to look for markers near its start.
func Head(line []byte, n int) []byte {
	if len(line) > n {
		return line[:n]
	}
	return line
}

func TitleFrom(text string) string {
	text = strings.TrimSpace(text)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "#>-* "))
		if line != "" {
			return Truncate(line, 80)
		}
	}
	return Truncate(text, 80)
}

// Truncate cuts s to at most n runes, with an ellipsis.
func Truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n-1]) + "…"
}

// OneLine joins the lines of s and truncates it.
func OneLine(s string, n int) string {
	return Truncate(strings.Join(strings.Fields(s), " "), n)
}

var (
	reminderRe = regexp.MustCompile(`(?s)<system-reminder>.*?</system-reminder>`)
	// A message that is only an XML-like block, as <environment_context>…</environment_context>,
	// is context the agent injected, not something the user typed.
	injectedRe = regexp.MustCompile(`(?s)^<([a-zA-Z_][\w-]*)[^>]*>.*</([a-zA-Z_][\w-]*)>$`)
)

// StripReminders removes the <system-reminder> blocks an agent adds to
// what the user typed.
func StripReminders(text string) string {
	return strings.TrimSpace(reminderRe.ReplaceAllString(text, ""))
}

// Injected reports whether a user message is context added by the agent.
func Injected(text string) bool {
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "# AGENTS.md instructions") {
		return true
	}
	m := injectedRe.FindStringSubmatch(text)
	return m != nil && m[1] == m[2]
}
