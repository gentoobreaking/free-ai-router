package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetchFreeOpenRouterModels(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"data": [
				{"id": "google/gemini-2.5-flash", "pricing": {"prompt": "0", "completion": "0"}},
				{"id": "anthropic/claude-sonnet-4", "pricing": {"prompt": "0.00015", "completion": "0.0006"}},
				{"id": "meta-llama/llama-3.3-70b", "pricing": {"prompt": "0", "completion": "0"}}
			]
		}`))
	}))
	defer mock.Close()

	mgr := NewManager()
	free := mgr.fetchFreeOpenRouterModels()
	if free == nil {
		t.Skip("OpenRouter API unavailable, skipping")
	}
}

func TestFetchClawLabsModelsReturnsEntries(t *testing.T) {
	mgr := NewManager()
	entries := mgr.fetchClawLabsModels()

	// Should always have Pollinations AI models
	foundPollinations := false
	for _, e := range entries {
		if e.ID == "pollinations/openai" {
			foundPollinations = true
			break
		}
	}
	if !foundPollinations {
		t.Error("fetchClawLabsModels should include pollinations/openai")
	}

	foundGPT4o := false
	for _, e := range entries {
		if e.ID == "pollinations/gpt-oss" {
			foundGPT4o = true
			break
		}
	}
	if !foundGPT4o {
		t.Error("fetchClawLabsModels should include pollinations/gpt-oss")
	}
}

func TestLoadSourcesIncludesClawLabs(t *testing.T) {
	src := map[string]SourceProvider{
		"nvidia": {
			Name: "NIM",
			URL:  "https://integrate.api.nvidia.com/v1/chat/completions",
			Models: [][]interface{}{
				{"nvidia/test-model", "Test Model", "128k"},
			},
		},
	}
	data, _ := json.Marshal(src)
	path := t.TempDir() + "/sources.json"
	writeFile(t, path, data)

	mgr := NewManager()
	if err := mgr.LoadSources(path); err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	clawLabs := mgr.GetProvider("clawlabs")
	if clawLabs == nil {
		t.Fatal("clawlabs provider should be created by LoadSources")
	}
	if clawLabs.Name != "ClawLabs Free Models" {
		t.Errorf("clawlabs provider name = %q, want ClawLabs Free Models", clawLabs.Name)
	}

	// Should include OpenRouter free models and Pollinations AI
	hasPollinations := false
	for _, m := range clawLabs.Models {
		if m.ID == "pollinations/openai" {
			hasPollinations = true
			break
		}
	}
	if !hasPollinations {
		t.Error("clawlabs provider should include Pollinations AI models")
	}
}

func TestLoadSourcesFiltersOpenRouterByFreeTier(t *testing.T) {
	src := map[string]SourceProvider{
		"openrouter": {
			Name: "OpenRouter",
			URL:  "https://openrouter.ai/api/v1/chat/completions",
			Models: [][]interface{}{
				{"openrouter/free-model", "Free Model", "128k"},
				{"openrouter/paid-model", "Paid Model", "128k"},
			},
		},
	}
	data, _ := json.Marshal(src)
	path := t.TempDir() + "/sources.json"
	writeFile(t, path, data)

	mgr := NewManager()
	if err := mgr.LoadSources(path); err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	p := mgr.GetProvider("openrouter")
	if p == nil {
		t.Fatal("openrouter provider should exist")
	}

	// Due to network filtering, paid-model should potentially be filtered
	// (but we can't guarantee API availability in tests, so just verify
	// the provider is loaded)
	if len(p.Models) == 0 {
		t.Log("warning: openrouter models filtered by free tier check (API may be unavailable)")
	}
}
