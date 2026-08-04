package providers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDiscoverNewApiModels(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("expected /v1/models, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "deepseek-v3", "owned_by": "siliconflow", "context_length": 131072},
				{"id": "qwen2.5-72b", "owned_by": "siliconflow", "context_length": 131072},
				{"id": "glm-4-9b", "owned_by": "baidu", "context_length": 128000}
			]
		}`))
	}))
	defer mock.Close()

	models, err := DiscoverNewApiModels(mock.URL)
	if err != nil {
		t.Fatalf("DiscoverNewApiModels: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
	if models[0].ID != "new-api/deepseek-v3" {
		t.Errorf("expected first model to be new-api/deepseek-v3, got %s", models[0].ID)
	}
	if models[0].Context != "" {
		t.Errorf("expected empty context, got %s", models[0].Context)
	}
}

func TestDiscoverNewApiModelsError(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer mock.Close()

	_, err := DiscoverNewApiModels(mock.URL)
	if err == nil {
		t.Error("expected error for 503 response")
	}
}


func TestEnvVarForProviderSiliconFlow(t *testing.T) {
	if got := EnvVarForProvider("siliconflow"); got != "" {
		t.Errorf("EnvVarForProvider(siliconflow) = %q, want empty (no env set)", got)
	}

	t.Setenv("SILICONFLOW_API_KEY", "sk-test")
	if got := EnvVarForProvider("siliconflow"); got != "sk-test" {
		t.Errorf("EnvVarForProvider(siliconflow) = %q, want sk-test", got)
	}
}

func TestEnvVarForProviderNewAPI(t *testing.T) {
	if got := EnvVarForProvider("new-api"); got != "" {
		t.Errorf("EnvVarForProvider(new-api) = %q, want empty (no env set)", got)
	}

	t.Setenv("NEW_API_API_KEY", "sk-test")
	if got := EnvVarForProvider("new-api"); got != "sk-test" {
		t.Errorf("EnvVarForProvider(new-api) = %q, want sk-test", got)
	}
}

func TestDiscoverModelsForNewApiProvider(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "deepseek-v3", "owned_by": "siliconflow", "context_length": 131072}
			]
		}`))
	}))
	defer mock.Close()

	mgr := NewManager()
	mgr.providers["new-api"] = &Provider{
		Key:          "new-api",
		Name:         "New-API Gateway",
		BaseURL:      mock.URL,
		Discoverable: true,
	}

	models, err := mgr.DiscoverModels("new-api")
	if err != nil {
		t.Fatalf("DiscoverModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].ID != "new-api/deepseek-v3" {
		t.Errorf("expected model ID with provider prefix, got %s", models[0].ID)
	}
}
