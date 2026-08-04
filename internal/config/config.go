package config

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ConfigFileName = ".freemodel-router.json"
const LegacyConfigFileName = ".free-router.json"
const EnvConfigPathVar = "FREMODEL_CONFIG_PATH"

type Config struct {
	APIKeys           map[string]interface{}    `json:"apiKeys"`
	Providers         map[string]ProviderConfig `json:"providers"`
	BannedModels      []string                  `json:"bannedModels"`
	AutoUpdate        AutoUpdateConfig          `json:"autoUpdate"`
	MinSweScore       *float64                  `json:"minSweScore"`
	ExcludedProviders []string                  `json:"excludedProviders"`
	PinningMode       string                    `json:"pinningMode"`
	ModelTags         map[string][]string       `json:"modelTags"`
	AutoPingEnabled   bool                      `json:"autoPingEnabled"`
	CodingOnly        bool                      `json:"codingOnly"`
	UI                UIConfig                  `json:"ui"`
}

type ProviderConfig struct {
	Enabled        bool   `json:"enabled"`
	Name           string `json:"name,omitempty"`
	BaseURL        string `json:"baseUrl,omitempty"`
	ModelID        string `json:"modelId,omitempty"`
	DiscoverModels bool   `json:"discoverModels,omitempty"`
	MaxTurns       int    `json:"maxTurns,omitempty"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	AuthMode       string `json:"authMode,omitempty"`
}

type AutoUpdateConfig struct {
	Enabled            bool    `json:"enabled"`
	IntervalHours      int     `json:"intervalHours"`
	LastCheckAt        string  `json:"lastCheckAt"`
	LastUpdateAt       *string `json:"lastUpdateAt"`
	LastVersionApplied *string `json:"lastVersionApplied"`
	LastError          *string `json:"lastError"`
}

type UIConfig struct {
	ScrollSortPauseMs int `json:"scrollSortPauseMs"`
}

type EnvOverride struct {
	Provider string
	EnvVar   string
}

var EnvOverrides = []EnvOverride{
	{Provider: "nvidia", EnvVar: "NVIDIA_API_KEY"},
	{Provider: "groq", EnvVar: "GROQ_API_KEY"},
	{Provider: "cerebras", EnvVar: "CEREBRAS_API_KEY"},
	{Provider: "opencode", EnvVar: "OPENCODE_API_KEY"},
	{Provider: "openrouter", EnvVar: "OPENROUTER_API_KEY"},
	{Provider: "openai-compatible", EnvVar: "OPENAI_COMPATIBLE_API_KEY"},
	{Provider: "ollama", EnvVar: "OLLAMA_API_KEY"},
	{Provider: "codestral", EnvVar: "CODESTRAL_API_KEY"},
	{Provider: "scaleway", EnvVar: "SCALEWAY_API_KEY"},
	{Provider: "kilocode", EnvVar: "KILOCODE_API_KEY"},
	{Provider: "googleai", EnvVar: "GOOGLE_API_KEY"},
}

func DefaultConfig() *Config {
	return &Config{
		APIKeys:           make(map[string]interface{}),
		Providers:         make(map[string]ProviderConfig),
		BannedModels:      []string{},
		AutoUpdate:        AutoUpdateConfig{Enabled: true, IntervalHours: 24},
		ExcludedProviders: []string{},
		PinningMode:       "canonical",
		ModelTags:         make(map[string][]string),
		AutoPingEnabled:   true,
		CodingOnly:        true,
		UI:                UIConfig{ScrollSortPauseMs: 1500},
	}
}

// ConfigPath resolves the config file location:
//  1. $FREMODEL_CONFIG_PATH if set (spec §18)
//  2. ~/.freemodel-router.json (spec §9.1)
func ConfigPath() (string, error) {
	if env := os.Getenv(EnvConfigPathVar); env != "" {
		return env, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, ConfigFileName), nil
}

func Load() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			legacyPath, legacyErr := legacyConfigPath()
			if legacyErr == nil {
				legacyData, legacyErr := os.ReadFile(legacyPath)
				if legacyErr == nil {
					var cfg Config
					if jsonErr := json.Unmarshal(legacyData, &cfg); jsonErr == nil {
						migrated := migrateLegacy(&cfg)
						// Persist the migrated config at the new location
						// so future loads skip legacy lookup (spec §22.2).
						if saveErr := Save(migrated); saveErr == nil {
							_ = os.Rename(legacyPath, legacyPath+".migrated")
						}
						return migrated, nil
					}
				}
			}
			return DefaultConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		backupPath := path + ".corrupt-" + time.Now().Format("20060102-150405")
		_ = os.Rename(path, backupPath)
		return DefaultConfig(), nil
	}

	return normalizeConfig(&cfg), nil
}

// GetPort resolves the router port: $FREMODEL_PORT overrides the default
// (spec §18 env vars).
func GetPort(defaultPort int) int {
	if env := os.Getenv("FREMODEL_PORT"); env != "" {
		var p int
		if _, err := fmt.Sscanf(env, "%d", &p); err == nil && p > 0 {
			return p
		}
	}
	return defaultPort
}

func Save(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	return saveTo(path, cfg)
}

func saveTo(path string, cfg *Config) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return err
	}

	return os.Rename(tmpPath, path)
}

func ExportToken(cfg *Config) (string, error) {
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}
	return "mrconf:v1:" + base64URLEncode(data), nil
}

func ImportToken(token string) (*Config, error) {
	const prefix = "mrconf:v1:"
	if !strings.HasPrefix(token, prefix) {
		return nil, errors.New("invalid config token format")
	}
	payload := token[len(prefix):]
	data, err := base64URLEncodeDecode(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to decode config token: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config token: %w", err)
	}
	return normalizeConfig(&cfg), nil
}

func ResolveAPIKey(provider string, cfg *Config) string {
	for _, env := range EnvOverrides {
		if env.Provider == provider {
			if val := os.Getenv(env.EnvVar); val != "" {
				return val
			}
		}
	}

	keys, ok := cfg.APIKeys[provider]
	if !ok {
		return ""
	}

	switch v := keys.(type) {
	case string:
		return v
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}

	return ""
}

func ResolveAPIKeys(provider string, cfg *Config) []string {
	for _, env := range EnvOverrides {
		if env.Provider == provider {
			if val := os.Getenv(env.EnvVar); val != "" {
				return []string{val}
			}
		}
	}

	keys, ok := cfg.APIKeys[provider]
	if !ok {
		return nil
	}

	switch v := keys.(type) {
	case string:
		return []string{v}
	case []interface{}:
		result := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				result = append(result, s)
			}
		}
		return result
	}

	return nil
}

func normalizeConfig(cfg *Config) *Config {
	if cfg.APIKeys == nil {
		cfg.APIKeys = make(map[string]interface{})
	}
	if cfg.Providers == nil {
		cfg.Providers = make(map[string]ProviderConfig)
	}
	if cfg.BannedModels == nil {
		cfg.BannedModels = []string{}
	}
	if cfg.ExcludedProviders == nil {
		cfg.ExcludedProviders = []string{}
	}
	if cfg.ModelTags == nil {
		cfg.ModelTags = make(map[string][]string)
	}
	if cfg.PinningMode == "" {
		cfg.PinningMode = "canonical"
	}
	if cfg.UI.ScrollSortPauseMs == 0 {
		cfg.UI.ScrollSortPauseMs = 1500
	}
	if cfg.AutoUpdate.IntervalHours == 0 {
		cfg.AutoUpdate.IntervalHours = 24
	}
	return cfg
}

func migrateLegacy(cfg *Config) *Config {
	newCfg := DefaultConfig()

	if cfg.APIKeys != nil {
		newCfg.APIKeys = cfg.APIKeys
	}
	if cfg.Providers != nil {
		newCfg.Providers = cfg.Providers
	}
	if cfg.BannedModels != nil {
		newCfg.BannedModels = cfg.BannedModels
	}

	for provider, pcfg := range newCfg.Providers {
		if pcfg.Enabled {
			continue
		}
		if _, ok := cfg.APIKeys[provider]; ok {
			pcfg.Enabled = true
			newCfg.Providers[provider] = pcfg
		}
	}

	return normalizeConfig(newCfg)
}

func legacyConfigPath() (string, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(homeDir, LegacyConfigFileName), nil
}

func base64URLEncode(data []byte) string {
	return base64.StdEncoding.WithPadding(base64.NoPadding).EncodeToString(data)
}

func base64URLEncodeDecode(s string) ([]byte, error) {
	var padded string
	switch len(s) % 4 {
	case 2:
		padded = s + "=="
	case 3:
		padded = s + "="
	default:
		padded = s
	}
	return base64.StdEncoding.DecodeString(padded)
}
