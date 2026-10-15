package parse

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/M2tHane/Distill/agentlog"
)

// Most of a log is tool results: files read, searches, command output. A
// transcript keeps only what tells how the work went, which an agent
// reading it needs to find what went wrong:
//
//   - the error of a call that failed, condensed to the lines that say
//     what failed;
//   - that the user rejected a call, and why if they said;
//   - that the user interrupted it;
//   - the short output of a command that succeeded, or the last lines of a
//     test, build or check;
//   - the head of a subagent's report.
const (
	errorBytes    = 1600 // of a failure's output
	okBytes       = 320  // of a command's whole output, to keep it all
	checkLines    = 6    // kept of a long test, build or check
	subagentBytes = 800
)

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	// The lines a harness adds around a command's output.
	exitRe  = regexp.MustCompile(`^(?:Exit code:? |Process exited with code |Command exited with code )(-?\d+)$`)
	noiseRe = regexp.MustCompile(`^(?:Wall time:? .*|Chunk ID: .*|Original token count: .*|Total output lines: .*|Output:|Script completed|Warning: truncated output.*|Output too large .*|Preview \(first .*|</?persisted-output>|Command running in background.*|\(no output\))$`)
	// errorRe finds an error in the output of a command whose exit code is
	// not known.
	errorRe = regexp.MustCompile(`(?m)^(?:error|Error|ERROR)\b[:\[ ]|command not found|no matches found|No such file or directory|Permission denied|Traceback \(most recent call last\)|^panic: |^--- FAIL|^FAIL\b|^fatal: |npm ERR!|error TS\d+|SyntaxError|ModuleNotFoundError|Cannot find module|Segmentation fault|is not recognized as an internal`)
	// errorLineRe picks the lines worth keeping of a long failure.
	errorLineRe = regexp.MustCompile(`(?i)error|fail|panic|exception|traceback|denied|not found|cannot|unable|invalid|refused|timeout|timed out|rejected|expected|^\s+at |^\s+File "|:\d+:\d+`)
	searchRe    = regexp.MustCompile(`(?:^|[|;&(]\s*)(?:grep|egrep|rg|ag|diff|cmp|test|\[)\s[^|;&]*$`)
	checkRe     = regexp.MustCompile(`(?i)\b(?:test|tests|pytest|vitest|jest|build|lint|eslint|tsc|typecheck|vet|check|cargo|make|mvn|gradle|ruff|mypy|clippy)\b`)
	rejectRe    = regexp.MustCompile(`(?s)doesn't want to proceed with this tool use.*?(?:the user said:\s*(.*))?$`)
)

// ToolResult reads a call's result into its Status and Output. isError is
// what the log says, which not every agent sets on every failure.
func ToolResult(tc *agentlog.ToolCall, text string, isError bool) {
	if strings.Contains(text, "doesn't want to proceed with this tool use") {
		tc.Status = agentlog.ToolRejected
		if m := rejectRe.FindStringSubmatch(text); m != nil {
			tc.Output = Truncate(strings.TrimSpace(m[1]), 600)
		}
		return
	}
	if strings.Contains(text, "[Request interrupted by user") {
		tc.Status, tc.Output = agentlog.ToolInterrupted, ""
		return
	}
	out, code, failed, completed := cleanOutput(text)
	shell := isShell(tc.Name)
	if code == 1 && !failed && searchRe.MatchString(tc.Summary) && !errorRe.MatchString(out) {
		// grep and the like exit 1 when nothing matches, which some agents
		// call an error.
		code, isError = 0, false
	}
	switch {
	case isError || failed || code > 0:
		tc.Status = agentlog.ToolFailed
	case code < 0 && !completed && shell && errorRe.MatchString(out):
		// No exit code: the error says it.
		tc.Status = agentlog.ToolFailed
	}
	if tc.Status == agentlog.ToolFailed {
		out = strings.TrimSpace(strings.NewReplacer("<tool_use_error>", "", "</tool_use_error>", "").Replace(out))
		if out == "" && code > 0 {
			out = "exit code " + strconv.Itoa(code)
		}
		tc.Output = condense(out, errorBytes)
		return
	}
	switch {
	case shell && len(out) <= okBytes && strings.Count(out, "\n") < 8:
		tc.Output = out
	case shell && checkRe.MatchString(tc.Summary):
		tc.Output = tail(out, checkLines, okBytes*2)
	case isSubagent(tc.Name):
		tc.Output = Truncate(out, subagentBytes)
	default:
		tc.Output = ""
	}
}

// cleanOutput takes off what the harness wrapped a result in. It finds the
// command's exit code (-1 when it does not say; the first non-zero of
// several commands), and whether the harness said the call failed or
// completed.
func cleanOutput(text string) (out string, code int, failed, completed bool) {
	code = -1
	text = strings.TrimSpace(ansiRe.ReplaceAllString(text, ""))
	var keep []string
	var clean func(text string)
	clean = func(text string) {
		for _, line := range strings.Split(text, "\n") {
			t := strings.TrimSpace(line)
			if m := exitRe.FindStringSubmatch(t); m != nil {
				c, _ := strconv.Atoi(m[1])
				code = exitCode(code, c)
				continue
			}
			switch t {
			case "Script failed":
				failed = true
				continue
			case "Script completed":
				completed = true
				continue
			case "Script error:", "{}":
				continue
			}
			if noiseRe.MatchString(t) {
				continue
			}
			// Codex: {"output": "…", "metadata": {"exit_code": 1}}, or a chunk
			// of a code-mode script: {"chunk_id": …, "exit_code": 1, "output": "…"}.
			if strings.HasPrefix(t, "{") {
				if o, c, ok := jsonOutput(t); ok {
					if c >= 0 {
						code = exitCode(code, c)
					}
					clean(ansiRe.ReplaceAllString(o, ""))
					continue
				}
			}
			keep = append(keep, line)
		}
	}
	clean(text)
	return strings.TrimSpace(strings.Join(keep, "\n")), code, failed, completed
}

// exitCode is the code of commands, of which the last ran with c: the
// first that failed.
func exitCode(code, c int) int {
	if code > 0 {
		return code
	}
	return c
}

func jsonOutput(s string) (string, int, bool) {
	if !strings.HasPrefix(s, "{") || !strings.Contains(s, `"output"`) {
		return "", -1, false
	}
	var v struct {
		Output   *string `json:"output"`
		ExitCode *int    `json:"exit_code"`
		Metadata struct {
			ExitCode *int `json:"exit_code"`
		} `json:"metadata"`
	}
	if json.Unmarshal([]byte(s), &v) != nil || v.Output == nil {
		return "", -1, false
	}
	code := -1
	if v.ExitCode != nil {
		code = *v.ExitCode
	} else if v.Metadata.ExitCode != nil {
		code = *v.Metadata.ExitCode
	}
	return *v.Output, code, true
}

func isShell(name string) bool {
	n := strings.ToLower(name)
	switch n {
	case "exec", "exec_command", "run_command", "terminal":
		return true
	}
	return strings.Contains(n, "bash") || strings.Contains(n, "shell")
}

func isSubagent(name string) bool {
	switch name {
	case "Task", "Agent", "spawn_agent", "subagent", "wait_agent":
		return true
	}
	return false
}

// condense cuts a long output to about n bytes: its first lines, the lines
// that look like errors, and its last lines.
func condense(s string, n int) string {
	if len(s) <= n {
		return s
	}
	lines := strings.Split(s, "\n")
	const head, tail = 6, 10
	if len(lines) <= head+tail {
		return clipBytes(s, n)
	}
	keep := make([]bool, len(lines))
	for i := range lines {
		keep[i] = i < head || i >= len(lines)-tail
	}
	picked := 0
	for i := head; i < len(lines)-tail && picked < 24; i++ {
		if errorLineRe.MatchString(lines[i]) {
			keep[i] = true
			picked++
		}
	}
	var b strings.Builder
	skipped := 0
	for i, line := range lines {
		if !keep[i] {
			skipped++
			continue
		}
		if skipped > 0 {
			b.WriteString("… (" + strconv.Itoa(skipped) + " lines)\n")
			skipped = 0
		}
		b.WriteString(Truncate(line, 240))
		b.WriteByte('\n')
	}
	out := strings.TrimSpace(b.String())
	if len(out) > n {
		// Too many error lines: their start and their end.
		return clipBytes(out[:n/2], n/2) + "\n…\n" + lastBytes(out, n/2)
	}
	return out
}

// tail returns the last n lines of s, at most max bytes.
func tail(s string, n, max int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = append([]string{"…"}, lines[len(lines)-n:]...)
	}
	return lastBytes(strings.Join(lines, "\n"), max)
}

func clipBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + "…"
}

func lastBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := len(s) - n
	for cut < len(s) && s[cut]&0xC0 == 0x80 {
		cut++
	}
	return "…" + s[cut:]
}
