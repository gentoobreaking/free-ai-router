package cli

import (
	"fmt"

	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

const BestPingRounds = 4

func RunBest(registry *models.Registry, resolveKey func(provider string) string) (string, error) {
	all := registry.GetAll()
	if len(all) == 0 {
		return "", fmt.Errorf("no models in catalog")
	}

	engine := ping.NewEngine(nil)
	engine.SetModels(registry.GetAll())

	for i := 0; i < BestPingRounds; i++ {
		engine.PingAllOnce(i == 0)
	}
	engine.Stop()

	var best *models.Model
	bestScore := -1.0
	for _, m := range registry.GetAll() {
		if m.Status != "up" {
			continue
		}
		score := m.Uptime*1000 - m.AvgLatency
		if score > bestScore {
			bestScore = score
			best = m
		}
	}

	if best == nil {
		return "", fmt.Errorf("no reachable models found")
	}

	fmt.Println(best.ID)
	return best.ID, nil
}
