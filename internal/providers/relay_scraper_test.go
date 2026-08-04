package providers

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateNewApiRelay(t *testing.T) {
	// Test valid new-api instance
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"id":"gpt-4o","owned_by":"openai"},{"id":"deepseek-v3","owned_by":"deepseek"}]}`))
	}))
	defer mock.Close()

	if !ValidateNewApiRelay(mock.URL) {
		t.Error("valid new-api instance should pass validation")
	}
}

func TestValidateNewApiRelayInvalid(t *testing.T) {
	// Test invalid endpoint
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer mock.Close()

	if ValidateNewApiRelay(mock.URL) {
		t.Error("invalid endpoint should fail validation")
	}
}

func TestValidateNewApiRelayBadJSON(t *testing.T) {
	// Test invalid JSON response
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`not valid json`))
	}))
	defer mock.Close()

	if ValidateNewApiRelay(mock.URL) {
		t.Error("invalid JSON should fail validation")
	}
}

func TestDiscoverModelsFromRelay(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"object": "list",
			"data": [
				{"id": "gpt-4o", "owned_by": "openai"},
				{"id": "deepseek-v3", "owned_by": "deepseek"},
				{"id": "claude-3", "owned_by": "anthropic"}
			]
		}`))
	}))
	defer mock.Close()

	models, err := DiscoverModelsFromRelay(mock.URL, "relay-test")
	if err != nil {
		t.Fatalf("DiscoverModelsFromRelay: %v", err)
	}
	if len(models) != 3 {
		t.Fatalf("expected 3 models, got %d", len(models))
	}
	if models[0].ID != "relay-test/gpt-4o" {
		t.Errorf("expected first model relay-test/gpt-4o, got %s", models[0].ID)
	}
}

func TestIsRelayCandidate(t *testing.T) {
	tests := []struct {
		url      string
		expected bool
	}{
		{"https://api.newapi.site/v1/models", true},
		{"https://new-api.example.com/v1/chat/completions", true},
		{"https://github.com/some/repo", false},
		{"https://example.com/style.css", false},
		{"https://example.com/image.png", false},
		{"https://api.openai.com/v1/models", true},
		{"https://google.com/search", false},
	}

	for _, tt := range tests {
		if got := isRelayCandidate(tt.url); got != tt.expected {
			t.Errorf("isRelayCandidate(%q) = %v, want %v", tt.url, got, tt.expected)
		}
	}
}

func TestScanV2EXRelaySites(t *testing.T) {
	// This test hits the real V2EX API - may skip if network is unavailable
	sites := scanV2EXRelaySites()
	t.Logf("Found %d relay sites from V2EX", len(sites))
	// We can't assert specific results since V2EX content changes,
	// but we can verify the function doesn't crash
}

func TestScanLinuxDoRelaySites(t *testing.T) {
	// This test hits the real linux.do API - may skip if network is unavailable
	sites := scanLinuxDoRelaySites()
	t.Logf("Found %d relay sites from linux.do", len(sites))
	// We can't assert specific results since linux.do content changes,
	// but we can verify the function doesn't crash
}
