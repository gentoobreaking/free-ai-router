package main

import (
	"fmt"
	"log"
	"time"

	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/providers"
)

func main() {
	provMgr := providers.NewManager()
	
	fmt.Println("Loading sources...")
	start := time.Now()
	
	// First load sources
	if err := provMgr.LoadSources(providers.DataDir() + "/data/sources.json"); err != nil {
		log.Fatalf("failed to load sources: %v", err)
	}
	
	fmt.Printf("LoadSources took %v\n", time.Since(start))
	
	// List all providers
	fmt.Println("\nProviders:")
	for key, p := range provMgr.GetAllProviders() {
		fmt.Printf("  %s: %d models, discoverable=%v, baseURL=%s\n", 
			key, len(p.Models), p.Discoverable, p.BaseURL)
	}
	
	// Check what's in the registry
	registry := models.NewRegistry()
	registry.LoadFromSources(provMgr)
	
	allModels := registry.GetAll()
	fmt.Printf("\nRegistry has %d total models\n", len(allModels))
	
	// Count by provider
	counts := make(map[string]int)
	for _, m := range allModels {
		counts[m.Provider]++
	}
	
	for provider, count := range counts {
		fmt.Printf("  %s: %d models\n", provider, count)
	}
}