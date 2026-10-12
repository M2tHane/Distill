package agentlog

import (
	"fmt"
	"io"
	"strings"
	"time"
)

// MarkdownOptions changes how a transcript is written as Markdown.
type MarkdownOptions struct {
	UserHeading      string // "User" by default
	AssistantHeading string // "Assistant" by default
	ToolInputs       bool   // write each call's full input, not only its summary
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
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "# %s\n", t.Title)
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
				if o.ToolInputs && tc.Input != "" && tc.Input != tc.Summary {
					fmt.Fprintf(&b, "- **%s**\n\n%s\n", tc.Name, fence(tc.Input))
				} else {
					fmt.Fprintf(&b, "- **%s** %s\n", tc.Name, code(tc.Summary))
				}
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
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
