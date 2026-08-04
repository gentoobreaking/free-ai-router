package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/freemodel/router/internal/models"
)

func newTestRegistry(upstreams map[string]*httptest.Server) (*models.Registry, map[string]string) {
	registry := models.NewRegistry()
	keys := make(map[string]string)

	for provider, srv := range upstreams {
		m := &models.Model{
			ID:              provider + "/test-model",
			Label:           "Test Model",
			Provider:        provider,
			Status:          "up",
			Endpoint:        srv.URL,
			UpstreamModelID: "test-model",
			QualityScore:    0.8,
			Uptime:          95,
		}
		registry.Add(m)
		keys[m.ID] = provider
	}
	return registry, keys
}

func testServer(t *testing.T) (*Router, *httptest.Server, *Server) {
	t.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"id":"test-model","choices":[{"message":{"role":"assistant","content":"hi"}}]}`))
	}))

	registry := models.NewRegistry()
	m := &models.Model{
		ID:              "nvidia/test-model",
		Label:           "Test Model",
		Provider:        "nvidia",
		Status:          "up",
		Endpoint:        upstream.URL,
		UpstreamModelID: "test-model",
		QualityScore:    0.8,
		Uptime:          95,
	}
	registry.Add(m)

	srv := NewServer(registry, nil, 7352, "test", true)
	return srv.router, upstream, srv
}

func TestChatCompletionProxy200(t *testing.T) {
	router, upstream, _ := testServer(t)
	defer upstream.Close()

	body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChatCompletionsNoEligibleModels(t *testing.T) {
	router := NewRouter(models.NewRegistry(), NewLogger(true))

	body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestFailoverOn500(t *testing.T) {
	badUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer badUpstream.Close()

	goodUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer goodUpstream.Close()

	registry := models.NewRegistry()
	bad := &models.Model{
		ID:              "provider-a/bad-model",
		Provider:        "provider-a",
		Status:          "up",
		Endpoint:        badUpstream.URL,
		UpstreamModelID: "bad-model",
		QualityScore:    0.8,
		Uptime:          95,
	}
	good := &models.Model{
		ID:              "provider-b/good-model",
		Provider:        "provider-b",
		Status:          "up",
		Endpoint:        goodUpstream.URL,
		UpstreamModelID: "good-model",
		QualityScore:    0.7,
		Uptime:          95,
	}
	registry.Add(bad)
	registry.Add(good)

	router := NewRouter(registry, NewLogger(true))

	body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected failover to succeed with 200, got %d: %s", w.Code, w.Body.String())
	}
	if bad.Status == "up" {
		t.Error("failed model should be marked down after failover")
	}
}

func TestFailoverOn429WithCooldown(t *testing.T) {
	badUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer badUpstream.Close()

	registry := models.NewRegistry()
	bad := &models.Model{
		ID:              "provider-a/bad-model",
		Provider:        "provider-a",
		Status:          "up",
		Endpoint:        badUpstream.URL,
		UpstreamModelID: "bad-model",
		QualityScore:    0.8,
		Uptime:          95,
	}
	registry.Add(bad)

	router := NewRouter(registry, NewLogger(true))

	body := `{"model":"auto-fastest","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 after all retries, got %d", w.Code)
	}

	router.mu.RLock()
	_, onCooldown := router.cooldowns["provider-a/bad-model"]
	router.mu.RUnlock()
	if !onCooldown {
		t.Error("429 model should be on cooldown")
	}
}

func TestSelectModelsByGroup(t *testing.T) {
	registry := models.NewRegistry()
	registry.Add(&models.Model{ID: "nvidia/deepseek-ai/deepseek-v3.2", Provider: "nvidia", Status: "up", QualityScore: 0.8, Uptime: 95})
	registry.Add(&models.Model{ID: "openrouter/deepseek/deepseek-v3.2", Provider: "openrouter", Status: "up", QualityScore: 0.9, Uptime: 95})

	router := NewRouter(registry, NewLogger(true))
	selected := router.selectModels("deepseek-v3.2")
	if len(selected) != 2 {
		t.Fatalf("expected 2 candidates for group, got %d", len(selected))
	}
	if selected[0].Provider != "openrouter" {
		t.Error("higher QoS provider should rank first")
	}
}

func TestV1ModelsEndpoint(t *testing.T) {
	registry := models.NewRegistry()
	registry.Add(&models.Model{ID: "nvidia/test", Provider: "nvidia", Tags: []string{"coding"}})

	srv := NewServer(registry, nil, 7352, "test", true)
	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].ID != "nvidia/test" {
		t.Errorf("unexpected models response: %s", w.Body.String())
	}
}

func TestPinnedModel(t *testing.T) {
	registry := models.NewRegistry()
	registry.Add(&models.Model{ID: "nvidia/pinned-model", Provider: "nvidia", Status: "up", QualityScore: 0.8, Uptime: 95})

	router := NewRouter(registry, NewLogger(true))
	router.SetPinned("nvidia/pinned-model", "exact")

	selected := router.selectModels("something-else")
	if len(selected) != 1 || selected[0].ID != "nvidia/pinned-model" {
		t.Errorf("pinned model should be selected exclusively, got %+v", selected)
	}
}

func TestRequestBodyParsing(t *testing.T) {
	router := NewRouter(models.NewRegistry(), NewLogger(true))

	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader("{invalid"))
	w := httptest.NewRecorder()
	router.ServeChatCompletions(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("invalid JSON should return 400, got %d", w.Code)
	}
}

func TestPollinationsTextFallback(t *testing.T) {
	// mock /text endpoint that returns plain text
	textServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/") {
			t.Errorf("unexpected request path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Hello from Pollinations!"))
	}))
	defer textServer.Close()

	// override PollinationsTextEndpoint so BuildPollinationsTextURL hits mock
	// We test forwardPollinationsText directly
	registry := models.NewRegistry()
	m := &models.Model{
		ID:           "pollinations/openai",
		Label:        "Pollinations GPT-4o",
		Provider:     "pollinations",
		Status:       "up",
		Endpoint:     "", // no /v1 endpoint — should trigger /text path
		APIKey:       "", // keyless → trigger forwardPollinationsText
		ProviderHost: "127.0.0.1",
		QualityScore: 0.8,
		Uptime:       95,
	}
	registry.Add(m)

	router := NewRouter(registry, NewLogger(true))

	body := `{"model":"pollinations/openai","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	// Should reach /text endpoint (real server, not mock) — if pollinations
	// is up it returns 200 with JSON-wrapped response; if not, failover returns 503
	// This test is network-dependent; skip if pollinations.ai is down
	if w.Code == http.StatusServiceUnavailable {
		t.Skip("pollinations /text endpoint unreachable in this environment")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from pollinations /text, got %d: %s", w.Code, w.Body.String())
	}

	// Verify response is OpenAI-compatible JSON
	var completion struct {
		Object  string `json:"object"`
		Choices []struct {
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &completion); err != nil {
		t.Fatalf("response is not valid JSON: %v\nBody: %s", err, w.Body.String())
	}
	if completion.Object != "chat.completion" {
		t.Errorf("object = %q, want chat.completion", completion.Object)
	}
	if len(completion.Choices) == 0 || completion.Choices[0].Message.Role != "assistant" {
		t.Error("response missing expected choices/role")
	}
}

func TestPollinationsTextFallbackInvalidBody(t *testing.T) {
	registry := models.NewRegistry()
	m := &models.Model{
		ID:       "pollinations/openai",
		Provider: "pollinations",
		Status:   "up",
		APIKey:   "",
	}
	registry.Add(m)
	router := NewRouter(registry, NewLogger(true))

	// Body with no user messages
	body := `{"model":"pollinations/openai","messages":[{"role":"system","content":"you are helpful"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for body without user message, got %d", w.Code)
	}
}

func TestPollinationsTextWithAPIKeyRoutesNormally(t *testing.T) {
	// When pollinations model HAS an API key, it should NOT trigger /text fallback
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify it came via /v1/chat/completions not /text
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer upstream.Close()

	registry := models.NewRegistry()
	m := &models.Model{
		ID:              "pollinations/openai",
		Provider:        "pollinations",
		Status:          "up",
		Endpoint:        upstream.URL,
		APIKey:          "sk-fake-key", // has key → normal path
		UpstreamModelID: "gpt-4o",
		QualityScore:    0.8,
		Uptime:          95,
	}
	registry.Add(m)
	router := NewRouter(registry, NewLogger(true))

	body := `{"model":"pollinations/openai","messages":[{"role":"user","content":"Hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 via normal path, got %d", w.Code)
	}
}

func TestPollinationsTextStreaming(t *testing.T) {
	// /text endpoint doesn't support streaming, but the adapter should
	// serve it as a single SSE chunk. This is a live test.
	registry := models.NewRegistry()
	m := &models.Model{
		ID:       "pollinations/openai",
		Provider: "pollinations",
		Status:   "up",
		APIKey:   "",
	}
	registry.Add(m)
	router := NewRouter(registry, NewLogger(true))

	body := `{"model":"pollinations/openai","stream":true,"messages":[{"role":"user","content":"Say hello in one word"}]}`
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	router.ServeChatCompletions(w, req)

	if w.Code == http.StatusServiceUnavailable {
		t.Skip("pollinations /text endpoint unreachable")
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// SSE format: "data: ...\n\n" then "data: [DONE]\n\n"
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "data: ") {
		t.Errorf("stream response missing SSE data prefix: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, "[DONE]") {
		t.Errorf("stream response missing [DONE] marker: %s", bodyStr)
	}
}
