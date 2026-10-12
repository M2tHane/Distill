package env

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestClaudeModels(t *testing.T) {
	home := t.TempDir()
	p := Paths{Home: home}
	if got := ClaudeModels(p); !slices.Equal(got, []string{"fable", "opus", "sonnet", "haiku"}) {
		t.Errorf("without settings: %v", got)
	}
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	write := func(s string) {
		os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(s), 0o644)
	}
	write(`{"model":"opus","env":{}}`)
	if got := ClaudeModels(p); !slices.Equal(got, []string{"fable", "opus", "sonnet", "haiku"}) {
		t.Errorf("official: %v", got)
	}
	write(`{"model":"opus","env":{"ANTHROPIC_BASE_URL":"https://x","ANTHROPIC_MODEL":"glm-5.3","ANTHROPIC_DEFAULT_OPUS_MODEL":"glm-5.3","ANTHROPIC_DEFAULT_HAIKU_MODEL":"glm-5.3-air"}}`)
	if got := ClaudeModels(p); !slices.Equal(got, []string{"glm-5.3", "glm-5.3-air", "opus"}) {
		t.Errorf("third party: %v", got)
	}
}
