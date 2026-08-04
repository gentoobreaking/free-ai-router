package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/providers"
)

const MAX_PROACTIVE_RETRIES = 5
const RateLimitCooldown = 60 * time.Second
const retryBackoff = 50 * time.Millisecond

type Router struct {
	mu          sync.RWMutex
	registry    *models.Registry
	logger      *Logger
	pool        *ping.TransportPool
	pinned      string
	pinningMode string
	cooldowns   map[string]time.Time
}

type modelRequest struct {
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

type chatMessage struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolCalls []struct {
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
	} `json:"tool_calls"`
}

func NewRouter(registry *models.Registry, logger *Logger) *Router {
	return &Router{
		registry:    registry,
		logger:      logger,
		pool:        ping.NewTransportPool(),
		pinningMode: "canonical",
		cooldowns:   make(map[string]time.Time),
	}
}

// SetPool shares the keep-alive transport pool across the ping engine and the
// proxy so both reuse per-host connections (spec §7.4).
func (r *Router) SetPool(pool *ping.TransportPool) {
	if pool != nil {
		r.pool = pool
	}
}

func (r *Router) SetPinned(modelID, mode string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pinned = modelID
	if mode != "" {
		r.pinningMode = mode
	}
}

func (r *Router) Pinned() (string, string) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pinned, r.pinningMode
}

func (r *Router) ServeChatCompletions(w http.ResponseWriter, req *http.Request) {
	start := time.Now()

	body, err := io.ReadAll(req.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read body"})
		return
	}

	var mReq modelRequest
	if err := json.Unmarshal(body, &mReq); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	candidates := r.selectModels(mReq.Model)
	if len(candidates) == 0 {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no eligible models available"})
		return
	}

	attempts := 0
	var lastErr error

	for _, m := range candidates {
		if attempts >= MAX_PROACTIVE_RETRIES {
			break
		}
		attempts++

		if r.isCooldown(m.ID) {
			continue
		}

		written, status, respBody, ttfb, fwdErr := r.forward(w, req, m, body, start)
		if fwdErr != nil {
			// Connection-level failure: retryable (§7.4)
			lastErr = fwdErr
			r.markFailure(m)
			time.Sleep(retryBackoff)
			continue
		}

		if written {
			// Either 2xx (response streamed/copied) or a non-retryable
			// upstream error proxied verbatim (401/403/404/400/422...).
			entry := &LogEntry{
				Timestamp: time.Now(),
				Model:     m.ID,
				Provider:  m.Provider,
				Status:    status,
				TTFB:      ttfb,
				Duration:  time.Since(start),
			}
			if content, toolCalls := extractRequestInfo(body); content != "" || len(toolCalls) > 0 {
				entry.Content = content
				entry.ToolCalls = toolCalls
			}
			if usage := extractUsage(respBody); usage != nil {
				entry.Usage = usage
			}
			r.logger.Log(entry)
			return
		}

		// Retryable upstream status (429 / 5xx) — spec §7.4
		r.markFailure(m)
		if status == http.StatusTooManyRequests {
			r.cooldowns[m.ID] = time.Now().Add(RateLimitCooldown)
		}
		lastErr = fmt.Errorf("upstream %s returned %d", m.ID, status)
		time.Sleep(retryBackoff)
	}

	if lastErr != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": fmt.Sprintf("all upstream attempts failed: %v", lastErr),
		})
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "all models are on cooldown"})
}

func (r *Router) selectModels(requested string) []*models.Model {
	r.mu.RLock()
	pinned := r.pinned
	pinningMode := r.pinningMode
	r.mu.RUnlock()

	all := r.registry.Snapshot()

	var pool []*models.Model
	if pinned != "" {
		if pinningMode == "exact" {
			if m := snapshotByID(all, pinned); m != nil && r.eligible(m) {
				pool = []*models.Model{m}
			}
		} else {
			for _, m := range all {
				if models.ResolveGroup(m.ID) == models.ResolveGroup(pinned) && r.eligible(m) {
					pool = append(pool, m)
				}
			}
		}
	} else {
		requested = strings.TrimPrefix(requested, "openrouter/")
		if requested == "auto-fastest" || requested == "" {
			for _, m := range all {
				if r.eligible(m) {
					pool = append(pool, m)
				}
			}
		} else if strings.HasPrefix(requested, "tag:") {
			tag := strings.TrimPrefix(requested, "tag:")
			for _, m := range all {
				if r.eligible(m) && containsTag(m.Tags, tag) {
					pool = append(pool, m)
				}
			}
		} else if strings.Contains(requested, "/") {
			for _, m := range all {
				if m.ID == requested && r.eligible(m) {
					pool = append(pool, m)
				}
			}
		} else {
			group := models.ResolveGroup(requested)
			for _, m := range all {
				if models.ResolveGroup(m.ID) == group && r.eligible(m) {
					pool = append(pool, m)
				}
			}
		}
	}

	if len(pool) == 0 {
		return nil
	}

	for i := 0; i < len(pool); i++ {
		for j := i + 1; j < len(pool); j++ {
			if qos(pool[j]) > qos(pool[i]) {
				pool[i], pool[j] = pool[j], pool[i]
			}
		}
	}

	return pool
}

func snapshotByID(all []*models.Model, id string) *models.Model {
	for _, m := range all {
		if m.ID == id {
			return m
		}
	}
	return nil
}

func (r *Router) eligible(m *models.Model) bool {
	if m.Disabled || m.Banned || m.Excluded {
		return false
	}
	if r.registry.CodingOnlyEnabled() && !containsTag(m.Tags, "coding") {
		return false
	}
	if m.Status != "up" {
		return false
	}
	if r.isCooldown(m.ID) {
		return false
	}
	return true
}

func (r *Router) isCooldown(modelID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	until, ok := r.cooldowns[modelID]
	if !ok {
		return false
	}
	if time.Now().After(until) {
		return false
	}
	return true
}

func (r *Router) markFailure(m *models.Model) {
	r.registry.UpdateModel(m.ID, func(x *models.Model) { x.Status = "down" })
}

// forward sends the request to upstream and returns how the call resolved:
//   - err != nil        → connection-level failure, nothing written, retryable
//   - written == true   → response delivered to the client (2xx success, or a
//     non-retryable upstream error proxied verbatim); caller must stop
//   - written == false  → retryable upstream status (429 / 5xx); nothing written
//
// respBody carries the upstream body for non-stream responses (used for
// logging usage); it is nil for streaming responses.
func (r *Router) forward(w http.ResponseWriter, req *http.Request, m *models.Model, body []byte, start time.Time) (bool, int, []byte, time.Duration, error) {
	// Pollinations /text fallback when no API key is available.
	// /v1/chat/completions requires a key (returns 401), so we route
	// to the unauthenticated /text/{prompt} endpoint and wrap the
	// plain-text response as OpenAI-compatible JSON.
	if m.Provider == "pollinations" && m.APIKey == "" {
		return r.forwardPollinationsText(w, m, body, start)
	}

	upstreamBody, err := rewriteModel(body, m.UpstreamModelID)
	if err != nil {
		// Defensive: a body that parsed as a request but is not a JSON
		// object is a client error, not an upstream failure (§7.3 step 6).
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "invalid request body: expected a JSON object",
		})
		return true, http.StatusBadRequest, nil, 0, nil
	}

	upstreamReq, err := http.NewRequest(http.MethodPost, m.Endpoint, bytes.NewReader(upstreamBody))
	if err != nil {
		return false, 0, nil, 0, err
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+m.APIKey)
	}

	client := &http.Client{
		Transport: r.pool.Get(m.ProviderHost),
		Timeout:   120 * time.Second,
	}

	resp, err := client.Do(upstreamReq)
	if err != nil {
		return false, 0, nil, 0, err
	}
	defer resp.Body.Close()

	ttfb := time.Since(start)

	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
		// Retryable: failover to next best model (§7.4)
		_, _ = io.Copy(io.Discard, resp.Body)
		return false, resp.StatusCode, nil, ttfb, nil
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if resp.StatusCode >= 400 {
		// Non-retryable (400/401/403/404/422...): proxy verbatim, no failover
		_, _ = io.Copy(w, resp.Body)
		return true, resp.StatusCode, nil, ttfb, nil
	}

	if isStreamRequest(body) {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return true, resp.StatusCode, nil, ttfb, nil
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			if err != nil {
				break
			}
		}
		return true, resp.StatusCode, nil, ttfb, nil
	}

	respBody, _ := io.ReadAll(resp.Body)
	if _, werr := w.Write(respBody); werr != nil {
		return true, resp.StatusCode, nil, ttfb, nil
	}
	return true, resp.StatusCode, respBody, ttfb, nil
}

// forwardPollinationsText routes a Pollinations model request to the
// unauthenticated /text/{prompt} endpoint and wraps the plain-text
// response as OpenAI-compatible JSON (§7.3 / T066).
//
// The /text endpoint does not support streaming, so stream requests
// are served as a single SSE chunk.
func (r *Router) forwardPollinationsText(w http.ResponseWriter, m *models.Model, body []byte, start time.Time) (bool, int, []byte, time.Duration, error) {
	prompt, err := providers.ConvertOpenAIToPollinations(body, m.ID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": fmt.Sprintf("pollinations /text adapter: %v", err),
		})
		return true, http.StatusBadRequest, nil, 0, nil
	}

	targetURL := providers.BuildPollinationsTextURL(prompt, m.ID)

	client := &http.Client{
		Transport: r.pool.Get("text.pollinations.ai"),
		Timeout:   120 * time.Second,
	}

	resp, err := client.Get(targetURL)
	if err != nil {
		return false, 0, nil, 0, err
	}
	defer resp.Body.Close()

	ttfb := time.Since(start)

	if resp.StatusCode >= 500 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return false, resp.StatusCode, nil, ttfb, nil
	}

	text, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, 0, nil, 0, err
	}

	wrapped := providers.WrapPollinationsResponse(string(text))

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	// For stream requests, serve as a single SSE chunk
	if isStreamRequest(body) {
		_ = writeSSEChunk(w, wrapped)
		_ = writeSSEDone(w)
	} else {
		_, _ = w.Write(wrapped)
	}

	return true, http.StatusOK, wrapped, ttfb, nil
}

// rewriteModel replaces the "model" field with the resolved upstream ID while
// preserving every other field in the request body (§7.3 step 6).
func rewriteModel(body []byte, upstreamID string) ([]byte, error) {
	if len(bytes.TrimSpace(body)) == 0 {
		return body, nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	payload["model"] = upstreamID
	return json.Marshal(payload)
}

func isStreamRequest(body []byte) bool {
	var mReq modelRequest
	if err := json.Unmarshal(body, &mReq); err != nil {
		return false
	}
	return mReq.Stream
}

func extractRequestInfo(body []byte) (string, []string) {
	var payload struct {
		Messages []chatMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil
	}
	var content string
	var toolCalls []string
	for _, msg := range payload.Messages {
		if msg.Content != "" && content == "" {
			content = msg.Content
		}
		for _, tc := range msg.ToolCalls {
			if tc.Function.Name != "" {
				toolCalls = append(toolCalls, tc.Function.Name)
			}
		}
	}
	return content, toolCalls
}

func extractUsage(body []byte) map[string]int {
	if len(body) == 0 {
		return nil
	}
	var payload struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if payload.Usage.TotalTokens == 0 && payload.Usage.PromptTokens == 0 && payload.Usage.CompletionTokens == 0 {
		return nil
	}
	return map[string]int{
		"promptTokens":     payload.Usage.PromptTokens,
		"completionTokens": payload.Usage.CompletionTokens,
		"totalTokens":      payload.Usage.TotalTokens,
	}
}

// hopByHopHeaders are single-hop headers that must not be forwarded to the
// client (RFC 7230 §6.1); the "Connection" header may name additional ones.
var hopByHopHeaders = []string{
	"connection",
	"keep-alive",
	"proxy-authenticate",
	"proxy-authorization",
	"proxy-connection",
	"te",
	"trailer",
	"transfer-encoding",
	"upgrade",
}

func copyHeaders(dst, src http.Header) {
	skip := make(map[string]bool)
	for k := range src {
		if isHopByHop(k) {
			skip[strings.ToLower(k)] = true
			if strings.EqualFold(k, "connection") {
				for _, v := range src[k] {
					for _, name := range strings.Split(v, ",") {
						skip[strings.ToLower(strings.TrimSpace(name))] = true
					}
				}
			}
		}
	}
	for k, vv := range src {
		if skip[strings.ToLower(k)] {
			continue
		}
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func isHopByHop(k string) bool {
	for _, h := range hopByHopHeaders {
		if strings.EqualFold(h, k) {
			return true
		}
	}
	return false
}

func containsTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func qos(m *models.Model) float64 {
	return models.ComputeQoS(m)
}

// writeSSEChunk writes a single SSE data chunk.
func writeSSEChunk(w io.Writer, data []byte) error {
	_, err := fmt.Fprintf(w, "data: %s\n\n", data)
	return err
}

// writeSSEDone writes the SSE stream-termination marker.
func writeSSEDone(w io.Writer) error {
	_, err := fmt.Fprint(w, "data: [DONE]\n\n")
	return err
}
