package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/freemodel/router/internal/config"
)

// TestVersionNewer: numeric comparison, not string equality (T051).
func TestVersionNewer(t *testing.T) {
	cases := []struct {
		latest  string
		current string
		want    bool
	}{
		{"v0.10", "v0.9", true},
		{"v0.9", "v0.10", false},
		{"v0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v1.0.0", "v0.9.9", true},
		{"v0.2.0", "v0.1.9", true},
		{"0.2.0", "v0.1.9", true},
	}
	for _, tc := range cases {
		if got := versionNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("versionNewer(%q, %q) = %v, want %v", tc.latest, tc.current, got, tc.want)
		}
	}
}

// TestParseSemver: missing parts default to zero.
func TestParseSemver(t *testing.T) {
	if m, mi, p := parseSemver("v1.2.3"); m != 1 || mi != 2 || p != 3 {
		t.Errorf("parseSemver(v1.2.3) = %d.%d.%d", m, mi, p)
	}
	if m, mi, p := parseSemver("v0.10"); m != 0 || mi != 10 || p != 0 {
		t.Errorf("parseSemver(v0.10) = %d.%d.%d", m, mi, p)
	}
}

// TestSHA256Verify: sha256Of computes the expected hash (T051).
func TestSHA256Verify(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")
	os.WriteFile(path, []byte("hello world"), 0600)

	sum, err := sha256Of(path)
	if err != nil {
		t.Fatalf("sha256Of: %v", err)
	}
	// sha256("hello world") = b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9
	if sum != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Errorf("sha256 = %s, want known hash", sum)
	}
}

// TestConfigAddKeyDoesNotPersistEnvKey: adding a key for an env-configured
// provider must not write the env key into the config file (T054).
func TestConfigAddKeyDoesNotPersistEnvKey(t *testing.T) {
	t.Setenv("FREMODEL_CONFIG_PATH", filepath.Join(t.TempDir(), "cfg.json"))
	t.Setenv("NVIDIA_API_KEY", "env-key")

	if err := config.Save(config.DefaultConfig()); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := configAddKey("nvidia", "new-key"); err != nil {
		t.Fatalf("configAddKey: %v", err)
	}

	loaded, err := config.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	keys := config.KeysFromConfig("nvidia", loaded)
	if len(keys) != 1 || keys[0] != "new-key" {
		t.Errorf("config keys = %v, want exactly [new-key]", keys)
	}
}

// TestConfigRemoveKeyMissingConfigKey: removing a key that only exists in the
// environment must error (T054).
func TestConfigRemoveKeyMissingConfigKey(t *testing.T) {
	t.Setenv("FREMODEL_CONFIG_PATH", filepath.Join(t.TempDir(), "cfg.json"))
	t.Setenv("NVIDIA_API_KEY", "env-key")

	cfg := config.DefaultConfig()
	cfg.APIKeys["nvidia"] = "real-key"
	if err := config.Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if err := configRemoveKey("nvidia", "not-the-key"); err == nil {
		t.Error("remove-key for a missing config key should error")
	}
	// Removing the actual config key succeeds even though an env key exists.
	if err := configRemoveKey("nvidia", "real-key"); err != nil {
		t.Errorf("remove-key for existing config key should succeed: %v", err)
	}
}

// TestKeysFromConfigIgnoresEnv: KeysFromConfig never returns env keys.
func TestKeysFromConfigIgnoresEnv(t *testing.T) {
	t.Setenv("NVIDIA_API_KEY", "env-key")
	cfg := config.DefaultConfig()
	if got := config.KeysFromConfig("nvidia", cfg); len(got) != 0 {
		t.Errorf("KeysFromConfig = %v, want none", got)
	}
	cfg.APIKeys["nvidia"] = []interface{}{"cfg-key-1"}
	if got := config.KeysFromConfig("nvidia", cfg); len(got) != 1 || got[0] != "cfg-key-1" {
		t.Errorf("KeysFromConfig = %v, want [cfg-key-1]", got)
	}
}
