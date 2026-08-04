package providers

import (
	"os"
	"path/filepath"
	"testing"
)

// TestDataDirResolvesInDev: on a dev machine the source directory is present,
// so DataDir() must resolve to a directory containing data/sources.json.
func TestDataDirResolvesInDev(t *testing.T) {
	dir := DataDir()
	if dir == "" {
		t.Fatal("DataDir() returned empty")
	}
	if fi, err := os.Stat(filepath.Join(dir, "data", "sources.json")); err != nil || fi.IsDir() {
		t.Fatalf("DataDir %s has no data/sources.json: %v", dir, err)
	}
}

// TestDataDirEnvOverride: FREMODEL_DATA_DIR wins over everything (T042).
func TestDataDirEnvOverride(t *testing.T) {
	t.Setenv("FREMODEL_DATA_DIR", "/tmp/fm-test-data")
	if got := DataDir(); got != "/tmp/fm-test-data" {
		t.Errorf("DataDir() = %q, want %q", got, "/tmp/fm-test-data")
	}
}
