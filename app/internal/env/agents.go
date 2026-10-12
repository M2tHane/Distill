package env

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/M2tHane/Distill/agentlog"
)

// Agents are the agents Distill knows, in the order it shows them.
var Agents = []agentlog.Agent{agentlog.Claude, agentlog.Codex, agentlog.Pi}

// AgentInfo is what Distill found of an agent on this computer.
type AgentInfo struct {
	Agent     agentlog.Agent
	Bin       string // empty when not installed
	Version   string
	Models    []string
	Detecting bool
}

func (a AgentInfo) Installed() bool { return a.Bin != "" }

// Commands are the agents' executables.
var Commands = map[agentlog.Agent]string{
	agentlog.Claude: "claude",
	agentlog.Codex:  "codex",
	agentlog.Pi:     "pi",
}

var versionRe = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// Detect finds an agent's command, its version and its models.
func Detect(p Paths, a agentlog.Agent) AgentInfo {
	info := AgentInfo{Agent: a}
	bin, err := exec.LookPath(Commands[a])
	if err != nil {
		return info
	}
	info.Bin = bin
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, bin, "--version").CombinedOutput(); err == nil {
		info.Version = versionRe.FindString(string(out))
	}
	switch a {
	case agentlog.Claude:
		info.Models = ClaudeModels(p)
	case agentlog.Codex:
		var cache struct {
			Models []struct {
				Slug string `json:"slug"`
			} `json:"models"`
		}
		if data, err := os.ReadFile(filepath.Join(p.Home, ".codex", "models_cache.json")); err == nil && json.Unmarshal(data, &cache) == nil {
			for _, m := range cache.Models {
				if m.Slug != "" && m.Slug != "codex-auto-review" {
					info.Models = append(info.Models, m.Slug)
				}
			}
		}
	case agentlog.Pi:
		if out, err := exec.CommandContext(ctx, bin, "--list-models").Output(); err == nil {
			sc := bufio.NewScanner(bytes.NewReader(out))
			for sc.Scan() {
				f := strings.Fields(sc.Text())
				if len(f) >= 2 && f[0] != "provider" {
					info.Models = append(info.Models, f[0]+"/"+f[1])
				}
			}
		}
	}
	return info
}

// claudeModelVars are the variables of Claude Code's settings that name
// models, as tools switching it to another provider set them.
var claudeModelVars = []string{
	"ANTHROPIC_MODEL",
	"ANTHROPIC_DEFAULT_OPUS_MODEL",
	"ANTHROPIC_DEFAULT_SONNET_MODEL",
	"ANTHROPIC_DEFAULT_HAIKU_MODEL",
	"ANTHROPIC_SMALL_FAST_MODEL",
}

// ClaudeModels are the models ~/.claude/settings.json names: its model and
// those of its env, which tools such as cc-switch write for a provider of
// other models. Without any, they are the aliases Claude Code takes for
// Anthropic's latest models.
func ClaudeModels(p Paths) []string {
	var s struct {
		Model string            `json:"model"`
		Env   map[string]string `json:"env"`
	}
	if data, err := os.ReadFile(filepath.Join(p.Home, ".claude", "settings.json")); err == nil {
		json.Unmarshal(data, &s)
	}
	var models []string
	add := func(m string) {
		if m = strings.TrimSpace(m); m != "" && !slices.Contains(models, m) {
			models = append(models, m)
		}
	}
	for _, v := range claudeModelVars {
		add(s.Env[v])
	}
	if len(models) == 0 {
		return []string{"fable", "opus", "sonnet", "haiku"}
	}
	// The settings' model may be an alias, which then means one of those.
	add(s.Model)
	return models
}
