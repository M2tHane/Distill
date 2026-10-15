package env

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/M2tHane/Distill/agentlog"
)

// Efforts are the reasoning effort levels a model takes, in the agent's
// words, from the least. The model "" is the agent's default one.
func (a AgentInfo) Efforts(model string) []string {
	if a.Agent == agentlog.Claude {
		return claudeEfforts(model)
	}
	if model == "" {
		model = a.DefaultModel
	}
	return a.efforts[strings.ToLower(model)]
}

// claudeEfforts are the levels of Claude Code's --effort the model takes.
// An alias or a model of another provider gets them all; Claude Code
// lowers what a model does not take.
func claudeEfforts(model string) []string {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "sonnet-4-5"), strings.Contains(m, "haiku-4-5"), strings.Contains(m, "-3-"):
		return nil
	case strings.Contains(m, "opus-4-5"):
		return []string{"low", "medium", "high"}
	case strings.Contains(m, "4-6"):
		return []string{"low", "medium", "high", "max"}
	}
	return []string{"low", "medium", "high", "xhigh", "max"}
}

var tomlModelRe = regexp.MustCompile(`(?m)^model\s*=\s*"([^"]*)"`)

// codexEfforts reads the levels of Codex's models from its models cache,
// and its default model from its config.
func codexEfforts(p Paths, info *AgentInfo) {
	var cache struct {
		Models []struct {
			Slug   string `json:"slug"`
			Levels []struct {
				Effort string `json:"effort"`
			} `json:"supported_reasoning_levels"`
		} `json:"models"`
	}
	if data, err := os.ReadFile(filepath.Join(p.Home, ".codex", "models_cache.json")); err == nil {
		json.Unmarshal(data, &cache)
	}
	info.efforts = map[string][]string{}
	for _, m := range cache.Models {
		var levels []string
		for _, l := range m.Levels {
			levels = append(levels, l.Effort)
		}
		info.efforts[strings.ToLower(m.Slug)] = levels
	}
	if data, err := os.ReadFile(filepath.Join(p.Home, ".codex", "config.toml")); err == nil {
		if m := tomlModelRe.FindSubmatch(data); m != nil {
			info.DefaultModel = string(m[1])
		}
	}
}

// piLevels are Pi's thinking levels, from the least.
var piLevels = []string{"off", "minimal", "low", "medium", "high", "xhigh", "max"}

// piModel is a model as Pi's model files describe it.
type piModel struct {
	ID       string `json:"id"`
	Provider string `json:"provider"`
	// Reasoning is a pointer as Pi takes a model without it as one that
	// does not think.
	Reasoning *bool `json:"reasoning"`
	// LevelMap maps a level to the model's own; null means it does not
	// take the level. xhigh and max are taken only when mapped.
	LevelMap map[string]*string `json:"thinkingLevelMap"`
}

// levels are the thinking levels Pi offers for the model, by its rule.
func (m piModel) levels() []string {
	if m.Reasoning == nil || !*m.Reasoning {
		return nil
	}
	var levels []string
	for _, l := range piLevels {
		mapped, ok := m.LevelMap[l]
		if ok && mapped == nil || (l == "xhigh" || l == "max") && !ok {
			continue
		}
		levels = append(levels, l)
	}
	return levels
}

// piEfforts reads the levels of Pi's models from its model files: the
// providers the user added and the catalog of built-in ones. thinks are
// the models `pi --list-models` says think, for those the files lack.
func piEfforts(p Paths, info *AgentInfo, thinks map[string]bool) {
	dir := filepath.Join(p.Home, ".pi", "agent")
	info.efforts = map[string][]string{}
	// A later entry of a model replaces an earlier one, as in Pi.
	add := func(provider string, m piModel) {
		info.efforts[strings.ToLower(provider+"/"+m.ID)] = m.levels()
	}
	var store map[string]struct {
		Models []piModel `json:"models"`
	}
	if data, err := os.ReadFile(filepath.Join(dir, "models-store.json")); err == nil && json.Unmarshal(data, &store) == nil {
		for provider, s := range store {
			for _, m := range s.Models {
				add(provider, m)
			}
		}
	}
	var custom struct {
		Providers map[string]struct {
			Models []piModel `json:"models"`
		} `json:"providers"`
	}
	if data, err := os.ReadFile(filepath.Join(dir, "models.json")); err == nil && json.Unmarshal(data, &custom) == nil {
		for provider, s := range custom.Providers {
			for _, m := range s.Models {
				add(provider, m)
			}
		}
	}
	for _, model := range info.Models {
		key := strings.ToLower(model)
		if _, ok := info.efforts[key]; !ok && thinks[model] {
			info.efforts[key] = slices.Clone(piLevels[:5])
		}
	}
	var settings struct {
		Provider string `json:"defaultProvider"`
		Model    string `json:"defaultModel"`
	}
	if data, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil && json.Unmarshal(data, &settings) == nil && settings.Model != "" {
		info.DefaultModel = settings.Provider + "/" + settings.Model
	}
}
