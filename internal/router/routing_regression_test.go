package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/freemodel/router/internal/models"
)

// recordingUpstream records every request body and connection peer it sees.
type recordingUpstream struct {
	mu       sync.Mutex
	bodies   []string
	peers    map[string]int
	requests int
	auths    []string
}

func newRecordingUpstream(t *testing.T, code int, body string) (*recordingUpstream, *httptest.Server) {
	t.Helper()
	rec := &recordingUpstream{peers: make(map[string]int)}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec.mu.Lock()
		rec.requests++
		rec.peers[r.RemoteAddr]++
		rec.auths = append(rec.auths, r.Header.Get("Authorization"))
		var buf bytes.Buffer
		buf.ReadFrom(r.Body)
		rec.bodies = append(rec.bodies, buf.String())
		rec.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return rec, srv
}

func (u *recordingUpstream) lastBody() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.bodies) == 0 {
		return ""
	}
	return u.bodies[len(u.bodies)-1]
}

func (u *recordingUpstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.requests
}

func (u *recordingUpstream) distinctPeers() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.peers)
}

func (u *recordingUpstream) lastAuth() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.auths) == 0 {
		return ""
	}
	return u.auths[len(u.auths)-1]
}

func newUpModel(id string, upstreamID string, srv *httptest.Server, quality float64, tags []string) *models.Model {
	return &models.Model{
		ID:              id,
		Provider:        strings.SplitN(id, "/", 2)[0],
		Status:          "up",
		Endpoint:        srv.URL,
		UpstreamModelID: upstreamID,
		QualityScore:    quality,
		Uptime:          95,
		Tags:            tags,
		APIKey:          "test-key",
	}
}

// TestProxyRewritesModelID covers spec §7.3 step 6: the proxy must forward the
// resolved upstream model ID, not the client's alias/group/tag string.
func TestProxyRewritesModelID(t *testing.T) {
	rec, srv := newRecordingUpstream(t, 200, `{"id":"upstream-model","choices":[{"message":{"role":"assistant","content":"hi"}}]}`)

	registry := models.NewRegistry()
	registry.Add(newUpModel("nvidia/deepseek-ai/deepseek-v3.2", "deepseek-ai/deepseek-v3.2", srv, 0.8, []string{"coding"}))

	router := NewRouter(registry, NewLogger(true))

	cases := []struct {
		name     string
		request  string
		expected string
	}{
		{"auto-fastest", "auto-fastest", "deepseek-ai/deepseek-v3.2"},
		{"group alias", "deepseek-v3.2", "deepseek-ai/deepseek-v3.2"},
		{"tag query", "tag:coding", "deepseek-ai/deepseek-v3.2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := `{"model":"` + tc.request + `","messages":[{"role":"user","content":"hi"}]}`
			req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
			w := httptest.NewRecorder()
			router.ServeChatCompletions(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
			}

			var sent map[string]interface{}
			if err := json.Unmarshal([]byte(rec.lastBody()), &sent); err != nil {
				t.Fatalf("upstream body not JSON: %v", err)
			}
			if got, _ := sent["model"].(string); got != tc.expected {
				t.Errorf("upstream model field = %q, want %q (full body: %s)", got, tc.expected, rec.lastBody())
			}
			if _, ok := sent["messages"]; !ok {
				t.Error("messages field must be preserved in rewritten body")
			}
		})
	}
}

// TestNonRetryable401ProxiedWithoutFailover: per spec §7.4 only 429/5xx/conn
// errors trigger failover; 401 must be returned to the client verbatim.
func TestNonRetryable401ProxiedWithoutFailover(t *testing.T) {
	rec, srv := newRecordingUpstream(t, 401, `{"error":"invalid_api_key"}`)

	registry := models.NewRegistry()
	registry.Add(newUpModel("provider-a/first", "first", srv, 0.8, nil))

	router := NewRouter(registry, NewLogger(true))

	body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 proxied to client, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid_api_key") {
		t.Errorf("upstream error body should be proxied, got %s", w.Body.String())
	}
	if rec.count() != 1 {
		t.Errorf("401 must not trigger failover retries, upstream hit %d times", rec.count())
	}

	// Non-retryable errors must not mark the model down.
	m := registry.Get("provider-a/first")
	if m == nil || m.Status != "up" {
		t.Error("model should remain up after non-retryable 401")
	}
}

// TestFailoverMatrix: 429 → failover + 60s cooldown; 500 → failover + down.
func TestFailoverMatrix(t *testing.T) {
	t.Run("429 fails over and applies cooldown", func(t *testing.T) {
		badRec, badSrv := newRecordingUpstream(t, 429, `{"error":"rate_limited"}`)
		goodRec, goodSrv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

		registry := models.NewRegistry()
		registry.Add(newUpModel("provider-a/bad", "bad", badSrv, 0.8, nil))
		registry.Add(newUpModel("provider-b/good", "good", goodSrv, 0.7, nil))

		router := NewRouter(registry, NewLogger(true))
		body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
		w := httptest.NewRecorder()
		router.ServeChatCompletions(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body)))

		if w.Code != http.StatusOK {
			t.Fatalf("failover should succeed, got %d", w.Code)
		}
		if badRec.count() != 1 || goodRec.count() != 1 {
			t.Errorf("expected 1 attempt each, got bad=%d good=%d", badRec.count(), goodRec.count())
		}
		if !router.isCooldown("provider-a/bad") {
			t.Error("429 model should be on cooldown")
		}
	})

	t.Run("500 fails over and marks down", func(t *testing.T) {
		badRec, badSrv := newRecordingUpstream(t, 500, `{"error":"boom"}`)
		goodRec, goodSrv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

		registry := models.NewRegistry()
		registry.Add(newUpModel("provider-a/bad", "bad", badSrv, 0.8, nil))
		registry.Add(newUpModel("provider-b/good", "good", goodSrv, 0.7, nil))

		router := NewRouter(registry, NewLogger(true))
		body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
		w := httptest.NewRecorder()
		router.ServeChatCompletions(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body)))

		if w.Code != http.StatusOK {
			t.Fatalf("failover should succeed, got %d", w.Code)
		}
		if badRec.count() != 1 || goodRec.count() != 1 {
			t.Errorf("expected 1 attempt each, got bad=%d good=%d", badRec.count(), goodRec.count())
		}
		m := registry.Get("provider-a/bad")
		if m == nil || m.Status == "up" {
			t.Error("500 model should be marked down")
		}
	})

	t.Run("network error fails over", func(t *testing.T) {
		// A closed server produces a connection error.
		closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		closedURL := closed.URL
		closed.Close()

		rec, goodSrv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

		registry := models.NewRegistry()
		registry.Add(&models.Model{
			ID: "provider-a/dead", Provider: "provider-a", Status: "up",
			Endpoint: closedURL, UpstreamModelID: "dead", QualityScore: 0.8, Uptime: 95,
		})
		registry.Add(newUpModel("provider-b/alive", "alive", goodSrv, 0.7, nil))

		router := NewRouter(registry, NewLogger(true))
		body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
		w := httptest.NewRecorder()
		router.ServeChatCompletions(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body)))

		if w.Code != http.StatusOK {
			t.Fatalf("failover after connection error should succeed, got %d", w.Code)
		}
		if rec.count() != 1 {
			t.Errorf("good upstream should be hit once, got %d", rec.count())
		}
	})
}

// TestProxyConnectionReuse: consecutive proxy requests to the same host must
// reuse the pooled keep-alive connection (Requirement #6 / §7.4).
func TestProxyConnectionReuse(t *testing.T) {
	rec, srv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

	registry := models.NewRegistry()
	registry.Add(newUpModel("provider-a/model", "model", srv, 0.8, nil))

	router := NewRouter(registry, NewLogger(true))

	for i := 0; i < 3; i++ {
		body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
		w := httptest.NewRecorder()
		router.ServeChatCompletions(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body)))
		if w.Code != http.StatusOK {
			t.Fatalf("request %d failed: %d", i, w.Code)
		}
	}

	if rec.distinctPeers() != 1 {
		t.Errorf("expected a single reused TCP connection, saw %d distinct peers", rec.distinctPeers())
	}
}

// TestProxySendsAPIKey: proxied requests must carry the model's Bearer token.
func TestProxySendsAPIKey(t *testing.T) {
	rec, srv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

	registry := models.NewRegistry()
	registry.Add(newUpModel("provider-a/model", "model", srv, 0.8, nil))

	router := NewRouter(registry, NewLogger(true))
	body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
	w := httptest.NewRecorder()
	router.ServeChatCompletions(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body)))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if auth := rec.lastAuth(); auth != "Bearer test-key" {
		t.Errorf("Authorization header = %q, want %q", auth, "Bearer test-key")
	}
}

// TestCodingOnlyEligibility: with codingOnly enabled, non-coding models must
// be excluded from selection (spec §3.2).
func TestCodingOnlyEligibility(t *testing.T) {
	_, codingSrv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	_, generalSrv := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

	registry := models.NewRegistry()
	registry.Add(newUpModel("provider-a/coding-model", "coding-model", codingSrv, 0.8, []string{"coding"}))
	registry.Add(newUpModel("provider-b/general-model", "general-model", generalSrv, 0.9, []string{"general"}))
	registry.FlagCodingOnly(true)

	router := NewRouter(registry, NewLogger(true))
	selected := router.selectModels("auto-fastest")
	if len(selected) != 1 || selected[0].ID != "provider-a/coding-model" {
		t.Errorf("codingOnly should exclude general model, got %+v", selected)
	}
}

// TestBannedModelEligibility: banned models must be excluded (spec §3.3).
func TestBannedModelEligibility(t *testing.T) {
	_, srvA := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)
	_, srvB := newRecordingUpstream(t, 200, `{"choices":[{"message":{"content":"ok"}}]}`)

	registry := models.NewRegistry()
	registry.Add(newUpModel("provider-a/ok-model", "ok-model", srvA, 0.7, nil))
	registry.Add(newUpModel("provider-b/bad-model", "bad-model", srvB, 0.9, nil))
	registry.BanModel("provider-b/bad-model")

	router := NewRouter(registry, NewLogger(true))
	selected := router.selectModels("auto-fastest")
	if len(selected) != 1 || selected[0].ID != "provider-a/ok-model" {
		t.Errorf("banned model should be excluded, got %+v", selected)
	}
}
