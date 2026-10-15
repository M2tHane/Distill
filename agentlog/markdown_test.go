package agentlog

import (
	"strings"
	"testing"
	"time"
)

func TestWriteMarkdownResults(t *testing.T) {
	fail := func(out string) ToolCall {
		return ToolCall{Name: "Bash", Summary: "grep -r x --include=*.ts", Status: ToolFailed, Output: out}
	}
	tr := &Transcript{
		Session: Session{Agent: Claude, ID: "s", Title: "t", Started: time.Unix(0, 0), Updated: time.Unix(0, 0)},
		Messages: []Message{
			{Role: User, Text: "找一下 x"},
			{Role: Assistant, Tools: []ToolCall{
				fail("(eval):1: no matches found: --include=*.ts"),
				fail("(eval):2: no matches found: --include=*.ts"),
				{Name: "Read", Summary: "/a.go"},
				{Name: "Edit", Summary: "/a.go", Status: ToolRejected, Output: "别改这个"},
				{Name: "Bash", Summary: "go test ./...", Output: "ok  a  0.1s"},
			}},
			{Role: Assistant, Text: "找到了", Interrupted: true},
		},
	}
	var b strings.Builder
	if err := WriteMarkdown(&b, tr, MarkdownOptions{ToolResults: true}); err != nil {
		t.Fatal(err)
	}
	md := b.String()
	for _, want := range []string{
		"tool_calls: 5\ntool_failures: 2\ninterruptions: 1\n",
		"> Tools: 5 calls, 2 failed, 1 rejected by the user; 1 interruptions by the user.",
		"> - ×2 `(eval):N: no matches found: --include=*.ts` (Bash)",
		"- **Bash** `grep -r x --include=*.ts` → failed\n  ```\n(eval):1: no matches found",
		"→ failed (the same error as before, 2nd time)\n- **Read** `/a.go`\n",
		"- **Edit** `/a.go` → rejected by the user\n  > 别改这个\n",
		"- **Bash** `go test ./...`\n  ```\nok  a  0.1s\n  ```\n",
		"找到了\n\n_(interrupted by the user)_\n",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("no %q in:\n%s", want, md)
		}
	}

	b.Reset()
	WriteMarkdown(&b, tr, MarkdownOptions{})
	if md := b.String(); strings.Contains(md, "failed") || strings.Contains(md, "interrupted") || strings.Contains(md, "tool_calls") {
		t.Errorf("results without ToolResults:\n%s", md)
	}
}
