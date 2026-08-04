package providers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const cacheFileName = "merged-cache.json"

// DefaultCacheTTL is the default cache lifetime.
var DefaultCacheTTL = defaultCacheTTL

const defaultCacheTTL = 60 * time.Minute

// ProviderCache is the on-disk structure for caching merged provider results.
type ProviderCache struct {
	Version      int                        `json:"version"`
	CachedAt     string                     `json:"cached_at"`
	SourcesHash  string                     `json:"sources_hash"`
	TTLMinutes   int                        `json:"ttl_minutes"`
	Stats        CacheStats                 `json:"stats"`
	Providers    map[string]*CachedProvider `json:"providers"`
}

type CacheStats struct {
	StaticModels   int `json:"static_models"`
	ClawlabsModels int `json:"clawlabs_models"`
	RelaySites     int `json:"relay_sites"`
	TotalModels    int `json:"total_models"`
}

type CachedProvider struct {
	Key          string       `json:"key"`
	Name         string       `json:"name"`
	URL          string       `json:"url"`
	Discoverable bool         `json:"discoverable"`
	Models       []ModelEntry `json:"models"`
	Enabled      bool         `json:"enabled"`
	BaseURL      string       `json:"base_url"`
}

// cachePath resolves the full path to the cache file.
func cachePath(dataDir string) string {
	return filepath.Join(dataDir, "data", cacheFileName)
}

// computeSourcesHash returns the SHA-256 hex digest of raw bytes.
func computeSourcesHash(data []byte) string {
	h := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%x", h)
}

// loadCache reads and validates the provider cache from disk.
// Returns nil on any error (missing, corrupted, stale — callers decide policy).
func loadCache(path string, sourcesHash string, ttl time.Duration) *ProviderCache {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var cache ProviderCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil
	}
	if cache.Version != 1 {
		return nil
	}
	if cache.SourcesHash != sourcesHash {
		return nil
	}
	if cache.Providers == nil || len(cache.Providers) == 0 {
		return nil
	}
	cachedAt, err := time.Parse(time.RFC3339, cache.CachedAt)
	if err != nil {
		return nil
	}
	if time.Since(cachedAt) > ttl {
		return nil
	}
	return &cache
}

// saveCache serialises the provider map to disk with mode 0600.
// Caller must hold the Manager lock.
func saveCache(providers map[string]*Provider, dataDir string, sourcesHash string, ttlMinutes int) {
	var cp map[string]*CachedProvider
	cp = make(map[string]*CachedProvider, len(providers))
	for k, v := range providers {
		cp[k] = &CachedProvider{
			Key:          v.Key,
			Name:         v.Name,
			URL:          v.URL,
			Discoverable: v.Discoverable,
			Models:       v.Models,
			Enabled:      v.Enabled,
			BaseURL:      v.BaseURL,
		}
	}

	var staticModels, clawlabsModels, relaySites, total int
	for k, p := range cp {
		n := len(p.Models)
		total += n
		switch {
		case k == "clawlabs":
			clawlabsModels = n
		case len(k) > 6 && k[:6] == "relay-":
			relaySites++
		default:
			staticModels += n
		}
	}

	cache := ProviderCache{
		Version:     1,
		CachedAt:    time.Now().UTC().Format(time.RFC3339),
		SourcesHash: sourcesHash,
		TTLMinutes:  ttlMinutes,
		Stats: CacheStats{
			StaticModels:   staticModels,
			ClawlabsModels: clawlabsModels,
			RelaySites:     relaySites,
			TotalModels:    total,
		},
		Providers: cp,
	}

	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return
	}

	p := cachePath(dataDir)
	os.MkdirAll(filepath.Dir(p), 0700)
	_ = os.WriteFile(p, data, 0600)
}

// restoreFromCache re-populates the in-memory provider map from a cached
// snapshot, skipping all HTTP discovery work.
// Caller must hold the Manager lock.
func restoreFromCache(m *Manager, cache *ProviderCache) {
	for k, v := range cache.Providers {
		models := make([]ModelEntry, len(v.Models))
		copy(models, v.Models)
		m.providers[k] = &Provider{
			Key:          v.Key,
			Name:         v.Name,
			URL:          v.URL,
			Discoverable: v.Discoverable,
			Models:       models,
			Enabled:      v.Enabled,
			BaseURL:      v.BaseURL,
		}
	}
}
