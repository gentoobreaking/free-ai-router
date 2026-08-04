package models

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type OpenRouterModel struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	ContextLength int    `json:"context_length"`
	Pricing       struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
	} `json:"pricing"`
	Benchmarks struct {
		ArtificialAnalysis struct {
			CodingIndex float64 `json:"coding_index"`
		} `json:"artificial_analysis"`
	} `json:"benchmarks"`
}

func readFile(path string) ([]byte, error) {
	return os.ReadFile(path)
}

func dataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

func DataPath(name string) string {
	return filepath.Join(dataDir(), "data", name)
}

type ScoringConfig struct {
	AAIndex       float64
	ArenaElo      float64
	Popularity    float64
	Recency       float64
	Features      float64
	ContextLength int
}

func ResolveQualityScore(canonicalID string, orModel *OpenRouterModel, offlineScores map[string]float64, sc *ScoringConfig) float64 {
	if orModel != nil && orModel.Benchmarks.ArtificialAnalysis.CodingIndex > 0 {
		return orModel.Benchmarks.ArtificialAnalysis.CodingIndex / 100
	}

	if sc != nil && sc.ArenaElo > 0 {
		return arenaToCodingScore(sc.ArenaElo)
	}

	if sc != nil {
		if score := metadataHeuristic(sc); score > 0 {
			return score
		}
	}

	if score, ok := offlineScores[canonicalID]; ok {
		return score
	}

	return 0.45
}

func arenaToCodingScore(elo float64) float64 {
	min := 1000.0
	max := 1400.0
	score := (elo - min) / (max - min)
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

func metadataHeuristic(sc *ScoringConfig) float64 {
	score := 0.0
	if sc.Popularity > 0 {
		score += sc.Popularity * 0.3
	}
	if sc.Recency > 0 {
		score += sc.Recency * 0.2
	}
	if sc.Features > 0 {
		score += sc.Features * 0.2
	}
	if sc.ContextLength > 0 {
		ctxScore := float64(sc.ContextLength) / 1000000
		if ctxScore > 0.3 {
			ctxScore = 0.3
		}
		score += ctxScore
	}
	return score
}

func IsFreeModel(m *OpenRouterModel) bool {
	return m.Pricing.Prompt == "0" && m.Pricing.Completion == "0"
}

func FetchOpenRouterCatalog(apiKey string) ([]OpenRouterModel, error) {
	url := "https://openrouter.ai/api/v1/models"
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OpenRouter catalog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("OpenRouter returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []OpenRouterModel `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	return result.Data, nil
}
