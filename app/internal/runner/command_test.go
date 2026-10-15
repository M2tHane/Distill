package runner

import (
	"slices"
	"strings"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
	"github.com/M2tHane/Distill/app/internal/env"
)

// TestCommands checks each agent's command for a first and a resumed turn,
// with a model and an effort.
func TestCommands(t *testing.T) {
	for _, c := range []struct {
		agent  agentlog.Agent
		resume bool
		want   string
		stdin  bool
	}{
		{agentlog.Claude, false, "-p --output-format stream-json --verbose --permission-mode acceptEdits --allowedTools " + claudeTools + " --session-id S --model M --effort high", true},
		{agentlog.Claude, true, "-p --output-format stream-json --verbose --permission-mode acceptEdits --allowedTools " + claudeTools + " --resume S --model M --effort high", true},
		{agentlog.Codex, false, `exec --json --skip-git-repo-check -c sandbox_mode="workspace-write" -C /d -m M -c model_reasoning_effort="high" -`, true},
		{agentlog.Codex, true, `exec resume --json --skip-git-repo-check -c sandbox_mode="workspace-write" -m M -c model_reasoning_effort="high" S -`, true},
		{agentlog.Pi, false, "-p --mode json --session-id S --model M --thinking high -- P", false},
	} {
		r := Request{Info: env.AgentInfo{Agent: c.agent}, Model: "M", Effort: "high", Dir: "/d", Prompt: "P", SessionID: "S", Resume: c.resume}
		args, stdin := adapters[c.agent].command(r)
		if got := strings.Join(args, " "); got != c.want {
			t.Errorf("%s resume=%v:\n got %s\nwant %s", c.agent, c.resume, got, c.want)
		}
		if (stdin == "P") != c.stdin {
			t.Errorf("%s: stdin %q", c.agent, stdin)
		}
		// Without a model or an effort, neither is passed.
		r.Model, r.Effort = "", ""
		args, _ = adapters[c.agent].command(r)
		if slices.Contains(args, "M") || slices.ContainsFunc(args, func(a string) bool { return strings.Contains(a, "high") }) {
			t.Errorf("%s: defaults passed: %v", c.agent, args)
		}
	}
}
