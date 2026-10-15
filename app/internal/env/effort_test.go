package env

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/M2tHane/Distill/agentlog"
)

func TestEfforts(t *testing.T) {
	home := t.TempDir()
	p := Paths{Home: home}
	write := func(name, s string) {
		path := filepath.Join(home, name)
		os.MkdirAll(filepath.Dir(path), 0o755)
		os.WriteFile(path, []byte(s), 0o644)
	}
	write(".codex/models_cache.json", `{"models":[
		{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"low"},{"effort":"medium"},{"effort":"high"}]},
		{"slug":"workbuddy/hy4","supported_reasoning_levels":[{"effort":"high"}]}]}`)
	write(".codex/config.toml", "notify = []\nmodel = \"gpt-6.1-sol\"\n[desktop]\nmodel = \"other\"\n")
	write(".pi/agent/models-store.json", `{"openai-codex":{"models":[
		{"id":"gpt-5.5","reasoning":true,"thinkingLevelMap":{"xhigh":"xhigh","minimal":"low"}},
		{"id":"plain","reasoning":false}]}}`)
	write(".pi/agent/models.json", `{"providers":{"magpie":{"models":[
		{"id":"codex/GPT-6.1-Sol","reasoning":true,"thinkingLevelMap":{"off":null,"minimal":null,"low":"low","medium":"medium","high":"high","xhigh":"xhigh","max":"max"}},
		{"id":"workbuddy/hy4","reasoning":true,"thinkingLevelMap":{"off":null,"minimal":null,"low":null,"medium":null,"high":"high","xhigh":null,"max":null}},
		{"id":"zcode/GLM-5.3-Flash","reasoning":true,"thinkingLevelMap":{"off":null,"minimal":null,"low":"low","medium":null,"high":"high","xhigh":null,"max":"max"}}]}}}`)
	write(".pi/agent/settings.json", `{"defaultProvider":"magpie","defaultModel":"codex/GPT-6.1-Sol"}`)

	codex := AgentInfo{Agent: agentlog.Codex}
	codexEfforts(p, &codex)
	pi := AgentInfo{Agent: agentlog.Pi, Models: []string{"other/thinker", "other/plain", "magpie/codex/gpt-6.1-sol", "magpie/workbuddy/hy4", "openai-codex/plain"}}
	piEfforts(p, &pi, map[string]bool{"other/thinker": true, "magpie/codex/gpt-6.1-sol": true, "magpie/workbuddy/hy4": true, "openai-codex/plain": true})
	claude := AgentInfo{Agent: agentlog.Claude}

	for _, c := range []struct {
		info  AgentInfo
		model string
		want  []string
	}{
		{codex, "gpt-6.1-sol", []string{"low", "medium", "high"}},
		{codex, "", []string{"low", "medium", "high"}},
		{codex, "workbuddy/hy4", []string{"high"}},
		{codex, "unknown", nil},
		{pi, "openai-codex/gpt-5.5", []string{"off", "minimal", "low", "medium", "high", "xhigh"}},
		{pi, "openai-codex/plain", nil},
		{pi, "magpie/codex/gpt-6.1-sol", []string{"low", "medium", "high", "xhigh", "max"}},
		{pi, "", []string{"low", "medium", "high", "xhigh", "max"}},
		{pi, "magpie/zcode/GLM-5.3-Flash", []string{"low", "high", "max"}},
		{pi, "other/thinker", []string{"off", "minimal", "low", "medium", "high"}},
		{pi, "other/plain", nil},
		{pi, "magpie/workbuddy/hy4", []string{"high"}},
		{claude, "", []string{"low", "medium", "high", "xhigh", "max"}},
		{claude, "claude-opus-4-6", []string{"low", "medium", "high", "max"}},
		{claude, "claude-haiku-4-5", nil},
	} {
		if got := c.info.Efforts(c.model); !slices.Equal(got, c.want) {
			t.Errorf("%s %q: %v, want %v", c.info.Agent, c.model, got, c.want)
		}
	}
}
