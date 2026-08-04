package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/providers"
)

func newTestServer(t *testing.T, cfg interface{}) *Server {
	t.Helper()
	registry := models.NewRegistry()
	srv := NewServer(registry, cfg, 9999, "v0.0.0-test", false)
	return srv
}

func postJSON(t *testing.T, srv *Server, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)
	return rr
}

// TestAPIModelsPing: POST /api/models/ping runs a real single ping against
// the model endpoint and reports status/code/latency (T046).
func TestAPIModelsPing(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"ok"}`))
	}))
	defer upstream.Close()

	registry := models.NewRegistry()
	registry.Add(&models.Model{
		ID: "demo/ping-me", Provider: "demo", Status: "pending",
		Endpoint: upstream.URL, UpstreamModelID: "ping-me",
	})

	srv := newTestServer(t, nil)
	srv.registry = registry

	rr := postJSON(t, srv, "/api/models/ping", `{"modelId":"demo/ping-me"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Status   string `json:"status"`
		HTTPCode int    `json:"httpCode"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v", err)
	}
	if resp.Status != "up" {
		t.Errorf("status = %q, want up", resp.Status)
	}
	if resp.HTTPCode != http.StatusOK {
		t.Errorf("httpCode = %d, want 200", resp.HTTPCode)
	}
}

// TestAPIModelsPingDown: an unreachable endpoint reports down.
func TestAPIModelsPingDown(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := closed.URL
	closed.Close()

	registry := models.NewRegistry()
	registry.Add(&models.Model{
		ID: "demo/dead", Provider: "demo", Status: "pending",
		Endpoint: url, UpstreamModelID: "dead",
	})

	srv := newTestServer(t, nil)
	srv.registry = registry

	rr := postJSON(t, srv, "/api/models/ping", `{"modelId":"demo/dead"}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Status   string `json:"status"`
		HTTPCode int    `json:"httpCode"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Status != "down" {
		t.Errorf("status = %q, want down", resp.Status)
	}
}

// TestAPIModelsPingNotFound: unknown model id → 404.
func TestAPIModelsPingNotFound(t *testing.T) {
	srv := newTestServer(t, nil)
	rr := postJSON(t, srv, "/api/models/ping", `{"modelId":"nope/missing"}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

// TestAPIAccountStatus: accounts must reflect config providers + apiKeys.
// Provider names are chosen outside EnvOverrides so the test is
// environment-independent.
func TestAPIAccountStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Providers["acme"] = config.ProviderConfig{Enabled: true}
	cfg.Providers["beta"] = config.ProviderConfig{Enabled: false}
	cfg.APIKeys["acme"] = []interface{}{"key-1", "key-2"}
	cfg.APIKeys["beta"] = "key-3"

	srv := newTestServer(t, cfg)

	req := httptest.NewRequest(http.MethodGet, "/api/account-status", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Accounts []struct {
			Provider string `json:"provider"`
			KeyCount int    `json:"keyCount"`
			Enabled  bool   `json:"enabled"`
		} `json:"accounts"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v", err)
	}
	byName := map[string]struct {
		KeyCount int
		Enabled  bool
	}{}
	for _, a := range resp.Accounts {
		byName[a.Provider] = struct {
			KeyCount int
			Enabled  bool
		}{a.KeyCount, a.Enabled}
	}
	nvidia, ok := byName["acme"]
	if !ok || nvidia.KeyCount != 2 || !nvidia.Enabled {
		t.Errorf("acme account = %+v, want keyCount 2 enabled true", byName["acme"])
	}
	groq, ok := byName["beta"]
	if !ok || groq.KeyCount != 1 || groq.Enabled {
		t.Errorf("beta account = %+v, want keyCount 1 enabled false", byName["beta"])
	}
}

// TestAPIProvidersDiscovery: POST /api/providers/<key> discovers models and
// merges them into the registry (T046).
func TestAPIProvidersDiscovery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/models") {
			w.Write([]byte(`{"data":[{"id":"model-a"},{"id":"model-b"}]}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	tmp := t.TempDir()
	sources := filepath.Join(tmp, "sources.json")
	os.WriteFile(sources, []byte(`{"demo":{"name":"Demo","url":"`+upstream.URL+`/v1/","discoverable":true,"models":[]}}`), 0600)

	mgr := providers.NewManager()
	if err := mgr.LoadSources(sources); err != nil {
		t.Fatalf("LoadSources: %v", err)
	}

	srv := newTestServer(t, nil)
	srv.SetProviders(mgr)

	rr := postJSON(t, srv, "/api/providers/demo", `{}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Added int `json:"added"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v", err)
	}
	if resp.Added != 2 {
		t.Errorf("added = %d, want 2", resp.Added)
	}

	m := srv.registry.Get("demo/model-a")
	if m == nil {
		t.Fatal("demo/model-a not merged into registry")
	}
	if m.UpstreamModelID != "model-a" {
		t.Errorf("upstreamModelId = %q, want model-a", m.UpstreamModelID)
	}
	if m.Endpoint == "" || m.ProviderHost == "" {
		t.Errorf("endpoint/host not applied: %+v", m)
	}

	// Second discovery must not duplicate.
	rr = postJSON(t, srv, "/api/providers/demo", `{}`)
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.Added != 0 {
		t.Errorf("second discovery added = %d, want 0", resp.Added)
	}
}

// TestAPIProvidersUnknownProvider: unknown provider key → 404.
func TestAPIProvidersUnknownProvider(t *testing.T) {
	srv := newTestServer(t, nil)
	srv.SetProviders(providers.NewManager())
	rr := postJSON(t, srv, "/api/providers/nope", `{}`)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}
