package targets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func withHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	orig := os.Getenv("HOME")
	t.Setenv("HOME", home)
	_ = orig
	return home
}

func TestOpenCodeWrite(t *testing.T) {
	home := withHome(t)
	target := &OpenCodeTarget{}

	if err := target.Write("auto-fastest"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(home, ".config", "opencode", "opencode.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	if cfg["$schema"] != "https://opencode.ai/config.json" {
		t.Error("missing $schema")
	}
	if cfg["model"] != "router/auto-fastest" {
		t.Errorf("model should be router/auto-fastest, got %v", cfg["model"])
	}
}

func TestOpenClawWrite(t *testing.T) {
	home := withHome(t)
	target := &OpenClawTarget{}

	if err := target.Write("auto-fastest"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(home, ".openclaw", "openclaw.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	providers, ok := cfg["models"].(map[string]interface{})["providers"].(map[string]interface{})
	if !ok {
		t.Fatal("missing models.providers")
	}
	if _, ok := providers["freemodel"]; !ok {
		t.Error("missing freemodel provider")
	}
}

func TestHermesWrite(t *testing.T) {
	home := withHome(t)
	target := &HermesTarget{}

	if err := target.Write("auto-fastest"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(home, ".hermes", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output should be valid YAML: %v", err)
	}

	model, ok := cfg["model"].(map[string]interface{})
	if !ok {
		t.Fatal("missing model key")
	}
	if model["provider"] != "freemodel" {
		t.Errorf("provider should be freemodel, got %v", model["provider"])
	}
	if model["default"] != "auto-fastest" {
		t.Errorf("default should be auto-fastest, got %v", model["default"])
	}
}

func TestPiWrite(t *testing.T) {
	home := withHome(t)
	target := &PiTarget{}

	if err := target.Write("auto-fastest"); err != nil {
		t.Fatalf("Write: %v", err)
	}

	path := filepath.Join(home, ".pi", "pi.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("output should be valid JSON: %v", err)
	}

	modelList, ok := cfg["model_list"].([]interface{})
	if !ok || len(modelList) == 0 {
		t.Fatal("missing model_list")
	}
	entry := modelList[0].(map[string]interface{})
	if entry["model_name"] != "freemodel" {
		t.Errorf("model_name should be freemodel, got %v", entry["model_name"])
	}
}

func TestBackupIfExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	os.WriteFile(path, []byte(`{"original":true}`), 0600)

	if err := backupIfExists(path); err != nil {
		t.Fatalf("backupIfExists: %v", err)
	}

	entries, _ := os.ReadDir(dir)
	backups := 0
	for _, e := range entries {
		if e.Name() != "config.json" {
			backups++
		}
	}
	if backups != 1 {
		t.Errorf("expected 1 backup file, got %d", backups)
	}
}

func TestFallbackModel(t *testing.T) {
	if IsInstalled("this-binary-surely-does-not-exist-xyz") {
		t.Error("IsInstalled should be false for a missing binary")
	}
}
