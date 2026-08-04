package targets

import (
	"os"
	"path/filepath"
)

type OpenClawTarget struct{}

func (t *OpenClawTarget) Name() string {
	return "OpenClaw"
}

func (t *OpenClawTarget) ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".openclaw", "openclaw.json")
}

func (t *OpenClawTarget) Write(modelID string) error {
	cfg := map[string]interface{}{
		"models": map[string]interface{}{
			"providers": map[string]interface{}{
				"freemodel": map[string]interface{}{
					"baseUrl": RouterBaseURL,
					"api":     "openai-completions",
					"apiKey":  "no-key",
					"models": []map[string]string{
						{"id": modelID, "name": "Freemodel Router"},
					},
				},
			},
		},
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model": map[string]string{
					"primary": "freemodel/" + modelID,
				},
				"models": map[string]interface{}{
					"freemodel/" + modelID: map[string]interface{}{},
				},
			},
		},
	}
	return writeJSONFile(t.ConfigPath(), cfg)
}
