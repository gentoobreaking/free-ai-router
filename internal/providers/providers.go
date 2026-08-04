package providers

import (
	"encoding/json"
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
}

func NewManager() *Manager {
	return &Manager{
		providers: make(map[string]*Provider),
	}
}

func (m *Manager) LoadSources(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var sources map[string]SourceProvider
	if err := json.Unmarshal(data, &sources); err != nil {
		return err
	}

	for key, src := range sources {
		models := make([]ModelEntry, 0, len(src.Models))
		for _, m := range src.Models {
			if len(m) >= 2 {
				id, _ := m[0].(string)
				label, _ := m[1].(string)
				context := ""
				if len(m) >= 3 {
					context, _ = m[2].(string)
				}
				models = append(models, ModelEntry{ID: id, Label: label, Context: context})
			}
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

	return nil
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
		models = append(models, ModelEntry{
			ID: providerKey + "/" + d.ID,
		})
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
	envMap := map[string]string{
		"nvidia":            "NVIDIA_API_KEY",
		"groq":              "GROQ_API_KEY",
		"cerebras":          "CEREBRAS_API_KEY",
		"opencode":          "OPENCODE_API_KEY",
		"openrouter":        "OPENROUTER_API_KEY",
		"openai-compatible": "OPENAI_COMPATIBLE_API_KEY",
		"ollama":            "OLLAMA_API_KEY",
		"codestral":         "CODESTRAL_API_KEY",
		"scaleway":          "SCALEWAY_API_KEY",
		"kilocode":          "KILOCODE_API_KEY",
		"googleai":          "GOOGLE_API_KEY",
	}

	if env, ok := envMap[provider]; ok {
		if val := os.Getenv(env); val != "" {
			return val
		}
	}

	return ""
}

func DataDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Dir(filepath.Dir(filepath.Dir(filename)))
}

var (
	_ = filepath.Join
	_ = strings.Join
)
