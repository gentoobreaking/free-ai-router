package providers

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type SourceProvider struct {
	Name         string          `json:"name"`
	URL          string          `json:"url"`
	Discoverable bool            `json:"discoverable,omitempty"`
	Models       [][]interface{} `json:"models"`
}

type Provider struct {
	Key          string
	Name         string
	URL          string
	Discoverable bool
	Models       []ModelEntry
	Enabled      bool
	BaseURL      string
}

type ModelEntry struct {
	ID      string
	Label   string
	Context string
}

type Manager struct {
	mu        sync.RWMutex
	providers map[string]*Provider
	logger    DiscoveryLogger
}

func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]*Provider),
	}
}

// SetLogger attaches a DiscoveryLogger for the four-phase discovery pipeline.
// Set nil to suppress all discovery output.
func (m *Manager) SetLogger(l DiscoveryLogger) {
	m.logger = l
}

func (m *Manager) LoadSources(path string) error {
	return m.LoadSourcesWithCache(path, defaultCacheTTL, false)
}

// LoadSourcesWithCache loads providers from sources.json, with an optional
// on-disk cache to avoid repeated HTTP discovery work. Set forceRefresh to
// true to skip cache reads (but still write a fresh cache after success).
func (m *Manager) LoadSourcesWithCache(path string, cacheTTL time.Duration, forceRefresh bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	sourcesHash := computeSourcesHash(data)
	dataDir := filepath.Dir(filepath.Dir(path)) // path is data/sources.json → parent is data dir parent

	// Try cache (skip if --refresh)
	if !forceRefresh {
		if cache := loadCache(cachePath(dataDir), sourcesHash, cacheTTL); cache != nil {
			restoreFromCache(m, cache)
			return nil
		}
	}

	// ── Full discovery (existing logic) ──
	log := loggerFor(m)

	var sources map[string]SourceProvider
	if err := json.Unmarshal(data, &sources); err != nil {
		return err
	}

	log.Info("phase=static loading %d provider definitions", len(sources))

	freeOpenRouterModels := m.fetchFreeOpenRouterModels()
	clawLabsModels := m.fetchClawLabsModels()

		for key, src := range sources {
			models := make([]ModelEntry, 0, len(src.Models))
			for _, me := range src.Models {
				if len(me) >= 2 {
					id, _ := me[0].(string)
					label, _ := me[1].(string)
					context := ""
					if len(me) >= 3 {
						context, _ = me[2].(string)
					}
					if key == "openrouter" && !freeOpenRouterModels[id] {
						continue
					}
					models = append(models, ModelEntry{ID: id, Label: label, Context: context})
				}
			}

			if key == "openrouter" {
				log.Info("static openrouter: %d models total, %d free-tier eligible", len(src.Models), len(models))
			} else if len(models) > 0 {
				log.Debug("static %s: %d models", key, len(models))
			}

			m.providers[key] = &Provider{
				Key:          key,
				Name:         src.Name,
				URL:          src.URL,
				Discoverable: src.Discoverable,
				Models:       models,
				Enabled:      true,
				BaseURL:      extractBaseURL(src.URL),
			}
		}

		// Merge ClawLabs aggregated models as a separate provider
		log.Info("phase=clawlabs merged %d models into provider \"clawlabs\"", len(clawLabsModels))
		m.providers["clawlabs"] = &Provider{
			Key:          "clawlabs",
			Name:         "ClawLabs Free Models",
			URL:          "",
			Discoverable: false,
			Models:       clawLabsModels,
			Enabled:      true,
			BaseURL:      "",
		}

		// Discover and add relay sites from community forums
		log.Info("phase=relay_scan starting")
		relaySites := ScannedRelaySites(log)
		log.Info("relay_scan result: %d sites found", len(relaySites))
		healthyRelayCount := 0
		for _, site := range relaySites {
			if !site.Healthy {
				continue
			}
			healthyRelayCount++
			providerKey := "relay-" + sanitizeRelayKey(site.BaseURL)
			if _, exists := m.providers[providerKey]; !exists {
				models, err := DiscoverModelsFromRelay(site.BaseURL, providerKey)
				if err != nil || len(models) == 0 {
					log.Warn("relay_scan skip %s: no models or error", site.BaseURL)
					continue
				}
				log.Info("relay_scan registered %s (%d models)", providerKey, len(models))
				m.providers[providerKey] = &Provider{
					Key:          providerKey,
					Name:         "Public Relay: " + site.BaseURL,
					URL:          site.BaseURL + "/v1/chat/completions",
					Discoverable: false,
					Models:       models,
					Enabled:      true,
					BaseURL:      site.BaseURL,
				}
			}
		}

		// Summary
		totalModels := 0
		for _, p := range m.providers {
			totalModels += len(p.Models)
		}
		log.Info("summary: %d providers, ~%d models total", len(m.providers), totalModels)

		// Save merged result to cache
		ttlMinutes := int(cacheTTL.Minutes())
		saveCache(m.providers, dataDir, sourcesHash, ttlMinutes)

		return nil
	}

	// Auto-discover models from discoverable providers (called after LoadSources returns).
	func (m *Manager) AutoDiscoverModels() {
		log := loggerFor(m)
		m.mu.RLock()
		providerKeys := make([]string, 0, len(m.providers))
		for key := range m.providers {
			providerKeys = append(providerKeys, key)
		}
		m.mu.RUnlock()

		log.Info("phase=autodiscover scanning %d discoverable providers", len(providerKeys))

		newCount := 0
		for _, key := range providerKeys {
			p := m.GetProvider(key)
			if p == nil || !p.Discoverable || p.BaseURL == "" {
				continue
			}
			discovered, err := m.DiscoverModels(key)
			if err != nil {
				log.Warn("autodiscover %s: error %v (skipped)", key, err)
				continue
			}
			if len(discovered) == 0 {
				continue
			}
			m.mu.Lock()
			prov, ok := m.providers[key]
			if ok {
				existing := make(map[string]bool, len(prov.Models))
				for _, em := range prov.Models {
					existing[em.ID] = true
				}
				localNew := 0
				for _, dm := range discovered {
					if !existing[dm.ID] {
						prov.Models = append(prov.Models, dm)
						localNew++
					}
				}
				if localNew > 0 {
					newCount += localNew
					log.Info("autodiscover %s: +%d new models (total %d)", key, localNew, len(prov.Models))
				} else {
					log.Debug("autodiscover %s: 0 new (all %d already known)", key, len(prov.Models))
				}
			}
			m.mu.Unlock()
		}
		if newCount > 0 {
			log.Info("autodiscover result: +%d new models total", newCount)
		} else {
			log.Info("autodiscover result: 0 new models")
		}
	}

// sanitizeRelayKey converts a URL to a safe provider key
func sanitizeRelayKey(baseURL string) string {
	// Remove protocol and replace special chars
	s := strings.TrimPrefix(baseURL, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, ":", "-")
	return s
}

func (m *Manager) fetchFreeOpenRouterModels() map[string]bool {
	resp, err := http.Get("https://openrouter.ai/api/v1/models")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			Pricing struct {
				Prompt     string `json:"prompt"`
				Completion string `json:"completion"`
			} `json:"pricing"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil
	}

	free := make(map[string]bool)
	for _, model := range result.Data {
		if model.Pricing.Prompt == "0" && model.Pricing.Completion == "0" {
			free[model.ID] = true
		}
	}
	return free
}

// fetchClawLabsModels fetches free models from OpenRouter (no auth) and
// combines them with Pollinations AI static entries, matching the
// ClawLabsAI/free-ai-models approach: daily-updated, no API keys needed.
func (m *Manager) fetchClawLabsModels() []ModelEntry {
	var entries []ModelEntry

	// 1. Fetch free OpenRouter models (no auth required)
	free := m.fetchFreeOpenRouterModels()
	if free != nil {
		for id := range free {
			parts := strings.SplitN(id, "/", 2)
			label := id
			if len(parts) == 2 {
				label = parts[1]
			}
			entries = append(entries, ModelEntry{
				ID:      id,
				Label:   label,
				Context: "",
			})
		}
	}

	// 2. Add Pollinations AI static models (no auth, unlimited)
	pollinations := []ModelEntry{
		{ID: "pollinations/openai", Label: "OpenAI", Context: "128k"},
		{ID: "pollinations/openai-fast", Label: "OpenAI Fast", Context: "128k"},
		{ID: "pollinations/gpt-oss", Label: "GPT OSS", Context: "128k"},
		{ID: "pollinations/deepseek", Label: "DeepSeek", Context: "128k"},
		{ID: "pollinations/deepseek-pro", Label: "DeepSeek Pro", Context: "128k"},
		{ID: "pollinations/gemini", Label: "Gemini", Context: "128k"},
		{ID: "pollinations/gemini-3-flash", Label: "Gemini 3 Flash", Context: "128k"},
		{ID: "pollinations/gemini-flash-lite-3.5", Label: "Gemini Flash Lite 3.5", Context: "128k"},
		{ID: "pollinations/gemini-fast", Label: "Gemini Fast", Context: "128k"},
		{ID: "pollinations/mistral", Label: "Mistral", Context: "128k"},
		{ID: "pollinations/mistral-small-3.2", Label: "Mistral Small 3.2", Context: "128k"},
		{ID: "pollinations/qwen-coder", Label: "Qwen Coder", Context: "128k"},
		{ID: "pollinations/kimi-k3", Label: "Kimi K3", Context: "128k"},
		{ID: "pollinations/claude", Label: "Claude", Context: "200k"},
		{ID: "pollinations/claude-fast", Label: "Claude Fast", Context: "200k"},
		{ID: "pollinations/claude-sonnet-5", Label: "Claude Sonnet 5", Context: "200k"},
		{ID: "pollinations/command-a-plus", Label: "Command A Plus", Context: "128k"},
		{ID: "pollinations/mercury", Label: "Mercury", Context: "128k"},
	}
	entries = append(entries, pollinations...)

	return entries
}

func (m *Manager) GetProvider(key string) *Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.providers[key]
	if !ok {
		return nil
	}
	clone := *p
	return &clone
}

func (m *Manager) GetAllProviders() map[string]*Provider {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]*Provider, len(m.providers))
	for k, v := range m.providers {
		clone := *v
		result[k] = &clone
	}
	return result
}

func (m *Manager) GetAllModels() []ModelEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []ModelEntry
	for _, p := range m.providers {
		result = append(result, p.Models...)
	}
	return result
}

func (m *Manager) MergeDiscoveredModels(providerKey string, models []ModelEntry) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.providers[providerKey]
	if !ok {
		return
	}

	existing := make(map[string]bool)
	for _, em := range p.Models {
		existing[em.ID] = true
	}

	for _, nm := range models {
		if !existing[nm.ID] {
			p.Models = append(p.Models, nm)
		}
	}
}

func (m *Manager) DiscoverModels(providerKey string) ([]ModelEntry, error) {
	p := m.GetProvider(providerKey)
	if p == nil || !p.Discoverable || p.BaseURL == "" {
		return nil, nil
	}

	// Google's models API lives under /v1beta/models, others use /v1/models.
	discoveryPath := "/v1/models"
	if providerKey == "googleai" {
		discoveryPath = "/v1beta/models"
	}
	discoveryURL := p.BaseURL + discoveryPath

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(discoveryURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	var models []ModelEntry
	for _, d := range result.Data {
		id := providerKey + "/" + d.ID
		if providerKey == "openrouter" {
			free := m.fetchFreeOpenRouterModels()
			if !free[id] {
				continue
			}
		}
		models = append(models, ModelEntry{ID: id})
	}

	return models, nil
}

func extractBaseURL(url string) string {
	if url == "" {
		return ""
	}

	parts := strings.SplitN(url, "/", 4)
	if len(parts) < 3 {
		return url
	}

	idx := strings.Index(url, "/v1/")
	if idx >= 0 {
		return url[:idx]
	}

	idx = strings.Index(url, "/v1beta/")
	if idx >= 0 {
		return url[:idx]
	}

	idx = strings.Index(url, "/v2/")
	if idx >= 0 {
		return url[:idx]
	}

	return url
}

func EnvVarForProvider(provider string) string {
	if env := GetProviderEnvVar(provider); env != "" {
		if val := os.Getenv(env); val != "" {
			return val
		}
		return ""
	}
	return ""
}

// DataDir resolves the directory containing the data/ folder:
//  1. $FREMODEL_DATA_DIR if set
//  2. the compile-time source directory (dev machines)
//  3. the executable's directory (containers where the binary sits next to data/)
func DataDir() string {
	if env := os.Getenv("FREMODEL_DATA_DIR"); env != "" {
		return env
	}
	_, filename, _, _ := runtime.Caller(0)
	srcDir := filepath.Dir(filepath.Dir(filepath.Dir(filename)))
	if _, err := os.Stat(filepath.Join(srcDir, "data")); err == nil {
		return srcDir
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		if _, err := os.Stat(filepath.Join(exeDir, "data")); err == nil {
			return exeDir
		}
	}
	return srcDir
}

var (
	_ = filepath.Join
	_ = strings.Join
)
