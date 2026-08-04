package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// RelaySite represents a discovered public new-api relay instance
type RelaySite struct {
	BaseURL    string `json:"base_url"`
	Models     []ModelEntry
	LastCheck  time.Time `json:"last_check"`
	Healthy    bool `json:"healthy"`
	LastSuccess time.Time `json:"last_success"`
}

// ScannedRelaySites returns discovered relay sites from community forums.
// log receives structured progress output during scanning.
func ScannedRelaySites(log DiscoveryLogger) []*RelaySite {
	var sites []*RelaySite

	log.Debug("relay_scan v2ex: scraping go/ai")
	v2exSites := scanV2EXRelaySites(log)
	log.Debug("relay_scan v2ex: %d relay candidates", len(v2exSites))
	sites = append(sites, v2exSites...)

	log.Debug("relay_scan linuxdo: scraping c/ai/analysis")
	ldSites := scanLinuxDoRelaySites(log)
	log.Debug("relay_scan linuxdo: %d relay candidates", len(ldSites))
	sites = append(sites, ldSites...)

	// Deduplicate by base URL
	seen := make(map[string]bool)
	var unique []*RelaySite
	for _, s := range sites {
		if !seen[s.BaseURL] {
			seen[s.BaseURL] = true
			unique = append(unique, s)
		}
	}

	log.Debug("relay_scan dedup: %d → %d unique URLs", len(sites), len(unique))

	// Validate each candidate
	healthy := 0
	for _, s := range unique {
		if ValidateNewApiRelay(s.BaseURL) {
			s.Healthy = true
			s.LastSuccess = time.Now()
			healthy++
			log.Info("relay_scan validate %s → healthy", s.BaseURL)
		} else {
			log.Debug("relay_scan validate %s → unhealthy (skipped)", s.BaseURL)
		}
	}

	if len(unique) > 0 {
		log.Info("relay_scan result: %d tested, %d healthy, %d failed", len(unique), healthy, len(unique)-healthy)
	} else {
		log.Info("relay_scan result: 0 sites found (normal in restricted networks)")
	}

	return unique
}

// scanV2EXRelaySites scans V2EX go/ai node for public new-api relay sites
func scanV2EXRelaySites(log DiscoveryLogger) []*RelaySite {
	return scanForumRelaySites("https://www.v2ex.com/go/ai", v2exKeywords, log)
}

// scanLinuxDoRelaySites scans linux.do AI board for public new-api relay sites
func scanLinuxDoRelaySites(log DiscoveryLogger) []*RelaySite {
	return scanForumRelaySites("https://linux.do/c/ai/analysis", linuxDoKeywords, log)
}

var v2exKeywords = []string{"公益 api", "免費轉發", "new-api", "one-api", "公益中轉"}
var linuxDoKeywords = []string{"公益", "免費", "new-api", "one-api", "轉發"}

// scanForumRelaySites fetches a forum page and extracts URLs that match keywords
func scanForumRelaySites(forumURL string, keywords []string, log DiscoveryLogger) []*RelaySite {
	// Use a custom User-Agent to avoid being blocked
	client := &http.Client{Timeout: 15 * time.Second}
	req, err := http.NewRequest("GET", forumURL, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FreeModelRouter/1.0; +https://github.com/freemodel/router)")

	resp, err := client.Do(req)
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

	// Search for URLs that might be new-api instances
	urlRegex := regexp.MustCompile(`https?://[a-zA-Z0-9][a-zA-Z0-9.-]*(:[0-9]+)?(/[^\s]*)?`)
	links := urlRegex.FindAllString(string(body), -1)

	// Filter links by keywords
	var relaySites []*RelaySite
	for _, link := range links {
		// Check if the surrounding text contains keywords
		// (simplified: just check if link text mentions new-api or api)
		if isRelayCandidate(link) {
			site := &RelaySite{
				BaseURL:   link,
				LastCheck: time.Now(),
			}
			if ValidateNewApiRelay(link) {
				site.Healthy = true
				site.LastSuccess = time.Now()
			}
			relaySites = append(relaySites, site)
		}
	}

	return relaySites
}

// isRelayCandidate checks if a URL looks like a new-api relay instance
func isRelayCandidate(link string) bool {
	// Skip known non-API URLs
	if strings.Contains(link, "github.com") || strings.Contains(link, "v2ex.com") || strings.Contains(link, "linux.do") {
		return false
	}
	if strings.Contains(link, "google.com") || strings.Contains(link, "bing.com") {
		return false
	}
	if strings.Contains(link, ".js") || strings.Contains(link, ".css") || strings.Contains(link, ".png") {
		return false
	}
	if strings.Contains(link, ".jpg") || strings.Contains(link, ".ico") || strings.Contains(link, ".svg") {
		return false
	}

	// Check for API-like URLs
	if strings.Contains(link, "/v1/") || strings.Contains(link, "/api/v1") || strings.Contains(link, "/api/") {
		return true
	}

	// Check for new-api/one-api related URLs
	if strings.Contains(link, "new-api") || strings.Contains(link, "one-api") || strings.Contains(link, "newapi") {
		return true
	}

	// Check for domain patterns that look like API providers
	if strings.Contains(link, ".ai/") || strings.Contains(link, "api.") || strings.Contains(link, "open.") {
		return true
	}

	return false
}

// ValidateNewApiRelay checks if a URL is a valid new-api instance
func ValidateNewApiRelay(baseURL string) bool {
	// Normalize URL
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}

	// Remove path, just use base
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return false
	}
	parsed.Path = ""
	if strings.HasSuffix(parsed.Path, "/") {
		parsed.Path = ""
	}
	baseURL = parsed.String()

	// Try /v1/models endpoint
	testURL := baseURL
	if !strings.Contains(testURL, "/v1") {
		testURL = baseURL + "/v1/models"
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return false
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false
	}

	// Check if response looks like an OpenAI models list
	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return false
	}
	if len(result.Data) == 0 {
		return false
	}

	return true
}

// DiscoverModelsFromRelay fetches models from a new-api relay instance
func DiscoverModelsFromRelay(baseURL string, providerKey string) ([]ModelEntry, error) {
	// Normalize URL
	if !strings.HasPrefix(baseURL, "http") {
		baseURL = "https://" + baseURL
	}

	testURL := baseURL
	if !strings.Contains(testURL, "/v1") {
		testURL = baseURL + "/v1/models"
	}

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("GET", testURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FreeModelRouter/1.0)")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("relay returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Data []struct {
			ID       string `json:"id"`
			OwnedByID string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	var models []ModelEntry
	for _, d := range result.Data {
		if d.ID == "" {
			continue
		}
		modelID := providerKey + "/" + d.ID
		models = append(models, ModelEntry{
			ID:      modelID,
			Label:   d.ID,
			Context: "",
		})
	}

	return models, nil
}

// DiscoverNewApiModels fetches models from a new-api instance.
// new-api implements the OpenAI-compatible /v1/models endpoint.
func DiscoverNewApiModels(baseURL string) ([]ModelEntry, error) {
	return DiscoverModelsFromRelay(baseURL, "new-api")
}
