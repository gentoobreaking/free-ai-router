package targets

import (
	"os"
	"path/filepath"
)

type PiTarget struct{}

func (t *PiTarget) Name() string {
	return "Pi Agent"
}

func (t *PiTarget) ConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".pi", "pi.json")
}

func (t *PiTarget) Write(modelID string) error {
	cfg := map[string]interface{}{
		"model_list": []map[string]interface{}{
			{
				"model_name": "freemodel",
				"model":      "openai/" + modelID,
				"api_base":   RouterBaseURL,
			},
		},
		"agents": map[string]interface{}{
			"defaults": map[string]interface{}{
				"model_name": "freemodel",
			},
		},
	}
	return writeJSONFile(t.ConfigPath(), cfg)
}
