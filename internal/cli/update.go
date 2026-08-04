package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/freemodel/router/internal/config"
)

const updateCheckInterval = 24 * time.Hour

type releaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

func CheckForUpdate(force bool) (string, error) {
	tarball := os.Getenv("FREMODEL_UPDATE_TARBALL")
	if tarball != "" {
		return tarball, nil
	}

	resp, err := http.Get("https://api.github.com/repos/freemodel/router/releases/latest")
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update check returned %d", resp.StatusCode)
	}

	var rel releaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", fmt.Errorf("failed to parse release info: %w", err)
	}

	current := strings.TrimPrefix(Version, "v")
	latest := strings.TrimPrefix(rel.TagName, "v")

	if latest == current {
		return "", nil
	}

	return rel.HTMLURL, nil
}

func RunUpdate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if isGitSource() {
		fmt.Println("Running from source (git). Auto-update disabled. Use `git pull` to update.")
		return nil
	}

	updateURL, err := CheckForUpdate(true)
	if err != nil {
		return err
	}
	if updateURL == "" {
		now := time.Now().Format(time.RFC3339)
		cfg.AutoUpdate.LastCheckAt = now
		cfg.AutoUpdate.LastError = nil
		_ = config.Save(cfg)
		fmt.Println("Already on the latest version:", Version)
		return nil
	}

	fmt.Println("Update available. Downloading...")
	if err := applyUpdate(updateURL); err != nil {
		errStr := err.Error()
		cfg.AutoUpdate.LastError = &errStr
		_ = config.Save(cfg)
		return err
	}

	now := time.Now().Format(time.RFC3339)
	cfg.AutoUpdate.LastCheckAt = now
	cfg.AutoUpdate.LastUpdateAt = &now
	cfg.AutoUpdate.LastError = nil
	_ = config.Save(cfg)

	fmt.Println("Update applied. Restarting...")
	restartProcess()
	return nil
}

func RunAutoUpdate(cmd string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	switch cmd {
	case "enable":
		cfg.AutoUpdate.Enabled = true
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Println("Auto-update enabled.")
	case "disable":
		cfg.AutoUpdate.Enabled = false
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Println("Auto-update disabled.")
	case "status":
		fmt.Printf("Auto-update: %v (interval: %dh)\n", cfg.AutoUpdate.Enabled, cfg.AutoUpdate.IntervalHours)
	default:
		fmt.Println("Usage: freemodel autoupdate [enable|disable|status]")
	}

	return nil
}

func isGitSource() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	// When built with `go run` or from a source checkout
	if strings.Contains(exe, "go-build") || strings.Contains(exe, os.TempDir()) {
		return true
	}
	exePath := filepath.Dir(exe)
	for _, marker := range []string{".git", "go.mod"} {
		if _, err := os.Stat(filepath.Join(exePath, marker)); err == nil {
			return true
		}
	}
	return false
}

func applyUpdate(source string) error {
	tarball := os.Getenv("FREMODEL_UPDATE_TARBALL")
	assetURL := tarball
	if assetURL == "" {
		// Try a predictable asset naming scheme from GitHub releases
		assetURL = fmt.Sprintf("https://github.com/freemodel/router/releases/latest/download/%s",
			assetName())
	}
	if source != "" && !strings.HasPrefix(source, "http") {
		assetURL = source
	}

	tmpDir, err := os.MkdirTemp("", "freemodel-update-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, "update.archive")
	if err := downloadFile(assetURL, archivePath); err != nil {
		return fmt.Errorf("failed to download update: %w", err)
	}

	binary, err := extractBinary(archivePath, tmpDir)
	if err != nil {
		return fmt.Errorf("failed to extract update: %w", err)
	}

	currentExe, err := os.Executable()
	if err != nil {
		return err
	}

	newData, err := os.ReadFile(binary)
	if err != nil {
		return err
	}

	backupPath := currentExe + ".old"
	_ = os.Remove(backupPath)
	if err := os.Rename(currentExe, backupPath); err != nil {
		// Windows: can't replace running binary; use temp + rename pattern
		return fmt.Errorf("failed to back up current binary: %w", err)
	}

	if err := os.WriteFile(currentExe, newData, 0755); err != nil {
		_ = os.Rename(backupPath, currentExe)
		return fmt.Errorf("failed to write new binary: %w", err)
	}

	_ = os.Remove(backupPath)
	return nil
}

func assetName() string {
	suffix := ""
	switch runtime.GOOS {
	case "darwin":
		suffix = "darwin-"
	case "linux":
		suffix = "linux-"
	case "windows":
		suffix = "windows-"
	}
	switch runtime.GOARCH {
	case "amd64":
		suffix += "amd64"
	case "arm64":
		suffix += "arm64"
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	return "freemodel-" + suffix + ext
}

func downloadFile(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned %d", resp.StatusCode)
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractBinary(archivePath, destDir string) (string, error) {
	if strings.HasSuffix(archivePath, ".tar.gz") || strings.HasSuffix(archivePath, ".tgz") {
		return extractTarGz(archivePath, destDir)
	}
	if strings.HasSuffix(archivePath, ".zip") {
		return extractZip(archivePath, destDir)
	}
	// Assume raw binary
	return archivePath, nil
}

func extractTarGz(archivePath, destDir string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if strings.HasPrefix(filepath.Base(hdr.Name), "freemodel") {
			outPath := filepath.Join(destDir, "freemodel")
			out, err := os.Create(outPath)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			_ = os.Chmod(outPath, 0755)
			return outPath, nil
		}
	}
	return "", fmt.Errorf("no binary found in tarball")
}

func extractZip(archivePath, destDir string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if strings.HasPrefix(filepath.Base(f.Name), "freemodel") {
			rc, err := f.Open()
			if err != nil {
				return "", err
			}
			defer rc.Close()

			outPath := filepath.Join(destDir, "freemodel.exe")
			out, err := os.Create(outPath)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(out, rc); err != nil {
				out.Close()
				return "", err
			}
			out.Close()
			return outPath, nil
		}
	}
	return "", fmt.Errorf("no binary found in zip")
}

func restartProcess() {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if _, err := os.Stat(exe); err != nil {
		return
	}
	args := append([]string{}, os.Args[1:]...)
	if err := syscallExec(exe, args); err != nil {
		fmt.Println("Automatic restart failed. Please restart manually.")
	}
}
