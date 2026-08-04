package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

const DefaultPort = 7352

type Server struct {
	mu       sync.RWMutex
	registry *models.Registry
	cfg      interface{}
	port     int
	router   *Router
	version  string
	logger   *Logger
	handler  http.Handler
}

func NewServer(registry *models.Registry, cfg interface{}, port int, version string, logEnabled bool) *Server {
	s := &Server{
		registry: registry,
		cfg:      cfg,
		port:     port,
		version:  version,
		logger:   NewLogger(logEnabled),
	}
	s.router = NewRouter(registry, s.logger)
	s.handler = s.buildHandler()
	return s
}

func (s *Server) Start() error {
	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	return http.ListenAndServe(addr, s.handler)
}

// SetPool shares the keep-alive transport pool between the ping engine and
// the proxy router (§7.4).
func (s *Server) SetPool(pool *ping.TransportPool) {
	s.router.SetPool(pool)
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) buildHandler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /v1/models", s.handleModels)
	mux.HandleFunc("POST /v1/chat/completions", s.handleChatCompletions)
	mux.HandleFunc("GET /", s.handleIndex)

	mux.HandleFunc("GET /api/models", s.handleAPIModels)
	mux.HandleFunc("GET /api/config", s.handleAPIConfigGet)
	mux.HandleFunc("POST /api/config", s.handleAPIConfigPost)
	mux.HandleFunc("GET /api/meta", s.handleAPIMeta)
	mux.HandleFunc("GET /api/pinned", s.handleAPIPinnedGet)
	mux.HandleFunc("POST /api/pinned", s.handleAPIPinnedPost)
	mux.HandleFunc("GET /api/auto-ping", s.handleAPIAutoPingGet)
	mux.HandleFunc("POST /api/auto-ping", s.handleAPIAutoPingPost)
	mux.HandleFunc("POST /api/models/ban", s.handleAPIModelsBan)
	mux.HandleFunc("POST /api/models/ping", s.handleAPIModelsPing)
	mux.HandleFunc("POST /api/providers/", s.handleAPIProviders)
	mux.HandleFunc("POST /api/providers-refresh-all", s.handleAPIProvidersRefreshAll)
	mux.HandleFunc("POST /api/config/import", s.handleAPIConfigImport)
	mux.HandleFunc("GET /api/config/export", s.handleAPIConfigExport)
	mux.HandleFunc("GET /api/account-status", s.handleAPIAccountStatus)
	mux.HandleFunc("GET /api/autoupdate", s.handleAPIAutoUpdateGet)
	mux.HandleFunc("POST /api/autoupdate", s.handleAPIAutoUpdatePost)
	mux.HandleFunc("PUT /api/models/tags", s.handleAPIModelsTags)
	mux.HandleFunc("GET /api/filter-rules", s.handleAPIFilterRulesGet)
	mux.HandleFunc("POST /api/filter-rules", s.handleAPIFilterRulesPost)
	mux.HandleFunc("GET /api/logs", s.handleAPILogs)

	return mux
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>FreeModel Router</title></head>
<body>
<h1>FreeModel Router</h1>
<p>Router is running on port %d.</p>
<p><code>baseURL: http://127.0.0.1:%d/v1</code></p>
</body></html>`, s.port, s.port)
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	models := s.registry.Snapshot()
	type modelInfo struct {
		ID   string   `json:"id"`
		Tags []string `json:"tags,omitempty"`
	}
	result := make([]modelInfo, 0, len(models))
	for _, m := range models {
		if m.Disabled || m.Banned || m.Excluded {
			continue
		}
		result = append(result, modelInfo{ID: m.ID, Tags: m.Tags})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

func (s *Server) handleChatCompletions(w http.ResponseWriter, r *http.Request) {
	s.router.ServeChatCompletions(w, r)
}

func (s *Server) handleAPIModels(w http.ResponseWriter, r *http.Request) {
	models := s.registry.Snapshot()
	type modelInfo struct {
		ID         string   `json:"id"`
		Provider   string   `json:"provider"`
		Label      string   `json:"label"`
		Context    string   `json:"context"`
		Status     string   `json:"status"`
		AvgLatency float64  `json:"avgLatency"`
		Uptime     float64  `json:"uptime"`
		Verdict    string   `json:"verdict"`
		Tags       []string `json:"tags"`
		Tier       string   `json:"tier"`
	}
	result := make([]modelInfo, 0, len(models))
	for _, m := range models {
		result = append(result, modelInfo{
			ID:         m.ID,
			Provider:   m.Provider,
			Label:      m.Label,
			Context:    m.Context,
			Status:     m.Status,
			AvgLatency: m.AvgLatency,
			Uptime:     m.Uptime,
			Verdict:    VerdictFor(m),
			Tags:       m.Tags,
			Tier:       m.Tier,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

func (s *Server) handleAPIConfigGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg)
}

func (s *Server) handleAPIConfigPost(w http.ResponseWriter, r *http.Request) {
	var payload map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := s.applyConfigPayload(payload); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// applyConfigPayload merges a client-provided partial config into the running
// config and persists it (spec §7.2 /api/config).
func (s *Server) applyConfigPayload(payload map[string]interface{}) error {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		return fmt.Errorf("config not available")
	}
	if v, ok := payload["apiKeys"]; ok {
		if keys, ok := v.(map[string]interface{}); ok {
			cfg.APIKeys = keys
		}
	}
	if v, ok := payload["bannedModels"]; ok {
		if list, ok := v.([]interface{}); ok {
			var banned []string
			for _, item := range list {
				if s, ok := item.(string); ok {
					banned = append(banned, s)
				}
			}
			cfg.BannedModels = banned
		}
	}
	if v, ok := payload["codingOnly"]; ok {
		if b, ok := v.(bool); ok {
			cfg.CodingOnly = b
		}
	}
	if v, ok := payload["autoPingEnabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.AutoPingEnabled = b
		}
	}
	if v, ok := payload["excludedProviders"]; ok {
		if list, ok := v.([]interface{}); ok {
			var excluded []string
			for _, item := range list {
				if s, ok := item.(string); ok {
					excluded = append(excluded, s)
				}
			}
			cfg.ExcludedProviders = excluded
		}
	}
	return config.Save(cfg)
}

func (s *Server) handleAPIMeta(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"version":         s.version,
		"updateAvailable": false,
	})
}

func (s *Server) handleAPIPinnedGet(w http.ResponseWriter, r *http.Request) {
	pinned, mode := s.router.Pinned()
	writeJSON(w, http.StatusOK, map[string]interface{}{"pinned": pinned, "mode": mode})
}

func (s *Server) handleAPIPinnedPost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModelID string `json:"modelId"`
		Mode    string `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	s.router.SetPinned(payload.ModelID, payload.Mode)
	writeJSON(w, http.StatusOK, map[string]interface{}{"pinned": payload.ModelID, "mode": payload.Mode})
}

func (s *Server) handleAPIAutoPingGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"autoPing": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"autoPing": cfg.AutoPingEnabled})
}

func (s *Server) handleAPIAutoPingPost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AutoPing *bool `json:"autoPing"`
	}
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if cfg, ok := s.cfg.(*config.Config); ok && cfg != nil && payload.AutoPing != nil {
		cfg.AutoPingEnabled = *payload.AutoPing
		if err := config.Save(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPIModelsBan(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModelID string `json:"modelId"`
		Banned  *bool  `json:"banned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	m := s.registry.Get(payload.ModelID)
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	s.registry.UpdateModel(payload.ModelID, func(x *models.Model) {
		if payload.Banned != nil {
			x.Banned = *payload.Banned
		} else {
			x.Banned = !x.Banned
		}
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"banned": m.Banned})
}

func (s *Server) handleAPIModelsPing(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPIProviders(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/providers/")
	writeJSON(w, http.StatusOK, map[string]interface{}{"refreshed": path})
}

func (s *Server) handleAPIProvidersRefreshAll(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPIConfigImport(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	imported, err := config.ImportToken(payload.Token)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if err := config.Save(imported); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if cfg, ok := s.cfg.(*config.Config); ok {
		*cfg = *imported
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPIConfigExport(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]string{"token": ""})
		return
	}
	token, err := config.ExportToken(cfg)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"token": token})
}

func (s *Server) handleAPIAccountStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
}

func (s *Server) handleAPIAutoUpdateGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"enabled": true, "intervalHours": 24})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"enabled":       cfg.AutoUpdate.Enabled,
		"intervalHours": cfg.AutoUpdate.IntervalHours,
		"lastCheckAt":   cfg.AutoUpdate.LastCheckAt,
	})
}

func (s *Server) handleAPIAutoUpdatePost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Enabled       *bool `json:"enabled"`
		IntervalHours *int  `json:"intervalHours"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config not available"})
		return
	}
	if payload.Enabled != nil {
		cfg.AutoUpdate.Enabled = *payload.Enabled
	}
	if payload.IntervalHours != nil && *payload.IntervalHours > 0 {
		cfg.AutoUpdate.IntervalHours = *payload.IntervalHours
	}
	if err := config.Save(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPIModelsTags(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModelID string   `json:"modelId"`
		Tags    []string `json:"tags"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	m := s.registry.Get(payload.ModelID)
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	s.registry.UpdateModel(payload.ModelID, func(x *models.Model) { x.Tags = payload.Tags })
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": m.Tags})
}

func (s *Server) handleAPIFilterRulesGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"minSweScore": nil, "excludedProviders": []string{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"minSweScore":       cfg.MinSweScore,
		"excludedProviders": cfg.ExcludedProviders,
	})
}

func (s *Server) handleAPIFilterRulesPost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		MinSweScore       *float64  `json:"minSweScore"`
		ExcludedProviders *[]string `json:"excludedProviders"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "config not available"})
		return
	}
	if payload.MinSweScore != nil {
		cfg.MinSweScore = payload.MinSweScore
	}
	if payload.ExcludedProviders != nil {
		cfg.ExcludedProviders = *payload.ExcludedProviders
	}
	if err := config.Save(cfg); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleAPILogs(w http.ResponseWriter, r *http.Request) {
	logs := s.logger.Recent()
	writeJSON(w, http.StatusOK, map[string]interface{}{"logs": logs})
}

func writeJSON(w http.ResponseWriter, code int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func VerdictFor(m *models.Model) string {
	if m.Status == "ratelimit" {
		return "Overloaded"
	}
	if m.Status == "pending" {
		return "Pending"
	}
	if m.Status != "up" {
		return "Not Active"
	}
	if m.AvgLatency == 0 {
		return "Pending"
	}
	switch {
	case m.AvgLatency < 400:
		return "Perfect"
	case m.AvgLatency < 1000:
		return "Normal"
	case m.AvgLatency < 3000:
		return "Slow"
	case m.AvgLatency < 5000:
		return "Very Slow"
	default:
		return "Unusable"
	}
}

func logFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".freemodel-router-logs.json"), nil
}
