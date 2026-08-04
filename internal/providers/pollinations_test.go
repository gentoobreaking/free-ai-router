package providers

import (
	"strings"
	"testing"
)

func TestIsPollinationsModel(t *testing.T) {
	tests := []struct {
		id      string
		want    bool
	}{
		{"pollinations/openai", true},
		{"pollinations/deepseek", true},
		{"nvidia/llama3-70b", false},
		{"openrouter/auto", false},
	}

	for _, tt := range tests {
		if got := IsPollinationsModel(tt.id); got != tt.want {
			t.Errorf("IsPollinationsModel(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestMapPollinationsModel(t *testing.T) {
	tests := []struct {
		id    string
		want  string
	}{
		{"pollinations/openai", "gpt-4o"},
		{"pollinations/deepseek", "deepseek-v3"},
		{"pollinations/gemini-3-flash", "gemini-3-flash"},
		{"pollinations/unknown-model", "unknown-model"},
	}

	for _, tt := range tests {
		if got := MapPollinationsModel(tt.id); got != tt.want {
			t.Errorf("MapPollinationsModel(%q) = %q, want %q", tt.id, got, tt.want)
		}
	}
}

func TestBuildPollinationsTextURL(t *testing.T) {
	url := BuildPollinationsTextURL("Hello", "pollinations/deepseek")
	if url != "https://text.pollinations.ai/Hello?model=deepseek-v3" {
		t.Errorf("BuildPollinationsTextURL() = %q, want https://text.pollinations.ai/Hello?model=deepseek-v3", url)
	}
}

func TestConvertOpenAIToPollinations(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"Hello world"}]}`)
	prompt, err := ConvertOpenAIToPollinations(body, "pollinations/openai")
	if err != nil {
		t.Fatalf("ConvertOpenAIToPollinations: %v", err)
	}
	if prompt != "Hello world" {
		t.Errorf("prompt = %q, want \"Hello world\"", prompt)
	}
}

func TestConvertOpenAIToPollinationsMultipleMessages(t *testing.T) {
	body := []byte(`{"model":"gpt-4o","messages":[{"role":"system","content":"You are helpful"},{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi there"},{"role":"user","content":"How are you?"}]}`)
	prompt, err := ConvertOpenAIToPollinations(body, "pollinations/openai")
	if err != nil {
		t.Fatalf("ConvertOpenAIToPollinations: %v", err)
	}
	// Should only include user messages
	if prompt != "Hello\nHow are you?" {
		t.Errorf("prompt = %q, want \"Hello\\nHow are you?\"", prompt)
	}
}

func TestWrapPollinationsResponse(t *testing.T) {
	result := WrapPollinationsResponse("Hello, I'm doing well!")
	if string(result) == "" {
		t.Error("WrapPollinationsResponse should return non-empty JSON")
	}
	// Verify it contains expected fields
	s := string(result)
	if !strings.Contains(s, "chatcmpl-pollinations") {
		t.Error("response should contain id")
	}
	if !strings.Contains(s, "Hello, I'm doing well!") {
		t.Error("response should contain the text content")
	}
}
