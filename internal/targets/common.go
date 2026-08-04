package targets

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const RouterBaseURL = "http://127.0.0.1:7352/v1"

type Target interface {
	Name() string
	ConfigPath() string
	Write(modelID string) error
}

func backupIfExists(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	ts := time.Now().Format("20060102-150405")
	backupPath := path + ".backup-" + ts
	return os.WriteFile(backupPath, data, 0600)
}

func writeJSONFile(path string, v interface{}) error {
	if err := backupIfExists(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func writeYAMLFile(path string, v interface{}) error {
	if err := backupIfExists(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}

func HomePath(rel string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, rel), nil
}

func IsInstalled(binary string) bool {
	if binary == "" {
		return false
	}
	_, err := exec.LookPath(binary)
	return err == nil
}
