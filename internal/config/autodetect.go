package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// KeySources maps a provider name to its candidate env var names and optional
// secondary detection paths. Used by AutoDetectKeys to discover keys from
// shell RC files and agent configs when os.Getenv returns empty.
type KeySource struct {
	Provider    string
	EnvVarNames []string
}

// keySources is the master list of provider → env var mappings for
// auto-detection. Order matters: first match wins per provider.
var keySources = []KeySource{
	{Provider: "nvidia", EnvVarNames: []string{"NVIDIA_API_KEY", "NVAPI_KEY"}},
	{Provider: "groq", EnvVarNames: []string{"GROQ_API_KEY"}},
	{Provider: "cerebras", EnvVarNames: []string{"CEREBRAS_API_KEY"}},
	{Provider: "openrouter", EnvVarNames: []string{"OPENROUTER_API_KEY"}},
	{Provider: "opencode", EnvVarNames: []string{"OPENCODE_API_KEY", "ZEN_OPENCODE_API_KEY"}},
	{Provider: "openai-compatible", EnvVarNames: []string{"OPENAI_COMPATIBLE_API_KEY"}},
	{Provider: "ollama", EnvVarNames: []string{"OLLAMA_API_KEY"}},
	{Provider: "codestral", EnvVarNames: []string{"CODESTRAL_API_KEY"}},
	{Provider: "scaleway", EnvVarNames: []string{"SCALEWAY_API_KEY"}},
	{Provider: "kilocode", EnvVarNames: []string{"KILOCODE_API_KEY"}},
	{Provider: "googleai", EnvVarNames: []string{"GOOGLE_API_KEY", "GEMINI_API_KEY"}},
	{Provider: "deepseek", EnvVarNames: []string{"DEEPSEEK_API_KEY"}},
	{Provider: "anthropic", EnvVarNames: []string{"ANTHROPIC_API_KEY"}},
	{Provider: "new-api", EnvVarNames: []string{"NEW_API_API_KEY"}},
	{Provider: "siliconflow", EnvVarNames: []string{"SILICONFLOW_API_KEY"}},
	{Provider: "baidu", EnvVarNames: []string{"QIANFAN_API_KEY"}},
	{Provider: "alibabacloud", EnvVarNames: []string{"DASHSCOPE_API_KEY"}},
	{Provider: "tencent", EnvVarNames: []string{"TENCENT_CLOUD_API_KEY"}},
	{Provider: "kuaipao", EnvVarNames: []string{"KUAIPAO_API_KEY"}},
}

// shellRCFiles lists the shell init files to scan for exported variables.
var shellRCFiles = []string{
	"~/.bash_extend",
	"~/.bashrc",
	"~/.bash_profile",
	"~/.zshrc",
	"~/.zprofile",
	"~/.profile",
}

// agentConfigPaths lists agent config files to scan for embedded API keys.
var agentConfigPaths = []string{
	"~/.config/opencode/opencode.json",
	"~/.config/openclaw/config.yaml",
	"~/.config/hermes/config.yaml",
	"~/.config/pi/pi.json",
}

// shellExportRe matches `export VAR=value` or `export VAR="value"`.
var shellExportRe = regexp.MustCompile(`^\s*export\s+(\w+)=["']?(.+?)["']?\s*$`)

// ---------- Public API ----------

// AutoDetectKeys discovers API keys from shell RC files and agent configs.
// Returns a map of provider → key. Does not check os.Getenv — those are
// handled earlier in the ResolveAPIKey priority chain.
func AutoDetectKeys() map[string]string {
	result := make(map[string]string)

	// Layer 1: Shell RC files (higher priority than agent configs)
	shellVars := ParseShellRCs()
	for _, src := range keySources {
		for _, envName := range src.EnvVarNames {
			if val, ok := shellVars[envName]; ok && val != "" {
				result[src.Provider] = val
				break
			}
		}
	}

	// Layer 2: Agent configs (lower priority — don't override shell)
	agentKeys := ParseAgentConfigs()
	for provider, key := range agentKeys {
		if _, exists := result[provider]; !exists {
			result[provider] = key
		}
	}

	return result
}

// ParseShellRCs scans all known shell RC files for `export VAR=value` lines
// and returns a flat map of env var name → value.
func ParseShellRCs() map[string]string {
	result := make(map[string]string)

	for _, f := range shellRCFiles {
		path := expandHome(f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			matches := shellExportRe.FindStringSubmatch(line)
			if len(matches) == 3 {
				name := strings.TrimSpace(matches[1])
				val := strings.TrimSpace(matches[2])
				// Strip trailing newline / carriage return
				val = strings.TrimRight(val, "\r")
				if name != "" && val != "" {
					result[name] = val
				}
			}
		}
	}

	return result
}

// ParseAgentConfigs scans installed agent config files for embedded API
// keys. Returns a map of provider → key. Best-effort: missing or malformed
// files are silently skipped.
func ParseAgentConfigs() map[string]string {
	result := make(map[string]string)

	for _, ap := range agentConfigPaths {
		path := expandHome(ap)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}

		switch {
		case strings.Contains(ap, "opencode"):
			for k, v := range parseOpenCodeConfig(data) {
				if _, exists := result[k]; !exists {
					result[k] = v
				}
			}
		case strings.Contains(ap, "openclaw"):
			for k, v := range parseOpenClawConfig(data) {
				if _, exists := result[k]; !exists {
					result[k] = v
				}
			}
		case strings.Contains(ap, "pi"):
			for k, v := range parsePiConfig(data) {
				if _, exists := result[k]; !exists {
					result[k] = v
				}
			}
		}
	}

	return result
}

// ---------- Agent-specific parsers ----------

// parseOpenCodeConfig recursively walks the OpenCode JSON config looking for
// objects that contain apiKey + baseURL fields, mapping baseURL → provider.
func parseOpenCodeConfig(data []byte) map[string]string {
	var root interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	result := make(map[string]string)
	walkJSON(root, result)
	return result
}

// walkJSON recursively descends a JSON value looking for objects with
// "apiKey" and optionally "baseURL" / "provider" fields.
func walkJSON(v interface{}, out map[string]string) {
	switch val := v.(type) {
	case map[string]interface{}:
		// Look for direct apiKey entry
		if key, ok := val["apiKey"]; ok {
			if keyStr, ok := key.(string); ok && keyStr != "" && !strings.HasPrefix(keyStr, "$") && keyStr != "__QCLAW_AUTH_GATEWAY_MANAGED__" {
				provider := guessProviderFromObject(val)
				if provider != "" {
					if _, exists := out[provider]; !exists {
						out[provider] = keyStr
					}
				}
			}
		}
		// Recurse into children
		for _, child := range val {
			walkJSON(child, out)
		}
	case []interface{}:
		for _, item := range val {
			walkJSON(item, out)
		}
	}
}

// parseOpenClawConfig scans an OpenClaw YAML-style config for provider apiKeys.
func parseOpenClawConfig(data []byte) map[string]string {
	result := make(map[string]string)
	content := string(data)

	// Simple key-value scanner: look for "apiKey:" lines near provider names
	lines := strings.Split(content, "\n")
	var currentProvider string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		// Detect provider block: "- provider: xxx" or "provider:" 
		for provider := range providerToBaseURLOffsets() {
			if strings.Contains(trimmed, fmt.Sprintf("\"%s\"", provider)) ||
				strings.Contains(trimmed, fmt.Sprintf("'%s'", provider)) ||
				strings.Contains(trimmed, fmt.Sprintf("provider: %s", provider)) {
				currentProvider = provider
			}
		}
		// Detect apiKey line
		if strings.Contains(trimmed, "apiKey:") || strings.Contains(trimmed, "api_key:") {
			parts := strings.SplitN(trimmed, ":", 2)
			if len(parts) == 2 && currentProvider != "" {
				key := strings.TrimSpace(parts[1])
				key = strings.Trim(key, "\"'")
				if key != "" && !strings.HasPrefix(key, "$") {
					if _, exists := result[currentProvider]; !exists {
						result[currentProvider] = key
					}
				}
			}
		}
	}
	return result
}

// parsePiConfig scans a Pi JSON config for apiKey entries.
func parsePiConfig(data []byte) map[string]string {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil
	}
	result := make(map[string]string)
	walkJSON(root, result)
	return result
}

// guessProviderFromObject tries to identify the provider from an object that
// already contains an apiKey (found by walkJSON). Checks baseURL, provider
// field, and model field for clues.
func guessProviderFromObject(obj map[string]interface{}) string {
	// Check explicit provider field
	if p, ok := obj["provider"].(string); ok && p != "" {
		return p
	}

	// Check baseURL for known patterns
	if baseURL, ok := obj["baseURL"].(string); ok {
		return guessProviderFromURL(baseURL)
	}
	if baseURL, ok := obj["baseUrl"].(string); ok {
		return guessProviderFromURL(baseURL)
	}

	return ""
}

// guessProviderFromURL matches a base URL against known provider signatures.
func guessProviderFromURL(url string) string {
	lower := strings.ToLower(url)
	for provider, fragments := range providerToBaseURLOffsets() {
		for _, frag := range fragments {
			if strings.Contains(lower, frag) {
				return provider
			}
		}
	}
	return ""
}

// providerToBaseURLOffsets returns known URL fragments that identify each
// provider. Used to map agent config baseURLs back to our provider keys.
func providerToBaseURLOffsets() map[string][]string {
	return map[string][]string{
		"nvidia":       {"integrate.api.nvidia.com", "ai.api.nvidia.com", "nvidia"},
		"groq":         {"api.groq.com", "groq"},
		"cerebras":     {"api.cerebras.ai", "cerebras"},
		"openrouter":   {"openrouter.ai"},
		"opencode":     {"api.opencode.ai", "opencode"},
		"googleai":     {"generativelanguage.googleapis.com", "googleapis.com/generative", "google"},
		"codestral":    {"api.mistral.ai", "codestral", "mistral"},
		"scaleway":     {"api.scaleway.ai", "scaleway"},
		"deepseek":     {"api.deepseek.com", "deepseek"},
		"anthropic":    {"api.anthropic.com", "anthropic"},
		"siliconflow":  {"api.siliconflow.cn", "siliconflow"},
		"alibabacloud": {"dashscope.aliyuncs.com", "dashscope"},
		"baidu":        {"qianfan.baidubce.com", "qianfan"},
		"tencent":      {"hunyuan.cloud.tencent.com", "hunyuan.tencentcloudapi.com"},
		"ollama":       {"localhost:11434", "127.0.0.1:11434", "ollama"},
		"openai-compatible": {"openai"},
	}
}

// ---------- Helpers ----------

// expandHome replaces leading ~/ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}
