package providers

import (
	"encoding/json"
	"testing"
)

// TestPollinationsAdapterComposability verifies that all adapter functions
// compose correctly end-to-end: OpenAI request → /text prompt → response → OpenAI JSON.
func TestPollinationsAdapterComposability(t *testing.T) {
	// Step 1 — Convert OpenAI request to /text prompt
	openaiBody := []byte(`{"model":"gpt-4o","messages":[{"role":"user","content":"What is the capital of France?"}]}`)

	prompt, err := ConvertOpenAIToPollinations(openaiBody, "pollinations/openai")
	if err != nil {
		t.Fatalf("ConvertOpenAIToPollinations: %v", err)
	}
	if prompt != "What is the capital of France?" {
		t.Errorf("prompt = %q, want \"What is the capital of France?\"", prompt)
	}

	// Step 2 — Build /text URL (no actual HTTP call)
	url := BuildPollinationsTextURL(prompt, "pollinations/openai")
	expectedPrefix := "https://text.pollinations.ai/What%20is%20the%20capital%20of%20France%3F?model=gpt-4o"
	if url != expectedPrefix {
		t.Errorf("BuildPollinationsTextURL = %q, want %q", url, expectedPrefix)
	}

	// Step 3 — Wrap plain-text response as OpenAI-compatible JSON
	rawResponse := "The capital of France is Paris."
	wrapped := WrapPollinationsResponse(rawResponse)

	// Step 4 — Verify the wrapped JSON can be parsed as a valid completion
	var completion struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Choices []struct {
			Index   int `json:"index"`
			Message struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(wrapped, &completion); err != nil {
		t.Fatalf("WrapPollinationsResponse produced invalid JSON: %v", err)
	}
	if completion.Object != "chat.completion" {
		t.Errorf("object = %q, want \"chat.completion\"", completion.Object)
	}
	if len(completion.Choices) != 1 {
		t.Fatalf("choices len = %d, want 1", len(completion.Choices))
	}
	if completion.Choices[0].Message.Content != rawResponse {
		t.Errorf("content = %q, want %q", completion.Choices[0].Message.Content, rawResponse)
	}
	if completion.Choices[0].Message.Role != "assistant" {
		t.Errorf("role = %q, want \"assistant\"", completion.Choices[0].Message.Role)
	}
	if completion.Choices[0].FinishReason != "stop" {
		t.Errorf("finish_reason = %q, want \"stop\"", completion.Choices[0].FinishReason)
	}
}

// TestPollinationsAdapterAllModels verifies that MapPollinationsModel covers
// every model in the static list and BuildPollinationsTextURL produces valid URLs.
func TestPollinationsAdapterAllModels(t *testing.T) {
	for internalID := range PollinationsModelMap {
		// Every internal model must map to a non-empty API model
		mapped := MapPollinationsModel(internalID)
		if mapped == "" {
			t.Errorf("MapPollinationsModel(%q) returned empty", internalID)
			continue
		}

		// BuildPollinationsTextURL must produce a valid URL for every model
		url := BuildPollinationsTextURL("hello", internalID)
		if url == "" {
			t.Errorf("BuildPollinationsTextURL(%q) returned empty", internalID)
			continue
		}
	}
}

// TestBuildPollinationsTextURLSpecialChars verifies proper URL encoding.
func TestBuildPollinationsTextURLSpecialChars(t *testing.T) {
	tests := []struct {
		prompt   string
		modelID  string
		contains string
	}{
		{"Hello world", "pollinations/openai", "Hello%20world"},
		{"What is 2+2?", "pollinations/deepseek", "What+is+2%2B2%3F"},
		{"Line1\nLine2", "pollinations/gemini", "Line1%0ALine2"},
	}

	for _, tt := range tests {
		url := BuildPollinationsTextURL(tt.prompt, tt.modelID)
		if url == "" {
			t.Errorf("BuildPollinationsTextURL(%q, %q) returned empty", tt.prompt, tt.modelID)
		}
		// URL should start with the text endpoint prefix
		if url[:len(PollinationsTextEndpoint)] != PollinationsTextEndpoint {
			t.Errorf("URL %q does not start with %q", url, PollinationsTextEndpoint)
		}
	}
}

// TestConvertOpenAIToPollinationsEdgeCases covers edge cases.
func TestConvertOpenAIToPollinationsEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantErr   bool
		wantEmpty bool
	}{
		{
			name:    "empty messages",
			body:    `{"model":"gpt-4o","messages":[]}`,
			wantErr: true,
		},
		{
			name: "system and assistant only (no user)",
			body: `{"model":"gpt-4o","messages":[{"role":"system","content":"You are helpful"},{"role":"assistant","content":"I am ready"}]}`,
			wantErr: true,
		},
		{
			name:    "invalid JSON",
			body:    `{not json`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt, err := ConvertOpenAIToPollinations([]byte(tt.body), "pollinations/openai")
			if tt.wantErr && err == nil {
				t.Errorf("expected error, got prompt=%q", prompt)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
