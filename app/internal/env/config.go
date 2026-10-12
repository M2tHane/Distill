package env

import (
	"encoding/json"
	"os"

	"github.com/M2tHane/Distill/agentlog"
)

// Config is what the user chose, kept in ~/.distill/config.json.
type Config struct {
	Agents map[agentlog.Agent]*AgentConfig `json:"agents"`
	// SkillsCopy applies skills as copies instead of symbolic links.
	SkillsCopy bool `json:"skillsCopy"`
	// BuiltinAsked is set once the user was asked where to install the
	// built-in skills.
	BuiltinAsked bool `json:"builtinAsked"`
	// Runner is the agent last used to distill.
	Runner agentlog.Agent `json:"runner"`
}

// AgentConfig is what the user chose for an agent.
type AgentConfig struct {
	Enabled bool   `json:"enabled"`
	Model   string `json:"model"` // empty: the agent's default
}

// LoadConfig reads the configuration, with defaults for what it lacks.
func LoadConfig(p Paths) *Config {
	cfg := &Config{}
	if data, err := os.ReadFile(p.Config); err == nil {
		json.Unmarshal(data, cfg)
	}
	if cfg.Agents == nil {
		cfg.Agents = map[agentlog.Agent]*AgentConfig{}
	}
	for _, a := range Agents {
		if cfg.Agents[a] == nil {
			cfg.Agents[a] = &AgentConfig{}
		}
	}
	return cfg
}

// Save writes the configuration.
func (cfg *Config) Save(p Paths) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(p.Root, 0o755); err != nil {
		return err
	}
	tmp := p.Config + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p.Config)
}
