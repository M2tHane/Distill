package agentlog

import (
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// MarkdownOptions changes how a transcript is written as Markdown.
type MarkdownOptions struct {
	UserHeading      string // "User" by default
	AssistantHeading string // "Assistant" by default
	ToolInputs       bool   // write each call's full input, not only its summary
	// ToolResults writes how calls ended and what of their results was
	// kept, interruptions, and a count of the failures atop.
	ToolResults bool
}

// WriteMarkdown writes a transcript as Markdown, with its metadata as front
// matter.
func WriteMarkdown(w io.Writer, t *Transcript, o MarkdownOptions) error {
	if o.UserHeading == "" {
		o.UserHeading = "User"
	}
	if o.AssistantHeading == "" {
		o.AssistantHeading = "Assistant"
	}
	var b strings.Builder
	b.WriteString("---\n")
	fmt.Fprintf(&b, "agent: %s\n", t.Agent.Label())
	fmt.Fprintf(&b, "session: %s\n", t.ID)
	fmt.Fprintf(&b, "title: %q\n", t.Title)
	if t.CWD != "" {
		fmt.Fprintf(&b, "cwd: %s\n", t.CWD)
	}
	if len(t.Models) > 0 {
		fmt.Fprintf(&b, "models: [%s]\n", strings.Join(t.Models, ", "))
	}
	fmt.Fprintf(&b, "started: %s\n", t.Started.Format(time.RFC3339))
	fmt.Fprintf(&b, "updated: %s\n", t.Updated.Format(time.RFC3339))
	st := toolStats(t)
	if o.ToolResults {
		fmt.Fprintf(&b, "tool_calls: %d\ntool_failures: %d\ninterruptions: %d\n", st.calls, st.failed, st.interrupted)
	}
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n", t.Title)
	if o.ToolResults {
		st.write(&b)
	}
	seen := map[string]int{} // failures by their key, to say when one repeats
	for _, m := range t.Messages {
		heading := o.AssistantHeading
		if m.Role == User {
			heading = o.UserHeading
		}
		fmt.Fprintf(&b, "\n## %s\n\n", heading)
		if m.Text != "" {
			b.WriteString(m.Text)
			b.WriteString("\n")
		}
		if len(m.Tools) > 0 {
			if m.Text != "" {
				b.WriteString("\n")
			}
			for _, tc := range m.Tools {
				var end, result string
				if o.ToolResults {
					end, result = toolResult(tc, seen)
				}
				if o.ToolInputs && tc.Input != "" && tc.Input != tc.Summary {
					fmt.Fprintf(&b, "- **%s**%s\n\n%s\n", tc.Name, end, fence(tc.Input))
				} else {
					fmt.Fprintf(&b, "- **%s** %s%s\n", tc.Name, code(tc.Summary), end)
				}
				b.WriteString(result)
			}
		}
		if o.ToolResults && m.Interrupted {
			b.WriteString("\n_(interrupted by the user)_\n")
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// toolResult is how a call ended, to end its line with, and what of its
// result was kept, to write under it. A failure seen before is only
// counted.
func toolResult(tc ToolCall, seen map[string]int) (end, result string) {
	switch tc.Status {
	case ToolFailed:
		key := errorKey(tc.Output)
		seen[key]++
		if n := seen[key]; n > 1 && key != "" {
			return fmt.Sprintf(" → failed (the same error as before, %s time)", ordinal(n)), ""
		}
		end = " → failed"
	case ToolRejected:
		if tc.Output != "" {
			result = "  > " + strings.ReplaceAll(tc.Output, "\n", "\n  > ") + "\n"
		}
		return " → rejected by the user", result
	case ToolInterrupted:
		return " → interrupted by the user", ""
	}
	if tc.Output != "" {
		result = fence(tc.Output) + "\n"
	}
	return end, result
}

func ordinal(n int) string {
	switch {
	case n%100 >= 11 && n%100 <= 13:
	case n%10 == 1:
		return fmt.Sprintf("%dst", n)
	case n%10 == 2:
		return fmt.Sprintf("%dnd", n)
	case n%10 == 3:
		return fmt.Sprintf("%drd", n)
	}
	return fmt.Sprintf("%dth", n)
}

// stats counts how a transcript's tool calls ended.
type stats struct {
	calls, failed, rejected, interrupted int
	repeated                             []repeat // failures that happened more than once
}

type repeat struct {
	key   string
	tools []string
	n     int
}

func toolStats(t *Transcript) stats {
	var st stats
	byKey := map[string]*repeat{}
	var order []string
	for _, m := range t.Messages {
		if m.Interrupted {
			st.interrupted++
		}
		for _, tc := range m.Tools {
			st.calls++
			switch tc.Status {
			case ToolFailed:
				st.failed++
				key := errorKey(tc.Output)
				if key == "" {
					continue
				}
				r := byKey[key]
				if r == nil {
					r = &repeat{key: key}
					byKey[key] = r
					order = append(order, key)
				}
				r.n++
				if !contains(r.tools, tc.Name) {
					r.tools = append(r.tools, tc.Name)
				}
			case ToolRejected:
				st.rejected++
			case ToolInterrupted:
				st.interrupted++
			}
		}
	}
	for _, k := range order {
		if r := byKey[k]; r.n > 1 {
			st.repeated = append(st.repeated, *r)
		}
	}
	sort.SliceStable(st.repeated, func(i, j int) bool { return st.repeated[i].n > st.repeated[j].n })
	return st
}

// write sums up the failures under the title, the repeated ones first.
func (st stats) write(b *strings.Builder) {
	if st.failed+st.rejected+st.interrupted == 0 {
		return
	}
	fmt.Fprintf(b, "\n> Tools: %d calls, %d failed, %d rejected by the user; %d interruptions by the user.\n", st.calls, st.failed, st.rejected, st.interrupted)
	if len(st.repeated) > 0 {
		b.WriteString(">\n> Errors that repeated:\n")
		for _, r := range st.repeated[:min(len(st.repeated), 10)] {
			fmt.Fprintf(b, "> - ×%d %s (%s)\n", r.n, code(r.key), strings.Join(r.tools, ", "))
		}
	}
}

var (
	errorLineRe = regexp.MustCompile(`(?i)error|fail|panic|exception|denied|not found|cannot|unable|invalid|refused|timed out|rejected`)
	digitsRe    = regexp.MustCompile(`\d+`)
)

// errorKey is a failure's output in short, the same when the same error
// happens again with other numbers in it: its first line that names an
// error, or its first line.
func errorKey(out string) string {
	first := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "…") {
			continue
		}
		if first == "" {
			first = line
		}
		if errorLineRe.MatchString(line) {
			first = line
			break
		}
	}
	first = digitsRe.ReplaceAllString(first, "N")
	if utf8.RuneCountInString(first) > 120 {
		first = string([]rune(first)[:119]) + "…"
	}
	return first
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func code(s string) string {
	if s == "" {
		return ""
	}
	ticks := "`"
	for strings.Contains(s, ticks) {
		ticks += "`"
	}
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		return ticks + " " + s + " " + ticks
	}
	return ticks + s + ticks
}

func fence(s string) string {
	ticks := "```"
	for strings.Contains(s, ticks) {
		ticks += "`"
	}
	return "  " + ticks + "\n" + s + "\n  " + ticks
}
