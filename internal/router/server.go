package router

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/providers"
)

const DefaultPort = 7352

type Server struct {
	registry  *models.Registry
	cfg       interface{}
	port      int
	router    *Router
	version   string
	logger    *Logger
	handler   http.Handler
	providers *providers.Manager
	engine    *ping.Engine

	updateMu        sync.Mutex
	updateCheck     func() (string, error)
	updateChecking  bool
	updateLastCheck time.Time
	updateAvailable bool
	updateURL       string
}

const updateCheckTTL = 30 * time.Minute

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

// SetProviders wires the provider source manager used by discovery endpoints.
func (s *Server) SetProviders(mgr *providers.Manager) {
	s.providers = mgr
}

// SetEngine wires the ping engine so /api/auto-ping can start/stop it.
func (s *Server) SetEngine(engine *ping.Engine) {
	s.engine = engine
}

// SetUpdateChecker wires the update check (cli.CheckForUpdate); results are
// cached with a TTL so /api/meta never blocks on repeated network calls.
func (s *Server) SetUpdateChecker(fn func() (string, error)) {
	s.updateCheck = fn
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
			Verdict:    ping.GetVerdict(m),
			Tags:       m.Tags,
			Tier:       m.Tier,
		})
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"data": result})
}

func (s *Server) handleAPIConfigGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	cfg.RLock()
	data, err := json.Marshal(cfg)
	cfg.RUnlock()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
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
	cfg.Lock()
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
	cfg.Unlock()
	return config.Save(cfg)
}

func (s *Server) handleAPIMeta(w http.ResponseWriter, r *http.Request) {
	updateAvailable, updateURL := s.maybeCheckUpdate()
	resp := map[string]interface{}{
		"version":         s.version,
		"updateAvailable": updateAvailable,
	}
	if updateAvailable {
		resp["updateUrl"] = updateURL
	}
	writeJSON(w, http.StatusOK, resp)
}

// maybeCheckUpdate runs the update checker at most every updateCheckTTL with
// singleflight; failures degrade to "no update" instead of erroring.
func (s *Server) maybeCheckUpdate() (bool, string) {
	s.updateMu.Lock()
	if s.updateCheck == nil {
		s.updateMu.Unlock()
		return false, ""
	}
	if time.Since(s.updateLastCheck) < updateCheckTTL || s.updateChecking {
		available, url := s.updateAvailable, s.updateURL
		s.updateMu.Unlock()
		return available, url
	}
	s.updateChecking = true
	s.updateMu.Unlock()

	url, err := s.updateCheck()

	s.updateMu.Lock()
	s.updateChecking = false
	s.updateLastCheck = time.Now()
	if err != nil {
		s.updateAvailable = false
		s.updateURL = ""
	} else {
		s.updateAvailable = url != ""
		s.updateURL = url
	}
	available, out := s.updateAvailable, s.updateURL
	s.updateMu.Unlock()
	return available, out
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
	cfg.RLock()
	autoPing := cfg.AutoPingEnabled
	cfg.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"autoPing": autoPing})
}

// handleAPIAutoPingPost persists the toggle and starts/stops the ping engine.
func (s *Server) handleAPIAutoPingPost(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		AutoPing *bool `json:"autoPing"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if cfg, ok := s.cfg.(*config.Config); ok && cfg != nil && payload.AutoPing != nil {
		cfg.Lock()
		cfg.AutoPingEnabled = *payload.AutoPing
		cfg.Unlock()
		if err := config.Save(cfg); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if s.engine != nil {
			if *payload.AutoPing {
				s.engine.Start()
			} else {
				s.engine.Stop()
			}
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
	var banned bool
	s.registry.UpdateModel(payload.ModelID, func(x *models.Model) {
		if payload.Banned != nil {
			x.Banned = *payload.Banned
		} else {
			x.Banned = !x.Banned
		}
		banned = x.Banned
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"banned": banned})
}

// handleAPIModelsPing runs a real synchronous single-model ping and reports
// the outcome without mutating registry state (spec §7.2).
func (s *Server) handleAPIModelsPing(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		ModelID string `json:"modelId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if payload.ModelID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "modelId is required"})
		return
	}
	m := s.registry.Get(payload.ModelID)
	if m == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "model not found"})
		return
	}
	status, httpCode, latency := pingModelNow(m)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"modelId":   m.ID,
		"status":    status,
		"httpCode":  httpCode,
		"latencyMs": latency.Milliseconds(),
	})
}

// pingModelNow performs a single ping against the model's endpoint with a
// short timeout (mirrors the TUI test-ping path).
func pingModelNow(m *models.Model) (string, int, time.Duration) {
	if m.Endpoint == "" {
		return "down", 0, 0
	}
	start := time.Now()
	body := `{"model":"` + m.UpstreamModelID + `","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`
	req, err := http.NewRequest(http.MethodPost, m.Endpoint, strings.NewReader(body))
	if err != nil {
		return "down", 0, 0
	}
	req.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "down", 0, time.Since(start)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return ping.StatusFromCode(resp.StatusCode), resp.StatusCode, time.Since(start)
}

// handleAPIProviders runs discovery for a single provider and merges the
// discovered models into the registry (spec §7.2 /api/providers/<key>).
func (s *Server) handleAPIProviders(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/providers/"), "/")
	if key == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider key is required"})
		return
	}
	if s.providers == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider manager not available"})
		return
	}
	if s.providers.GetProvider(key) == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "unknown provider"})
		return
	}
	discovered, err := s.providers.DiscoverModels(key)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	added := s.mergeDiscovered(key, discovered)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"provider":   key,
		"discovered": len(discovered),
		"added":      added,
	})
}

// handleAPIProvidersRefreshAll discovers models for every keyed, enabled,
// discoverable provider and merges the results into the registry.
func (s *Server) handleAPIProvidersRefreshAll(w http.ResponseWriter, r *http.Request) {
	if s.providers == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "provider manager not available"})
		return
	}
	cfg, _ := s.cfg.(*config.Config)
	refreshed := []string{}
	added := 0
	for _, p := range s.providers.GetAllProviders() {
		if !p.Discoverable || !providerEnabled(cfg, p.Key) {
			continue
		}
		discovered, err := s.providers.DiscoverModels(p.Key)
		if err != nil {
			continue
		}
		added += s.mergeDiscovered(p.Key, discovered)
		refreshed = append(refreshed, p.Key)
	}
	writeJSON(w, http.StatusOK, map[string]interface{}{"refreshed": refreshed, "added": added})
}

func providerEnabled(cfg *config.Config, key string) bool {
	if cfg == nil {
		return true
	}
	if config.ResolveAPIKey(key, cfg) == "" {
		return false
	}
	if pcfg, ok := cfg.Providers[key]; ok {
		return pcfg.Enabled
	}
	return true
}

// mergeDiscovered adds discovered models that are not already in the registry
// and returns the number of new models.
func (s *Server) mergeDiscovered(providerKey string, discovered []providers.ModelEntry) int {
	added := 0
	for _, me := range discovered {
		if me.ID == "" || s.registry.Get(me.ID) != nil {
			continue
		}
		m := &models.Model{
			ID:       me.ID,
			Provider: providerKey,
			Label:    me.Label,
			Context:  me.Context,
			Status:   "pending",
		}
		parts := strings.SplitN(me.ID, "/", 2)
		if len(parts) == 2 {
			m.UpstreamModelID = parts[1]
		}
		if p := s.providers.GetProvider(providerKey); p != nil {
			m.Endpoint = p.URL
			m.ProviderHost = p.BaseURL
		}
		s.registry.Add(m)
		added++
	}
	return added
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
		cfg.ReplaceWith(imported)
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

// handleAPIAccountStatus reports per-provider key counts and enabled state
// from config + env overrides (spec §7.2 /api/account-status).
func (s *Server) handleAPIAccountStatus(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"accounts": []interface{}{}})
		return
	}
	names := []string{}
	seen := map[string]bool{}
	addName := func(n string) {
		if n != "" && !seen[n] {
			seen[n] = true
			names = append(names, n)
		}
	}
	cfg.RLock()
	for name := range cfg.Providers {
		addName(name)
	}
	for name := range cfg.APIKeys {
		addName(name)
	}
	accounts := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		pcfg := cfg.Providers[name]
		accounts = append(accounts, map[string]interface{}{
			"provider": name,
			"keyCount": len(config.ResolveAPIKeys(name, cfg)),
			"enabled":  pcfg.Enabled,
		})
	}
	cfg.RUnlock()
	writeJSON(w, http.StatusOK, map[string]interface{}{"accounts": accounts})
}

func (s *Server) handleAPIAutoUpdateGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"enabled": true, "intervalHours": 24})
		return
	}
	cfg.RLock()
	resp := map[string]interface{}{
		"enabled":       cfg.AutoUpdate.Enabled,
		"intervalHours": cfg.AutoUpdate.IntervalHours,
		"lastCheckAt":   cfg.AutoUpdate.LastCheckAt,
	}
	cfg.RUnlock()
	writeJSON(w, http.StatusOK, resp)
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
	cfg.Lock()
	if payload.Enabled != nil {
		cfg.AutoUpdate.Enabled = *payload.Enabled
	}
	if payload.IntervalHours != nil && *payload.IntervalHours > 0 {
		cfg.AutoUpdate.IntervalHours = *payload.IntervalHours
	}
	cfg.Unlock()
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
	var tags []string
	s.registry.UpdateModel(payload.ModelID, func(x *models.Model) {
		x.Tags = payload.Tags
		tags = x.Tags
	})
	writeJSON(w, http.StatusOK, map[string]interface{}{"tags": tags})
}

func (s *Server) handleAPIFilterRulesGet(w http.ResponseWriter, r *http.Request) {
	cfg, ok := s.cfg.(*config.Config)
	if !ok || cfg == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{"minSweScore": nil, "excludedProviders": []string{}})
		return
	}
	cfg.RLock()
	resp := map[string]interface{}{
		"minSweScore":       cfg.MinSweScore,
		"excludedProviders": cfg.ExcludedProviders,
	}
	cfg.RUnlock()
	writeJSON(w, http.StatusOK, resp)
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
	cfg.Lock()
	if payload.MinSweScore != nil {
		cfg.MinSweScore = payload.MinSweScore
	}
	if payload.ExcludedProviders != nil {
		cfg.ExcludedProviders = *payload.ExcludedProviders
	}
	cfg.Unlock()
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

func logFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".freemodel-router-logs.json"), nil
}
