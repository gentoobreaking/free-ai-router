package main

import (
	"fmt"
	"time"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/providers"
)

func main() {
	provMgr := providers.NewManager()
	if err := provMgr.LoadSources(providers.DataDir() + "/data/sources.json"); err != nil {
		fmt.Printf("Failed to load sources: %v\n", err)
		return
	}

	registry := models.NewRegistry()
	registry.LoadFromSources(provMgr)

	cfg, err := config.Load()
	if err != nil {
		fmt.Printf("Config load error (using defaults): %v\n", err)
		cfg = config.DefaultConfig()
	}

	for _, m := range registry.GetAll() {
		m.APIKey = config.ResolveAPIKey(m.Provider, cfg)
	}

	// Set correct endpoint for pollinations
	for _, m := range registry.GetAll() {
		if m.Provider == "pollinations" {
			m.Endpoint = "https://gen.pollinations.ai/v1/chat/completions"
		}
	}

	engine := ping.NewEngine(nil)
	engine.SetRegistry(registry)
	engine.SetModels(registry.GetAll())
	engine.SetInterval(2 * time.Second)
	engine.Start()

	time.Sleep(8 * time.Second)

	engine.Stop()

	results := registry.Snapshot()

	for _, m := range results {
		if m.Status == "up" || (m.ID != "") {
			fmt.Printf("%-40s | %-15s | %-10s | %5dms | %5.0f%%\n",
				m.ID, m.Provider, m.Status, int(m.AvgLatency), m.Uptime)
		}
	}

	upCount := 0
	noKeyCount := 0
	pollinationUp := 0
	for _, m := range results {
		if m.Status == "up" {
			upCount++
		}
		if m.Status == "noauth" {
			noKeyCount++
		}
		if m.Provider == "pollinations" && m.Status == "up" {
			pollinationUp++
		}
	}

	fmt.Printf("\n--- Summary ---\n")
	fmt.Printf("Total models loaded: %d\n", len(results))
	fmt.Printf("Up and reachable: %d\n", upCount)
	fmt.Printf("No auth needed (noauth): %d\n", noKeyCount)
	fmt.Printf("Pollinations AI up: %d\n", pollinationUp)
}
