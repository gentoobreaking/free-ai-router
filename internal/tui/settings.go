package tui

import (
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/providers"
)

// SettingsScreen manages the provider configuration view.
type SettingsScreen struct {
	cfg       *config.Config
	index     int
	keyEdit   bool
	keyBuf    string
	editFor   string
	message   string
}

func NewSettingsScreen(cfg *config.Config) *SettingsScreen {
	return &SettingsScreen{cfg: cfg}
}

func (s *SettingsScreen) SetConfig(cfg *config.Config) { s.cfg = cfg }

// HandleKey processes a key event. Returns a command and returns true if the
// settings screen should remain open, false to close it (back to table).
func (s *SettingsScreen) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	// When editing a key, intercept all input.
	if s.keyEdit {
		switch msg.String() {
		case "enter":
			if s.cfg != nil && s.editFor != "" {
				s.cfg.APIKeys[s.editFor] = s.keyBuf
				_ = config.Save(s.cfg)
			}
			s.keyEdit = false
			s.keyBuf = ""
			s.editFor = ""
		case "esc":
			s.keyEdit = false
			s.keyBuf = ""
			s.editFor = ""
		case "backspace":
			if len(s.keyBuf) > 0 {
				s.keyBuf = s.keyBuf[:len(s.keyBuf)-1]
			}
		default:
			ch := msg.String()
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 && msg.Runes[0] < 127 {
				s.keyBuf += ch
			}
		}
		return nil, true
	}

	switch msg.String() {
	case "esc", "q":
		return nil, false
	case "ctrl+c":
		return tea.Quit, false
	case "up", "k":
		if s.index > 0 {
			s.index--
		}
	case "down", "j":
		provs := s.providers()
		if s.index < len(provs)-1 {
			s.index++
		}
	case " ":
		provs := s.providers()
		if s.index >= 0 && s.index < len(provs) {
			name := provs[s.index].Name
			if s.cfg != nil {
				pcfg := s.cfg.Providers[name]
				pcfg.Enabled = !pcfg.Enabled
				s.cfg.Providers[name] = pcfg
				_ = config.Save(s.cfg)
			}
		}
	case "enter":
		provs := s.providers()
		if s.index >= 0 && s.index < len(provs) {
			p := provs[s.index]
			s.keyEdit = true
			s.editFor = p.Name
			s.keyBuf = p.Key
		}
	case "t", "T":
		provs := s.providers()
		if s.index >= 0 && s.index < len(provs) {
			name := provs[s.index].Name
			s.message = name + ": ping not available in settings (use main screen)"
		}
	case "d", "D":
		provs := s.providers()
		if s.index >= 0 && s.index < len(provs) {
			name := provs[s.index].Name
			if s.cfg != nil {
				delete(s.cfg.APIKeys, name)
				_ = config.Save(s.cfg)
			}
		}
	case "o", "O":
		provs := s.providers()
		if s.index >= 0 && s.index < len(provs) {
			name := provs[s.index].Name
			if url := providers.GetProviderSignupURL(name); url != "" {
				_ = openBrowser(url)
				s.message = "opened " + url
			} else {
				s.message = "no signup URL for " + name
			}
		}
	}
	return nil, true
}

func (s *SettingsScreen) providers() []SettingsProvider {
	seen := make(map[string]bool)
	var result []SettingsProvider

	for _, p := range providers.ProviderKeys() {
		sp := SettingsProvider{Name: p, Enabled: false}
		if s.cfg != nil {
			if pcfg, ok := s.cfg.Providers[p]; ok {
				sp.Enabled = pcfg.Enabled
			}
			if key := config.ResolveAPIKey(p, s.cfg); key != "" {
				sp.Key = key
			}
		}
		result = append(result, sp)
		seen[p] = true
	}

	if s.cfg != nil {
		for name := range s.cfg.Providers {
			if seen[name] {
				continue
			}
			sp := SettingsProvider{Name: name, Enabled: s.cfg.Providers[name].Enabled}
			if key := config.ResolveAPIKey(name, s.cfg); key != "" {
				sp.Key = key
			}
			result = append(result, sp)
		}
	}
	return result
}

func (s *SettingsScreen) View() string {
	return RenderSettings(s.providers(), s.index, s.keyEdit, s.keyBuf, s.message)
}

// --- Ping helper (same as original, used from settings) ---

func pingModelNowTUI(m *models.Model) (string, int, time.Duration) {
	if m.Endpoint == "" {
		return "down", 0, 0
	}
	start := time.Now()
	body := `{"model":"` + m.UpstreamModelID + `","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`
	req, err := http.NewRequest(http.MethodPost, m.Endpoint, strings.NewReader(body))
	if err != nil {
		return "down", 0, 0
	}
	req.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "down", 0, time.Since(start)
	}
	defer resp.Body.Close()
	_, _ = io.ReadAll(resp.Body)
	return ping.StatusFromCode(resp.StatusCode), resp.StatusCode, time.Since(start)
}
