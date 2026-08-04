package providers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverModels(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"model-1"},{"id":"model-2"}]}`))
	}))
	defer mock.Close()

	mgr := NewManager()
	mgr.providers["test-provider"] = &Provider{
		Key:          "test-provider",
		Name:         "Test Provider",
		BaseURL:      mock.URL,
		Discoverable: true,
	}

	models, err := mgr.DiscoverModels("test-provider")
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
	if models[0].ID != "test-provider/model-1" {
		t.Errorf("discovered model should be namespaced, got %s", models[0].ID)
	}
}

func TestDiscoverModelsNonDiscoverable(t *testing.T) {
	mgr := NewManager()
	mgr.providers["ollama"] = &Provider{
		Key:          "ollama",
		Name:         "Ollama",
		Discoverable: false,
	}

	models, err := mgr.DiscoverModels("ollama")
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if models != nil {
		t.Errorf("non-discoverable provider should return nil, got %v", models)
	}
}

func TestMergeDiscoveredModels(t *testing.T) {
	mgr := NewManager()
	mgr.providers["nvidia"] = &Provider{
		Key:   "nvidia",
		Name:  "NIM",
		URL:   "https://integrate.api.nvidia.com/v1/chat/completions",
		Models: []ModelEntry{
			{ID: "nvidia/static-model", Label: "Static"},
		},
	}

	mgr.MergeDiscoveredModels("nvidia", []ModelEntry{
		{ID: "nvidia/static-model", Label: "Duplicate"},
		{ID: "nvidia/new-model", Label: "New"},
	})

	p := mgr.GetProvider("nvidia")
	if len(p.Models) != 2 {
		t.Fatalf("expected 2 models after merge (static dedup + new), got %d", len(p.Models))
	}
}

func TestLoadSources(t *testing.T) {
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

	p := mgr.GetProvider("nvidia")
	if p == nil {
		t.Fatal("nvidia provider should exist")
	}
	if p.Name != "NIM" {
		t.Errorf("provider name should be NIM, got %s", p.Name)
	}
	if len(p.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(p.Models))
	}
	if p.BaseURL != "https://integrate.api.nvidia.com" {
		t.Errorf("baseURL should strip /v1 path, got %s", p.BaseURL)
	}
}

func TestExtractBaseURL(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"https://integrate.api.nvidia.com/v1/chat/completions", "https://integrate.api.nvidia.com"},
		{"https://generativelanguage.googleapis.com/v1beta/models", "https://generativelanguage.googleapis.com"},
		{"http://localhost:11434", "http://localhost:11434"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := extractBaseURL(tt.in); got != tt.want {
			t.Errorf("extractBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestEnvVarForProvider(t *testing.T) {
	t.Setenv("GROQ_API_KEY", "gsk_test")
	if got := EnvVarForProvider("groq"); got != "gsk_test" {
		t.Errorf("EnvVarForProvider(groq) = %q, want gsk_test", got)
	}

	t.Setenv("NVIDIA_API_KEY", "")
	if got := EnvVarForProvider("nvidia"); got != "" {
		t.Errorf("empty env should return empty, got %q", got)
	}
}
