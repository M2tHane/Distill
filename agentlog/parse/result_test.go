package parse

import (
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
)

func TestToolResult(t *testing.T) {
	long := strings.Repeat("line of output\n", 400)
	cases := []struct {
		name, summary, text string
		isError             bool
		status              agentlog.ToolStatus
		output              string // what the output must contain; "" for none
	}{
		{"Bash", "go test ./...", "Exit code 1\n--- FAIL: TestX\nFAIL", true, agentlog.ToolFailed, "--- FAIL: TestX"},
		// zsh's error, without the exit code: still a failure.
		{"Bash", "grep -r foo --include=*.ts", "(eval):1: no matches found: --include=*.ts", false, agentlog.ToolFailed, "no matches found"},
		// grep finding nothing exits 1, which is no error.
		{"Bash", "grep -rn foo src", "Exit code 1", true, agentlog.ToolOK, ""},
		{"bash", "rg foo", "(no output)\n\nCommand exited with code 1", true, agentlog.ToolOK, ""},
		{"Bash", "ls", "a.go\nb.go", false, agentlog.ToolOK, "a.go\nb.go"},
		// A long success is dropped, but a test's end is kept.
		{"Bash", "cat big.txt", long, false, agentlog.ToolOK, ""},
		{"Bash", "npm test", long + "Tests: 5 passed", false, agentlog.ToolOK, "Tests: 5 passed"},
		{"Read", "/a.go", "package a\n\nfunc A() {}", false, agentlog.ToolOK, ""},
		{"Edit", "/a.go", "<tool_use_error>String to replace not found</tool_use_error>", true, agentlog.ToolFailed, "String to replace not found"},
		{"Edit", "/a.go", "The user doesn't want to proceed with this tool use. The tool use was rejected. To tell you how to proceed, the user said:\n别改这个文件", true, agentlog.ToolRejected, "别改这个文件"},
		{"Bash", "make", "[Request interrupted by user for tool use]", true, agentlog.ToolInterrupted, ""},
		// Codex: a chunk's exit code, inside a script that completed.
		{"exec", "npm run build", "Script completed\nWall time 1.0 seconds\nOutput:\n{\"chunk_id\":\"a\",\"exit_code\":2,\"output\":\"src/a.ts(3,1): error TS2304: Cannot find name 'x'.\\n\"}", false, agentlog.ToolFailed, "error TS2304"},
		{"exec", "git status", "Script completed\nWall time 0.1 seconds\nOutput:\n{\"chunk_id\":\"a\",\"exit_code\":0,\"output\":\" M a.go\\n\"}", false, agentlog.ToolOK, "M a.go"},
		// A script that completed is no failure, though its output names errors.
		{"exec", "tools.list()", "Script completed\nOutput:\n[{\"name\":\"x\",\"description\":\"Error: No such file or directory\"}]", false, agentlog.ToolOK, `"name":"x"`},
		{"exec", "patch a.go", "Script failed\nWall time 0.0 seconds\nOutput:\n\nScript error:\napply_patch verification failed: Failed to find expected lines", false, agentlog.ToolFailed, "apply_patch verification failed"},
		{"exec_command", "curl x", "Chunk ID: 1\nWall time: 0.1 seconds\nProcess exited with code 7\nOutput:\ncurl: (7) Failed to connect", false, agentlog.ToolFailed, "curl: (7) Failed to connect"},
		{"shell", "make", `{"output":"make: *** No rule to make target","metadata":{"exit_code":2}}`, false, agentlog.ToolFailed, "No rule to make target"},
		{"Bash", "ls", "\x1b[31mError: x\x1b[0m", true, agentlog.ToolFailed, "Error: x"},
	}
	for _, c := range cases {
		tc := agentlog.ToolCall{Name: c.name, Summary: c.summary}
		ToolResult(&tc, c.text, c.isError)
		if tc.Status != c.status {
			t.Errorf("%s %q: status %q, want %q (output %q)", c.name, c.summary, tc.Status, c.status, tc.Output)
		}
		if c.output == "" && tc.Output != "" || !strings.Contains(tc.Output, c.output) {
			t.Errorf("%s %q: output %q, want %q", c.name, c.summary, tc.Output, c.output)
		}
		for _, noise := range []string{"Wall time", "Script", "Chunk ID", "Exit code", "\x1b"} {
			if strings.Contains(tc.Output, noise) {
				t.Errorf("%s %q: output keeps %q: %q", c.name, c.summary, noise, tc.Output)
			}
		}
	}
}

func TestCondense(t *testing.T) {
	var b strings.Builder
	b.WriteString("Running tests\n")
	for range 300 {
		b.WriteString("ok   some/package   0.1s\n")
	}
	b.WriteString("--- FAIL: TestLogin (0.00s)\n    login_test.go:12: expected 200, got 500\n")
	for range 300 {
		b.WriteString("ok   other/package   0.1s\n")
	}
	b.WriteString("FAIL\n")
	out := condense(b.String(), errorBytes)
	if len(out) > errorBytes+16 {
		t.Errorf("%d bytes", len(out))
	}
	for _, want := range []string{"Running tests", "--- FAIL: TestLogin", "login_test.go:12", "FAIL", "lines)"} {
		if !strings.Contains(out, want) {
			t.Errorf("condensed output lacks %q:\n%s", want, out)
		}
	}
}
