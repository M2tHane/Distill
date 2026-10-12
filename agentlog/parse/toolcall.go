package parse

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/dade/agentlog"
)

const (
	summaryRunes = 200
	inputBytes   = 16 << 10
)

// summaryKeys are the arguments that best say what a call did, in order.
var summaryKeys = []string{
	"command", "cmd", "file_path", "notebook_path", "path", "pattern", "query",
	"url", "skill", "description", "prompt", "subject", "message",
}

// NewToolCall describes a call from its JSON arguments.
func NewToolCall(name string, args json.RawMessage) agentlog.ToolCall {
	tc := agentlog.ToolCall{Name: name}
	var m map[string]any
	if json.Unmarshal(args, &m) != nil || m == nil {
		raw := strings.TrimSpace(string(args))
		if raw == "null" || raw == "{}" {
			raw = ""
		}
		tc.Summary, tc.Input = OneLine(raw, summaryRunes), clip(raw)
		return tc
	}
	for _, k := range summaryKeys {
		if v, ok := m[k]; ok {
			if s := argString(v); s != "" {
				tc.Summary = OneLine(s, summaryRunes)
				break
			}
		}
	}
	if s, ok := m["command"].(string); ok && len(m) <= 3 {
		tc.Input = clip(s)
	} else if s, ok := m["cmd"].(string); ok && len(m) <= 3 {
		tc.Input = clip(s)
	} else {
		tc.Input = clip(indentJSON(args))
	}
	if tc.Summary == "" {
		tc.Summary = OneLine(compactJSON(args), summaryRunes)
	}
	return tc
}

// textToolCall describes a call whose input is free text, as a patch or code.
func TextToolCall(name, input string) agentlog.ToolCall {
	tc := agentlog.ToolCall{Name: name, Input: clip(input)}
	if files := patchFiles(input); len(files) > 0 {
		tc.Summary = OneLine("patch "+strings.Join(files, ", "), summaryRunes)
		return tc
	}
	if cmds := jsCommands(input); len(cmds) > 0 {
		tc.Summary = OneLine(strings.Join(cmds, " ; "), summaryRunes)
		return tc
	}
	tc.Summary = OneLine(input, summaryRunes)
	return tc
}

func argString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []any:
		parts := make([]string, 0, len(v))
		for _, p := range v {
			parts = append(parts, fmt.Sprint(p))
		}
		// ["bash", "-lc", "script"] says what ran in its last part.
		if len(parts) == 3 && (parts[1] == "-lc" || parts[1] == "-c") {
			return parts[2]
		}
		return strings.Join(parts, " ")
	}
	return ""
}

var (
	patchFileRe = regexp.MustCompile(`\*\*\* (?:Add|Update|Delete) File: ([^\n\\]+)`)
	jsCmdRe     = regexp.MustCompile("cmd:\\s*(\"(?:[^\"\\\\]|\\\\.)*\"|'(?:[^'\\\\]|\\\\.)*'|`[^`]*`)")
)

func patchFiles(s string) []string {
	var files []string
	for _, m := range patchFileRe.FindAllStringSubmatch(s, 8) {
		files = append(files, strings.TrimSpace(m[1]))
	}
	return files
}

// jsCommands finds the shell commands a Codex code-mode script runs.
func jsCommands(s string) []string {
	var cmds []string
	for _, m := range jsCmdRe.FindAllStringSubmatch(s, 4) {
		q := m[1]
		var cmd string
		if q[0] == '"' {
			if json.Unmarshal([]byte(q), &cmd) != nil {
				cmd = q[1 : len(q)-1]
			}
		} else {
			cmd = q[1 : len(q)-1]
		}
		cmds = append(cmds, cmd)
	}
	return cmds
}

func clip(s string) string {
	if len(s) <= inputBytes {
		return s
	}
	cut := inputBytes
	for cut > 0 && s[cut]&0xC0 == 0x80 {
		cut--
	}
	return s[:cut] + fmt.Sprintf("\n… (%d more bytes)", len(s)-cut)
}

func indentJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Indent(&buf, raw, "", "  ") != nil {
		return string(raw)
	}
	return buf.String()
}

func compactJSON(raw json.RawMessage) string {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return string(raw)
	}
	return buf.String()
}
