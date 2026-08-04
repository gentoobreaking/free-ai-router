package providers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// PollinationsModelMap maps internal model names to Pollinations API model names
var PollinationsModelMap = map[string]string{
	"pollinations/openai":             "gpt-4o",
	"pollinations/openai-fast":        "gpt-4o",
	"pollinations/gpt-oss":            "gpt-oss",
	"pollinations/deepseek":           "deepseek-v3",
	"pollinations/deepseek-pro":       "deepseek-v3",
	"pollinations/gemini":             "gemini",
	"pollinations/gemini-3-flash":     "gemini-3-flash",
	"pollinations/gemini-flash-lite-3.5": "gemini-2.5-flash",
	"pollinations/gemini-fast":        "gemini-2.0-flash",
	"pollinations/mistral":            "mistral",
	"pollinations/mistral-small-3.2":  "mistral-small-3.2",
	"pollinations/qwen-coder":         "qwen-coder",
	"pollinations/kimi-k3":            "kimi-k3",
	"pollinations/claude":             "claude",
	"pollinations/claude-fast":        "claude-fast",
	"pollinations/claude-sonnet-5":    "claude-sonnet-5",
	"pollinations/command-a-plus":     "command-a-plus",
	"pollinations/mercury":            "mercury",
}

// PollinationsTextEndpoint is the base URL for the /text/{prompt} endpoint
const PollinationsTextEndpoint = "https://text.pollinations.ai"

// IsPollinationsModel returns true if the model ID belongs to a Pollinations provider
func IsPollinationsModel(modelID string) bool {
	return strings.HasPrefix(modelID, "pollinations/")
}

// MapPollinationsModel maps an internal model ID to the Pollinations API model name
func MapPollinationsModel(modelID string) string {
	if mapped, ok := PollinationsModelMap[modelID]; ok {
		return mapped
	}
	// Strip the "pollinations/" prefix
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) == 2 {
		return parts[1]
	}
	return modelID
}

// BuildPollinationsTextURL builds a /text/{prompt} URL for the given model
func BuildPollinationsTextURL(prompt, modelID string) string {
	mappedModel := MapPollinationsModel(modelID)
	encodedPrompt := url.PathEscape(prompt)
	params := url.Values{}
	params.Set("model", mappedModel)
	return fmt.Sprintf("%s/%s?%s", PollinationsTextEndpoint, encodedPrompt, params.Encode())
}

// PollinationsTextRequest represents a request to the /text endpoint
type PollinationsTextRequest struct {
	Prompt     string `json:"prompt"`
	Model      string `json:"model"`
	MaxTokens  int    `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
}

// PollinationsTextResponse represents a response from the /text endpoint
// The actual response is plain text, but we wrap it for OpenAI compatibility
type PollinationsTextResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
}

// ConvertOpenAIToPollinations converts an OpenAI chat/completions request body
// to a Pollinations /text/{prompt} request
func ConvertOpenAIToPollinations(body []byte, modelID string) (string, error) {
	var req struct {
		Model       string `json:"model"`
		Messages    []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
		MaxTokens   *int    `json:"max_tokens"`
		Temperature *float64 `json:"temperature"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", err
	}

	// Build prompt from messages
	var prompt strings.Builder
	for _, msg := range req.Messages {
		if msg.Role == "user" {
			if prompt.Len() > 0 {
				prompt.WriteString("\n")
			}
			prompt.WriteString(msg.Content)
		}
	}
	if prompt.Len() == 0 {
		return "", fmt.Errorf("no user message found in request")
	}

	return prompt.String(), nil
}

// WrapPollinationsResponse converts plain text from /text endpoint to OpenAI-compatible JSON
func WrapPollinationsResponse(text string) []byte {
	resp := PollinationsTextResponse{
		ID:      "chatcmpl-pollinations",
		Object:  "chat.completion",
		Created: time.Now().Unix(),
	}
	resp.Choices = make([]struct {
		Index        int `json:"index"`
		Message      struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	}, 1)
	resp.Choices[0].Index = 0
	resp.Choices[0].Message.Role = "assistant"
	resp.Choices[0].Message.Content = text
	resp.Choices[0].FinishReason = "stop"

	data, _ := json.Marshal(resp)
	return data
}

// PingPollinationsText pings the Pollinations /text endpoint to check availability
// Returns 200 if the model is accessible without auth
func PingPollinationsText(modelID string) (int, error) {
	mappedModel := MapPollinationsModel(modelID)
	testURL := fmt.Sprintf("%s/hello?model=%s", PollinationsTextEndpoint, url.QueryEscape(mappedModel))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, nil
}
