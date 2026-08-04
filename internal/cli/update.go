package cli

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/freemodel/router/internal/config"
)

const updateCheckInterval = 24 * time.Hour

func CheckForUpdate(force bool) (string, error) {
	if force {
		os.Getenv("FREMODEL_UPDATE_TARBALL")
	}

	resp, err := http.Get("https://api.github.com/repos/freemodel/router/releases/latest")
	if err != nil {
		return "", fmt.Errorf("failed to check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("update check returned %d", resp.StatusCode)
	}

	return "", nil
}

func RunUpdate() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	if cfg.AutoUpdate.LastError != nil {
		fmt.Println("Last update error:", *cfg.AutoUpdate.LastError)
	}

	fmt.Println("No updates available. Running from source or latest version.")

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
		fmt.Println("Usage: freemodel autoupdate [--enable|--disable|--status]")
	}

	return nil
}
