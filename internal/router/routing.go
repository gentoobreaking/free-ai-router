package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/freemodel/router/internal/models"
)

const MAX_PROACTIVE_RETRIES = 5
const RateLimitCooldown = 60 * time.Second

type Router struct {
	mu           sync.RWMutex
	registry     *models.Registry
	logger       *Logger
	pinned       string
	pinningMode  string
	cooldowns    map[string]time.Time
	modelGroups  map[string][]string
}

type modelRequest struct {
	Model       string `json:"model"`
	Stream      bool   `json:"stream"`
}

func NewRouter(registry *models.Registry, logger *Logger) *Router {
	return &Router{
		registry:    registry,
		logger:      logger,
		pinningMode: "canonical",
		cooldowns:   make(map[string]time.Time),
		modelGroups: make(map[string][]string),
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

		ok, status, ttfb, err := r.forward(w, req, m, body, mReq.Stream, start)
		if !ok || err != nil {
			if err != nil {
				lastErr = err
			}
			r.markFailure(m)
			if status == http.StatusTooManyRequests {
				r.cooldowns[m.ID] = time.Now().Add(RateLimitCooldown)
			}
			continue
		}

		if status >= 500 || status == http.StatusTooManyRequests {
			r.markFailure(m)
			if status == http.StatusTooManyRequests {
				r.cooldowns[m.ID] = time.Now().Add(RateLimitCooldown)
			}
			lastErr = fmt.Errorf("upstream returned %d", status)
			continue
		}

		r.logger.Log(&LogEntry{
			Timestamp: time.Now(),
			Model:     m.ID,
			Provider:  m.Provider,
			Status:    status,
			TTFB:      ttfb,
			Duration:  time.Since(start),
		})
		return
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

	all := r.registry.GetAll()

	var pool []*models.Model
	if pinned != "" {
		if pinningMode == "exact" {
			if m := r.registry.Get(pinned); m != nil && r.eligible(m) {
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

func (r *Router) eligible(m *models.Model) bool {
	if m.Disabled || m.Banned || m.Excluded {
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
	m.Status = "down"
}

func (r *Router) forward(w http.ResponseWriter, req *http.Request, m *models.Model, body []byte, stream bool, start time.Time) (bool, int, time.Duration, error) {
	upstreamReq, err := http.NewRequest(http.MethodPost, m.Endpoint, strings.NewReader(string(body)))
	if err != nil {
		return false, 0, 0, err
	}

	upstreamReq.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		upstreamReq.Header.Set("Authorization", "Bearer "+m.APIKey)
	}

	client := &http.Client{Timeout: 120 * time.Second}

	resp, err := client.Do(upstreamReq)
	if err != nil {
		return false, 0, 0, err
	}
	defer resp.Body.Close()

	ttfb := time.Since(start)

	if resp.StatusCode >= 400 {
		return true, resp.StatusCode, ttfb, fmt.Errorf("upstream %s returned %d", m.ID, resp.StatusCode)
	}

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)

	if stream {
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				if _, werr := w.Write(buf[:n]); werr != nil {
					return true, resp.StatusCode, ttfb, nil
				}
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
			if err != nil {
				break
			}
		}
	} else {
		_, _ = io.Copy(w, resp.Body)
	}

	return true, resp.StatusCode, ttfb, nil
}

func copyHeaders(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
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
