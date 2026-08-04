package tui

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/freemodel/router/internal/config"
)

// WizardStep enumerates the wizard flow states.
type WizardStep int

const (
	WizWelcome WizardStep = iota
	WizProviders
	WizKeyEntry
	WizDone
)

// WizardProvider holds per-provider onboarding metadata (mirrors cli/onboard.go).
type WizardProvider struct {
	Name     string
	Prefix   string
	SignupURL string
}

var wizardProviders = []WizardProvider{
	{"nvidia", "nvapi-", "https://build.nvidia.com/"},
	{"groq", "gsk_", "https://console.groq.com/keys"},
	{"cerebras", "cerebras", "https://cloud.cerebras.ai/"},
	{"openrouter", "sk-or-", "https://openrouter.ai/keys"},
	{"googleai", "AIza", "https://aistudio.google.com/apikey"},
	{"opencode", "oc-", "https://opencode.ai/"},
	{"codestral", "", "https://codestral.mistral.ai/"},
	{"scaleway", "", "https://console.scaleway.com/"},
	{"kilocode", "", "https://kilocode.ai/"},
	{"ollama", "", "https://ollama.com/"},
}

// isFirstRun returns true when no config file exists or no API keys are configured.
func isFirstRun(cfg *config.Config) bool {
	if cfg == nil {
		return true
	}
	if cfg.APIKeys == nil || len(cfg.APIKeys) == 0 {
		return true
	}
	return false
}

// WizardModel is a lightweight Bubble Tea model for the first-run wizard.
// It transitions to the main Model once the user completes or skips the flow.
type WizardModel struct {
	cfg       *config.Config
	step      WizardStep
	idx       int          // current provider index (WizProviders / WizKeyEntry)
	keyBuf    string       // key entry buffer
	msg       string       // status / warning message
	collected map[string]string // provider → key, collected during wizard
	quit      bool
}

func NewWizardModel(cfg *config.Config) *WizardModel {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	return &WizardModel{
		cfg:       cfg,
		step:      WizWelcome,
		collected: make(map[string]string),
	}
}

func (w *WizardModel) Init() tea.Cmd {
	return nil
}

func (w *WizardModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if w.quit {
		// User quit during wizard; save whatever keys were collected
		w.saveKeys()
		return w.mainModel(), tea.Quit
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return w.handleKey(msg)
	case tea.WindowSizeMsg:
		// ignore resize during wizard
		return w, nil
	default:
		return w, nil
	}
}

func (w *WizardModel) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	// Global: Ctrl+C quits
	if key == "ctrl+c" {
		w.quit = true
		return w, tea.Quit
	}

	switch w.step {
	case WizWelcome:
		if key == "enter" {
			w.step = WizProviders
			w.idx = 0
			return w, nil
		}
		if key == "s" || key == "S" {
			// Skip wizard entirely
			w.step = WizDone
			return w, nil
		}
		if key == "q" || key == "esc" {
			w.quit = true
			return w, tea.Quit
		}
		return w, nil

	case WizProviders:
		return w.handleProvidersInput(key)

	case WizKeyEntry:
		return w.handleKeyEntryInput(key)

	case WizDone:
		w.saveKeys()
		return w.mainModel(), nil
	}
	return w, nil
}

func (w *WizardModel) handleProvidersInput(key string) (tea.Model, tea.Cmd) {
	if w.idx >= len(wizardProviders) {
		w.step = WizDone
		return w, nil
	}

	p := wizardProviders[w.idx]

	switch key {
	case "o", "O":
		// Open browser + enter key
		_ = openBrowser(p.SignupURL)
		w.msg = fmt.Sprintf("Opening %s in browser...", p.SignupURL)
		w.step = WizKeyEntry
		w.keyBuf = ""
		return w, nil

	case "e", "E":
		// Enter key manually
		w.msg = ""
		w.step = WizKeyEntry
		w.keyBuf = ""
		return w, nil

	case "s", "S":
		// Skip this provider
		w.idx++
		w.msg = "Skipped."
		return w, nil

	case "q", "esc":
		// Quit wizard early, go to next step (done) so keys collected so far are saved
		w.step = WizDone
		return w, nil

	case "up", "k":
		if w.idx > 0 {
			w.idx--
		}
		return w, nil

	case "down", "j":
		if w.idx < len(wizardProviders)-1 {
			w.idx++
		}
		return w, nil
	}
	return w, nil
}

func (w *WizardModel) handleKeyEntryInput(key string) (tea.Model, tea.Cmd) {
	if w.idx >= len(wizardProviders) {
		w.step = WizDone
		return w, nil
	}

	p := wizardProviders[w.idx]

	switch key {
	case "enter":
		trimmed := strings.TrimSpace(w.keyBuf)
		if trimmed != "" {
			if p.Prefix != "" && !strings.HasPrefix(trimmed, p.Prefix) {
				w.msg = fmt.Sprintf("Warning: key does not start with expected prefix %q. Press Enter again to save anyway.", p.Prefix)
				w.keyBuf = trimmed
				return w, nil
			}
			w.collected[p.Name] = trimmed
			// Enable the provider in config
			pcfg, ok := w.cfg.Providers[p.Name]
			if !ok {
				pcfg = config.ProviderConfig{}
			}
			pcfg.Enabled = true
			w.cfg.Providers[p.Name] = pcfg
			w.msg = fmt.Sprintf("Key saved for %s.", p.Name)
		} else {
			w.msg = "Skipped (no key entered)."
		}
		w.idx++
		w.step = WizProviders
		return w, nil

	case "esc":
		// Cancel key entry, go back to provider selection
		w.step = WizProviders
		w.msg = ""
		return w, nil

	case "ctrl+c":
		w.quit = true
		w.step = WizDone
		return w, tea.Quit

	case "backspace":
		if len(w.keyBuf) > 0 {
			w.keyBuf = w.keyBuf[:len(w.keyBuf)-1]
		}
		return w, nil

	default:
		// Only accept printable characters for key entry
		if len(key) == 1 && key[0] >= 32 && key[0] < 127 {
			w.keyBuf += key
		}
		return w, nil
	}
}

func (w *WizardModel) saveKeys() {
	for provider, key := range w.collected {
		w.cfg.APIKeys[provider] = key
	}
	if len(w.collected) > 0 {
		_ = config.Save(w.cfg)
	}
}

func (w *WizardModel) mainModel() *Model {
	m := NewModel()
	m.SetConfig(w.cfg)
	return m
}

func (w *WizardModel) View() string {
	switch w.step {
	case WizWelcome:
		return w.viewWelcome()
	case WizProviders:
		return w.viewProviders()
	case WizKeyEntry:
		return w.viewKeyEntry()
	case WizDone:
		return w.viewDone()
	}
	return ""
}

var wizardStyle = lipgloss.NewStyle().Padding(1, 2)
var wizardBox = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("62")).
	Padding(1, 2).
	Width(64)
var wizardTitle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))

func (w *WizardModel) viewWelcome() string {
	logo := `
  ___  ___  ___  ___  ___  ___  ___  ___
 | __|| _ \| __|| __|| __|| _ \|_ _|| _ \
 | _| |   /| _| | _| | __||   / | | |  _/
 |_|  |_|_\|___||___||___||_|_\ |_| |_|_\

       Free AI Model Router
        zero-cost inference gateway

  Discover and proxy free-tier LLM models
  from 10+ providers — zero API costs.

  This wizard will help you set up API keys
  for each provider (optional — some models
  work without keys).

  [Enter] Start setup   [S] Skip for now   [Q] Quit
`
	return wizardBox.Render(logo)
}

func (w *WizardModel) viewProviders() string {
	var b strings.Builder
	b.WriteString(wizardTitle.Render("  API Key Setup"))
	b.WriteString("\n\n")

	b.WriteString("  For each provider, choose:\n")
	b.WriteString("  [O] Open browser + enter key\n")
	b.WriteString("  [E] Enter key manually\n")
	b.WriteString("  [S] Skip\n\n")

	for i, p := range wizardProviders {
		prefix := "  "
		cursor := " "
		if i == w.idx {
			prefix = "▶ "
			cursor = lipgloss.NewStyle().Foreground(lipgloss.Color("45")).Render(">")
		}
		status := ""
		if _, ok := w.collected[p.Name]; ok {
			status = lipgloss.NewStyle().Foreground(lipgloss.Color("46")).Render(" ✓ key set")
		}
		b.WriteString(fmt.Sprintf("%s%s %-14s %s\n", prefix, cursor, p.Name, status))
	}

	b.WriteString("\n  ↑↓/jk: navigate  O/E/S: choose  Q: finish\n")
	if w.msg != "" {
		b.WriteString("\n  " + w.msg)
	}

	return wizardBox.Render(b.String())
}

func (w *WizardModel) viewKeyEntry() string {
	if w.idx >= len(wizardProviders) {
		return w.wizardBox("Saving...")
	}

	p := wizardProviders[w.idx]
	var b strings.Builder
	b.WriteString(wizardTitle.Render(fmt.Sprintf("  Enter API Key: %s", p.Name)))
	b.WriteString("\n\n")

	masked := strings.Repeat("•", len(w.keyBuf))
	if masked == "" {
		masked = "(type to enter...)"
	}

	b.WriteString(fmt.Sprintf("  Key: %s\n", masked))
	b.WriteString(fmt.Sprintf("\n  %s\n", p.SignupURL))

	b.WriteString("\n  [Enter] Save   [Esc] Cancel\n")
	if w.msg != "" {
		b.WriteString("\n  " + w.msg)
	}

	return wizardBox.Render(b.String())
}

func (w *WizardModel) viewDone() string {
	w.saveKeys()

	var b strings.Builder
	var keys []string
	for p := range w.collected {
		keys = append(keys, p)
	}
	b.WriteString(wizardTitle.Render("  Setup Complete!"))
	b.WriteString("\n\n")
	if len(keys) > 0 {
		b.WriteString(fmt.Sprintf("  Configured: %s\n", strings.Join(keys, ", ")))
	} else {
		b.WriteString("  No keys configured — using keyless models only.\n")
	}
	b.WriteString("\n  Press any key to start...")

	return wizardBox.Render(b.String())
}

func (w *WizardModel) wizardBox(content string) string {
	return wizardBox.Render(content)
}

// openBrowser opens url with the platform's default browser (best effort).
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Run()
}
