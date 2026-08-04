package targets

import (
	"os"
	"path/filepath"
)

type HermesTarget struct{}

func (t *HermesTarget) Name() string {
	return "Hermes Agent"
}

func (t *HermesTarget) ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".hermes", "config.yaml")
}

func (t *HermesTarget) Write(modelID string) error {
	cfg := map[string]interface{}{
		"model": map[string]interface{}{
			"provider": "freemodel",
			"default":  modelID,
		},
	}
	return writeYAMLFile(t.ConfigPath(), cfg)
}
