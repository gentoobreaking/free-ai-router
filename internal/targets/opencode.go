package targets

import (
	"os"
	"path/filepath"
)

type OpenCodeTarget struct{}

func (t *OpenCodeTarget) Name() string {
	return "OpenCode"
}

func (t *OpenCodeTarget) ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opencode", "opencode.json")
}

func (t *OpenCodeTarget) Write(modelID string) error {
	cfg := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]interface{}{
			"router": map[string]interface{}{
				"npm":  "@ai-sdk/openai-compatible",
				"name": "freemodel",
				"options": map[string]interface{}{
					"baseURL": RouterBaseURL,
					"apiKey":  "dummy-key",
				},
				"models": map[string]interface{}{
					modelID: map[string]interface{}{
						"name": "Freemodel Router",
					},
				},
			},
		},
		"model": "router/" + modelID,
	}
	return writeJSONFile(t.ConfigPath(), cfg)
}
