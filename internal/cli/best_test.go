package cli

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/freemodel/router/internal/models"
)

// TestRunBestSendsAPIKey verifies --best resolves keys before pinging, so
// authenticated upstreams respond 200 instead of 401 (spec §10.3).
func TestRunBestSendsAPIKey(t *testing.T) {
	var mu sync.Mutex
	var authed bool

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authed = r.Header.Get("Authorization") == "Bearer test-key"
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer srv.Close()

	registry := models.NewRegistry()
	registry.Add(&models.Model{
		ID:              "nvidia/test-model",
		Provider:        "nvidia",
		Status:          "pending",
		Endpoint:        srv.URL,
		UpstreamModelID: "test-model",
		QualityScore:    0.8,
		Uptime:          100,
	})

	resolveKey := func(provider string) string {
		if provider == "nvidia" {
			return "test-key"
		}
		return ""
	}

	id, err := RunBest(registry, resolveKey)
	if err != nil {
		t.Fatalf("RunBest: %v", err)
	}
	if id != "nvidia/test-model" {
		t.Errorf("RunBest returned %q, want nvidia/test-model", id)
	}

	mu.Lock()
	defer mu.Unlock()
	if !authed {
		t.Error("upstream should have received the Bearer key from resolveKey")
	}
}

// TestRunBestNoKeysStillCompletes: models without keys must not crash --best.
func TestRunBestNoKeysStillCompletes(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := closed.URL
	closed.Close()

	registry := models.NewRegistry()
	registry.Add(&models.Model{
		ID: "provider/model", Provider: "provider",
		Status: "pending", Endpoint: deadURL, UpstreamModelID: "model",
	})

	_, err := RunBest(registry, nil)
	if err == nil {
		t.Fatal("expected an error when no model is reachable")
	}
}
