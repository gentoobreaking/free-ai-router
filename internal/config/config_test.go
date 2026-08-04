package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg == nil {
		t.Fatal("DefaultConfig returned nil")
	}
	if !cfg.CodingOnly {
		t.Error("CodingOnly should default to true")
	}
	if !cfg.AutoPingEnabled {
		t.Error("AutoPingEnabled should default to true")
	}
	if cfg.PinningMode != "canonical" {
		t.Errorf("PinningMode should be canonical, got %s", cfg.PinningMode)
	}
	if cfg.UI.ScrollSortPauseMs != 1500 {
		t.Errorf("ScrollSortPauseMs should be 1500, got %d", cfg.UI.ScrollSortPauseMs)
	}
	if cfg.AutoUpdate.IntervalHours != 24 {
		t.Errorf("AutoUpdate interval should be 24, got %d", cfg.AutoUpdate.IntervalHours)
	}
}

func TestNormalizeConfigShape(t *testing.T) {
	cfg := &Config{}
	normalized := normalizeConfig(cfg)
	if normalized.APIKeys == nil {
		t.Error("APIKeys should be initialized")
	}
	if normalized.Providers == nil {
		t.Error("Providers should be initialized")
	}
	if normalized.BannedModels == nil {
		t.Error("BannedModels should be initialized")
	}
	if normalized.ModelTags == nil {
		t.Error("ModelTags should be initialized")
	}
	if normalized.PinningMode != "canonical" {
		t.Error("PinningMode should default to canonical")
	}
	if normalized.UI.ScrollSortPauseMs != 1500 {
		t.Error("ScrollSortPauseMs should default to 1500")
	}
}

func TestExportImportToken(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKeys["nvidia"] = "nvapi-test"

	token, err := ExportToken(cfg)
	if err != nil {
		t.Fatalf("ExportToken: %v", err)
	}

	if !strings.HasPrefix(token, "mrconf:v1:") {
		t.Errorf("token should have mrconf:v1: prefix, got %q", token)
	}

	imported, err := ImportToken(token)
	if err != nil {
		t.Fatalf("ImportToken: %v", err)
	}

	if imported.APIKeys["nvidia"] != "nvapi-test" {
		t.Error("imported config should preserve apiKeys")
	}
}

func TestImportTokenInvalid(t *testing.T) {
	_, err := ImportToken("invalid-token")
	if err == nil {
		t.Error("should reject invalid token")
	}
}

func TestSaveAndLoad(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	cfg := DefaultConfig()
	cfg.APIKeys["groq"] = "gsk_test"
	cfg.CodingOnly = false

	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.APIKeys["groq"] != "gsk_test" {
		t.Error("loaded config should preserve groq key")
	}
	if loaded.CodingOnly {
		t.Error("loaded config should preserve codingOnly=false")
	}
}

func TestSaveFilePermissions(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	path, _ := ConfigPath()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0600 {
		t.Errorf("config file should be 0600, got %v", info.Mode().Perm())
	}
}

func TestLoadCorruptConfig(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	path, _ := ConfigPath()
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte("{invalid json"), 0600)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with corrupt config should fall back to defaults: %v", err)
	}
	if cfg == nil {
		t.Fatal("should return default config on corruption")
	}
}

func TestResolveAPIKeyEnvOverride(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKeys["nvidia"] = "config-key"

	orig := os.Getenv("NVIDIA_API_KEY")
	defer os.Setenv("NVIDIA_API_KEY", orig)

	os.Setenv("NVIDIA_API_KEY", "env-key")
	if got := ResolveAPIKey("nvidia", cfg); got != "env-key" {
		t.Errorf("env var should override config, got %q", got)
	}

	os.Unsetenv("NVIDIA_API_KEY")
	if got := ResolveAPIKey("nvidia", cfg); got != "config-key" {
		t.Errorf("config key should be used when env unset, got %q", got)
	}
}

func TestResolveAPIKeysMultiAccount(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIKeys["openrouter"] = []interface{}{"sk-or-1", "sk-or-2"}

	keys := ResolveAPIKeys("openrouter", cfg)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d", len(keys))
	}
	if keys[0] != "sk-or-1" || keys[1] != "sk-or-2" {
		t.Error("keys should be in order")
	}
}

func TestMigrateLegacyConfig(t *testing.T) {
	legacy := &Config{
		APIKeys:   map[string]interface{}{"nvidia": "nvapi-legacy"},
		Providers: map[string]ProviderConfig{},
	}

	migrated := migrateLegacy(legacy)
	if migrated.APIKeys["nvidia"] != "nvapi-legacy" {
		t.Error("legacy API keys should be migrated")
	}
}

func TestConfigJSONSchema(t *testing.T) {
	raw := `{
		"apiKeys": {"nvidia": "nvapi-xxx", "openrouter": ["sk-or-xxx", "sk-or-yyy"]},
		"providers": {
			"nvidia": {"enabled": true},
			"openai-compatible:my-vllm": {
				"enabled": true, "name": "Local vLLM",
				"baseUrl": "http://localhost:8000/v1",
				"modelId": "qwen-coder",
				"discoverModels": true,
				"maxTurns": 20
			}
		},
		"bannedModels": [],
		"autoUpdate": {"enabled": true, "intervalHours": 24},
		"minSweScore": null,
		"excludedProviders": [],
		"pinningMode": "canonical",
		"modelTags": {"deepseek-ai/deepseek-v3.2": ["coding"]},
		"autoPingEnabled": true,
		"codingOnly": true,
		"ui": {"scrollSortPauseMs": 1500}
	}`

	var cfg Config
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		t.Fatalf("schema should parse: %v", err)
	}
	if !cfg.CodingOnly {
		t.Error("codingOnly should parse as true")
	}
	if cfg.Providers["openai-compatible:my-vllm"].MaxTurns != 20 {
		t.Error("maxTurns should parse")
	}
	if len(cfg.ModelTags["deepseek-ai/deepseek-v3.2"]) != 1 {
		t.Error("modelTags should parse")
	}
}
