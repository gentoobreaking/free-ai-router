package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigPathDefault(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	origEnv := os.Getenv(EnvConfigPathVar)
	defer os.Setenv(EnvConfigPathVar, origEnv)

	os.Unsetenv(EnvConfigPathVar)
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	want := filepath.Join(tmpDir, ConfigFileName)
	if path != want {
		t.Errorf("default config path = %q, want %q (spec §9.1)", path, want)
	}
}

func TestConfigPathEnvOverride(t *testing.T) {
	orig := os.Getenv(EnvConfigPathVar)
	defer os.Setenv(EnvConfigPathVar, orig)

	override := filepath.Join(t.TempDir(), "custom.json")
	os.Setenv(EnvConfigPathVar, override)

	path, err := ConfigPath()
	if err != nil {
		t.Fatalf("ConfigPath: %v", err)
	}
	if path != override {
		t.Errorf("env override config path = %q, want %q (spec §18)", path, override)
	}
}

func TestConfigPathEnvOverrideRoundTrip(t *testing.T) {
	orig := os.Getenv(EnvConfigPathVar)
	defer os.Setenv(EnvConfigPathVar, orig)

	dir := t.TempDir()
	override := filepath.Join(dir, "nested", "config.json")
	os.Setenv(EnvConfigPathVar, override)

	cfg := DefaultConfig()
	cfg.APIKeys["groq"] = "gsk-env-test"

	if err := Save(cfg); err != nil {
		t.Fatalf("Save with env override: %v", err)
	}
	if _, err := os.Stat(override); err != nil {
		t.Fatalf("config should be written to env path: %v", err)
	}

	loaded, err := Load()
	if err != nil {
		t.Fatalf("Load with env override: %v", err)
	}
	if loaded.APIKeys["groq"] != "gsk-env-test" {
		t.Error("config round-trip through env path failed")
	}

	info, err := os.Stat(override)
	if err == nil && info.Mode().Perm() != 0600 {
		t.Errorf("config file should be 0600, got %v", info.Mode().Perm())
	}
}

func TestLegacyMigrationWritesNewConfig(t *testing.T) {
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	origEnv := os.Getenv(EnvConfigPathVar)
	defer os.Setenv(EnvConfigPathVar, origEnv)

	os.Unsetenv(EnvConfigPathVar)
	tmpDir := t.TempDir()
	os.Setenv("HOME", tmpDir)

	legacyPath := filepath.Join(tmpDir, LegacyConfigFileName)
	legacy := `{"apiKeys":{"nvidia":"nvapi-legacy"},"providers":{"nvidia":{"enabled":false}}}`
	if err := os.WriteFile(legacyPath, []byte(legacy), 0600); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load with legacy config: %v", err)
	}
	if cfg.APIKeys["nvidia"] != "nvapi-legacy" {
		t.Errorf("legacy keys should migrate, got %v", cfg.APIKeys)
	}

	// Spec §22.2: migration must persist to the new location.
	newPath := filepath.Join(tmpDir, ConfigFileName)
	if _, err := os.Stat(newPath); err != nil {
		t.Fatalf("migrated config should be written to %s: %v", newPath, err)
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Errorf("legacy config should be renamed after migration, stat err: %v", err)
	}
}
