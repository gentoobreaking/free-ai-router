package models

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeTier(t *testing.T) {
	tests := []struct {
		score float64
		want  string
	}{
		{0.75, "S+"},
		{0.65, "S"},
		{0.55, "A+"},
		{0.45, "A"},
		{0.37, "A-"},
		{0.32, "B+"},
		{0.25, "B"},
		{0.15, "C"},
	}
	for _, tt := range tests {
		if got := ComputeTier(tt.score); got != tt.want {
			t.Errorf("ComputeTier(%f) = %s, want %s", tt.score, got, tt.want)
		}
	}
}

func TestLoadAliases(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "aliases.json")
	data := map[string]string{"kimi-k2.5": "moonshotai/kimi-k2.5"}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0600)

	aliases, err := LoadAliases(path)
	if err != nil {
		t.Fatalf("LoadAliases: %v", err)
	}
	if aliases["kimi-k2.5"] != "moonshotai/kimi-k2.5" {
		t.Error("alias should resolve")
	}
}

func TestLoadScores(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "scores.json")
	data := map[string]float64{"deepseek-ai/deepseek-v3.2": 0.88}
	b, _ := json.Marshal(data)
	os.WriteFile(path, b, 0600)

	scores, err := LoadScores(path)
	if err != nil {
		t.Fatalf("LoadScores: %v", err)
	}
	if scores["deepseek-ai/deepseek-v3.2"] != 0.88 {
		t.Error("score should load")
	}
}

func TestResolveGroup(t *testing.T) {
	if ResolveGroup("nvidia/deepseek-ai/deepseek-v3.2") != "deepseek-v3.2" {
		t.Error("group should be last path segment")
	}
	if ResolveGroup("kimi-k2.5") != "kimi-k2.5" {
		t.Error("single id should be its own group")
	}
}

func TestCanonicalizeID(t *testing.T) {
	aliases := map[string]string{"kimi-k2.5": "moonshotai/kimi-k2.5"}
	if got := canonicalizeID("kimi-k2.5", aliases); got != "moonshotai/kimi-k2.5" {
		t.Errorf("canonicalize should resolve alias, got %s", got)
	}
	if got := canonicalizeID("openrouter/unknown", aliases); got != "unknown" {
		t.Errorf("unknown id should be returned as-is, got %s", got)
	}
}

func TestQualityScoreHierarchy(t *testing.T) {
	offline := map[string]float64{"model/a": 0.5}

	// 1. AA coding index takes priority
	orModel := &OpenRouterModel{}
	orModel.Benchmarks.ArtificialAnalysis.CodingIndex = 82.0
	sc := &ScoringConfig{}
	score := ResolveQualityScore("model/a", orModel, offline, sc)
	if score != 0.82 {
		t.Errorf("AA index should take priority, got %f", score)
	}

	// 2. Arena Elo regression
	orModel2 := &OpenRouterModel{}
	score = ResolveQualityScore("model/a", orModel2, offline, &ScoringConfig{ArenaElo: 1200})
	if score != 0.5 {
		t.Errorf("arena elo 1200 should map to 0.5, got %f", score)
	}

	// 3. Metadata heuristic
	score = ResolveQualityScore("model/a", orModel2, offline, &ScoringConfig{Popularity: 1, Recency: 1, Features: 1, ContextLength: 128000})
	if score <= 0 {
		t.Errorf("metadata heuristic should return positive, got %f", score)
	}

	// 4. Offline fallback
	score = ResolveQualityScore("model/a", orModel2, offline, nil)
	if score != 0.5 {
		t.Errorf("offline fallback should be 0.5, got %f", score)
	}

	// 5. Default 0.45
	score = ResolveQualityScore("model/unknown", orModel2, offline, nil)
	if score != 0.45 {
		t.Errorf("default should be 0.45, got %f", score)
	}
}

func TestIsFreeModel(t *testing.T) {
	free := &OpenRouterModel{}
	free.Pricing.Prompt = "0"
	free.Pricing.Completion = "0"
	if !IsFreeModel(free) {
		t.Error("zero pricing should be free")
	}

	paid := &OpenRouterModel{}
	paid.Pricing.Prompt = "0.0001"
	paid.Pricing.Completion = "0.0001"
	if IsFreeModel(paid) {
		t.Error("nonzero pricing should not be free")
	}
}

func TestFindByGroup(t *testing.T) {
	r := NewRegistry()
	r.Add(&Model{ID: "nvidia/deepseek-ai/deepseek-v3.2", Provider: "nvidia"})
	r.Add(&Model{ID: "openrouter/deepseek/deepseek-v3.2", Provider: "openrouter"})
	r.Add(&Model{ID: "groq/llama-3.3-70b-versatile", Provider: "groq"})

	group := FindByGroup(r, "deepseek-v3.2")
	if len(group) != 2 {
		t.Fatalf("expected 2 models in group, got %d", len(group))
	}
}

func TestSortModelsByAvg(t *testing.T) {
	m1 := &Model{ID: "a", AvgLatency: 500}
	m2 := &Model{ID: "b", AvgLatency: 100}
	m3 := &Model{ID: "c", AvgLatency: 300}
	list := []*Model{m1, m2, m3}

	SortModels(list, "avg", false)
	if list[0].ID != "b" || list[2].ID != "a" {
		t.Error("avg sort should order ascending")
	}

	SortModels(list, "avg", true)
	if list[0].ID != "a" || list[2].ID != "b" {
		t.Error("avg sort reverse should order descending")
	}
}

func TestFilterByTier(t *testing.T) {
	models := []*Model{
		{ID: "a", Tier: "S+"},
		{ID: "b", Tier: "A+"},
		{ID: "c", Tier: "S+"},
	}
	result := FilterByTier(models, "S+")
	if len(result) != 2 {
		t.Fatalf("expected 2 S+ models, got %d", len(result))
	}
}

func TestFilterBySearch(t *testing.T) {
	models := []*Model{
		{ID: "nvidia/deepseek-v3", Label: "DeepSeek V3"},
		{ID: "groq/llama", Label: "Llama 3"},
	}
	result := FilterBySearch(models, "deepseek")
	if len(result) != 1 {
		t.Fatalf("expected 1 match, got %d", len(result))
	}
}

func TestFindBestModel(t *testing.T) {
	models := []*Model{
		{ID: "a", Status: "up", QualityScore: 0.5, Uptime: 95, AvgLatency: 100},
		{ID: "b", Status: "up", QualityScore: 0.9, Uptime: 95, AvgLatency: 100},
		{ID: "c", Status: "down", QualityScore: 1.0, Uptime: 100, AvgLatency: 10},
	}
	best := FindBestModel(models)
	if best == nil {
		t.Fatal("should find best model")
	}
	if best.ID != "b" {
		t.Errorf("b should have best QoS, got %s", best.ID)
	}
}

func TestComputeQoS(t *testing.T) {
	// high uptime: multiplier 1.0
	m1 := &Model{QualityScore: 0.8, Uptime: 98, AvgLatency: 100}
	qos1 := ComputeQoS(m1)

	// low uptime: multiplier 0.2
	m2 := &Model{QualityScore: 0.8, Uptime: 50, AvgLatency: 100}
	qos2 := ComputeQoS(m2)

	if qos1 <= qos2 {
		t.Error("higher uptime should yield higher QoS")
	}
}
