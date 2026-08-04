package router

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

// TestAutoPingToggleStartsStopsEngine: POST /api/auto-ping must actually
// start/stop the ping engine, and a disabled config must not run it (T050).
func TestAutoPingToggleStartsStopsEngine(t *testing.T) {
	t.Setenv("FREMODEL_CONFIG_PATH", filepath.Join(t.TempDir(), "cfg.json"))

	cfg := config.DefaultConfig()
	cfg.AutoPingEnabled = false
	srv := newTestServer(t, cfg)

	engine := ping.NewEngine(nil)
	srv.SetEngine(engine)
	if engine.Running() {
		t.Fatal("engine must not run when auto-ping is disabled")
	}

	rr := postJSON(t, srv, "/api/auto-ping", `{"autoPing":true}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("enable: expected 200, got %d", rr.Code)
	}
	if !engine.Running() {
		t.Error("engine should start after enable")
	}

	rr = postJSON(t, srv, "/api/auto-ping", `{"autoPing":false}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("disable: expected 200, got %d", rr.Code)
	}
	if engine.Running() {
		t.Error("engine should stop after disable")
	}
}

// TestAPIMetaUpdateChecker: /api/meta reports a real update check result and
// caches it (T052).
func TestAPIMetaUpdateChecker(t *testing.T) {
	registry := models.NewRegistry()
	srv := NewServer(registry, nil, 9999, "v1.0.0", false)

	calls := 0
	srv.SetUpdateChecker(func() (string, error) {
		calls++
		return "https://example.com/releases/v1.1.0", nil
	})

	getMeta := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
		rr := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rr, req)
		return rr
	}

	rr := getMeta()
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp struct {
		Version         string `json:"version"`
		UpdateAvailable bool   `json:"updateAvailable"`
		UpdateURL       string `json:"updateUrl"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad response: %v", err)
	}
	if resp.Version != "v1.0.0" {
		t.Errorf("version = %q", resp.Version)
	}
	if !resp.UpdateAvailable || resp.UpdateURL == "" {
		t.Errorf("updateAvailable = %v, updateUrl = %q; want true with url", resp.UpdateAvailable, resp.UpdateURL)
	}

	_ = getMeta()
	if calls != 1 {
		t.Errorf("update checker invoked %d times, want 1 (cached)", calls)
	}
}

// TestAPIMetaUpdateCheckerError: checker failure degrades to no-update.
func TestAPIMetaUpdateCheckerError(t *testing.T) {
	registry := models.NewRegistry()
	srv := NewServer(registry, nil, 9999, "v1.0.0", false)
	srv.SetUpdateChecker(func() (string, error) { return "", errors.New("network down") })

	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	var resp struct {
		UpdateAvailable bool `json:"updateAvailable"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.UpdateAvailable {
		t.Error("updateAvailable should be false when the check fails")
	}
}

// TestAPIMetaNoChecker: without a checker, updateAvailable is false.
func TestAPIMetaNoChecker(t *testing.T) {
	srv := newTestServer(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/meta", nil)
	rr := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rr, req)

	var resp struct {
		UpdateAvailable bool `json:"updateAvailable"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp.UpdateAvailable {
		t.Error("updateAvailable should be false without a checker")
	}
}

// TestAPIConfigConcurrentGetPost: concurrent GET/POST must not race under
// -race (T053).
func TestAPIConfigConcurrentGetPost(t *testing.T) {
	t.Setenv("FREMODEL_CONFIG_PATH", filepath.Join(t.TempDir(), "cfg.json"))

	cfg := config.DefaultConfig()
	srv := newTestServer(t, cfg)
	body := `{"codingOnly":true}`

	var wg sync.WaitGroup
	for i := 0; i < 40; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
		}()
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(body))
			rr := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rr, req)
		}()
	}
	wg.Wait()
}
