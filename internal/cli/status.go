package cli

import (
	"fmt"

	"github.com/freemodel/router/internal/config"
)

func RunStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	fmt.Println("FreeModel Router Status")
	fmt.Println("========================")
	fmt.Println()

	fmt.Println("Configured Providers:")
	if len(cfg.Providers) == 0 {
		fmt.Println("  (none configured)")
	}
	for provider, pcfg := range cfg.Providers {
		state := "off"
		if pcfg.Enabled {
			state = "on"
		}
		keys := config.ResolveAPIKeys(provider, cfg)
		keyCount := len(keys)
		fmt.Printf("  %-24s [%s] keys: %d\n", provider, state, keyCount)
	}

	fmt.Println()
	fmt.Println("Auto-update:")
	fmt.Printf("  enabled: %v, interval: %dh\n", cfg.AutoUpdate.Enabled, cfg.AutoUpdate.IntervalHours)

	fmt.Println()
	fmt.Println("Router:")
	fmt.Printf("  http://127.0.0.1:%d/v1\n", config.GetPort(7352))

	return nil
}
