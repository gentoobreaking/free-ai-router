package tui

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/targets"
)

const defaultScrollSortPause = 1500 * time.Millisecond

type Model struct {
	registry        *models.Registry
	cfg             *config.Config
	engine          *ping.Engine
	width           int
	height          int
	selected        int
	searchQuery     string
	searchActive    bool
	sortKey         string
	sortReverse     bool
	tierFilter      string
	providerFilter  string
	codingOnly      bool
	intervalMs      int
	showSettings    bool
	settingsIndex   int
	settingsKeyEdit bool
	settingsKeyBuf  string
	settingsEditFor string
	settingsMsg     string
	showHelp        bool
	pickerOpen      bool
	pickerIndex     int
	pickerMsg       string
	pickerTargets   []targets.Target
	quit            bool
	paused          bool
	pauseUntil      time.Time
	pauseMs         time.Duration

	// First-run wizard state
	wizardActive bool
	wizard       *WizardModel
}

func NewModel() *Model {
	return &Model{
		sortKey:    "0",
		intervalMs: 2000,
		codingOnly: true,
		width:      120,
		height:     40,
		pauseMs:    defaultScrollSortPause,
	}
}

func (m *Model) SetRegistry(registry *models.Registry) {
	m.registry = registry
	m.engine = ping.NewEngine(nil)
	m.engine.SetRegistry(registry)
	m.engine.SetModels(registry.Snapshot())
	m.engine.Start()
}

func (m *Model) SetConfig(cfg *config.Config) {
	m.cfg = cfg
	m.pauseMs = time.Duration(cfg.UI.ScrollSortPauseMs) * time.Millisecond
}

type tickMsg time.Time

func (m *Model) Init() tea.Cmd {
	return tea.Sequence(tea.EnterAltScreen, tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg { return tickMsg(time.Now()) }))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Delegate to wizard when active
	if m.wizardActive && m.wizard != nil {
		wm, cmd := m.wizard.Update(msg)
		newWiz := wm.(*WizardModel)
		m.wizard = newWiz
		if newWiz.step == WizDone {
			// Wizard finished — apply collected keys to config
			for provider, key := range newWiz.collected {
				if m.cfg != nil {
					m.cfg.APIKeys[provider] = key
					pcfg := m.cfg.Providers[provider]
					pcfg.Enabled = true
					m.cfg.Providers[provider] = pcfg
				}
			}
			if len(newWiz.collected) > 0 && m.cfg != nil {
				_ = config.Save(m.cfg)
			}
			m.wizardActive = false
			m.wizard = nil
		}
		if newWiz.quit {
			m.wizardActive = false
			m.wizard = nil
			m.quit = true
			return m, tea.Quit
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		return m, tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg { return tickMsg(time.Now()) })

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.quit = true
			return m, tea.Quit
		}

		if m.pickerOpen {
			return m.handlePickerInput(msg)
		}
		if m.showSettings {
			return m.handleSettingsInput(msg)
		}
		if m.searchActive {
			return m.handleSearchInput(msg)
		}
		return m.handleInput(msg)
	}

	return m, nil
}

func (m *Model) View() string {
	if m.registry == nil && !m.wizardActive {
		return lipgloss.NewStyle().Align(lipgloss.Center).Render("Loading...")
	}

	if m.wizardActive && m.wizard != nil {
		return m.wizard.View()
	}

	if m.showHelp {
		return RenderHelp()
	}
	if m.showSettings {
		return RenderSettings(m.settingsProviders(), m.settingsIndex, m.settingsKeyEdit, m.settingsKeyBuf, m.settingsMsg)
	}
	if m.pickerOpen {
		return m.renderTargetPicker()
	}

	return RenderLayout(m.renderOptions())
}

func (m *Model) renderOptions() *RenderOptions {
	return &RenderOptions{
		Models:         m.filteredModels(),
		SelectedIndex:  m.selected,
		SearchQuery:    m.searchQuery,
		SearchActive:   m.searchActive,
		TotalCount:     len(m.registry.Snapshot()),
		SortKey:        m.sortKey,
		SortReverse:    m.sortReverse,
		TierFilter:     m.tierFilter,
		ProviderFilter: m.providerFilter,
		CodingOnly:     m.codingOnly,
		IntervalMs:     m.intervalMs,
		Width:          m.width,
		Height:         m.height,
	}
}

func (m *Model) filteredModels() []*models.Model {
	return FilterModels(m.registry.Snapshot(), m.codingOnly, m.tierFilter, m.providerFilter, m.searchQuery)
}

func (m *Model) handleInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.navigate(-1)
	case "down", "j":
		m.navigate(1)
	case "pageup":
		m.navigate(-10)
	case "pagedown":
		m.navigate(10)
	case "g":
		m.selected = 0
	case "G":
		m.selected = m.visibleCount() - 1
	case "enter":
		m.openTargetPicker()
	case "/":
		m.searchActive = true
	case "c", "C":
		m.codingOnly = !m.codingOnly
	case "t", "T":
		m.cycleTierFilter()
	case "n", "N":
		m.cycleProviderFilter()
	case "w":
		m.changeInterval(-500)
	case "x":
		m.changeInterval(500)
	case "p", "P":
		m.showSettings = true
		m.settingsIndex = 0
	case "?":
		m.showHelp = true
	}
	return m, nil
}

func (m *Model) navigate(delta int) {
	m.paused = true
	m.pauseUntil = time.Now().Add(m.pauseMs)
	m.selected += delta
	if m.selected < 0 {
		m.selected = 0
	}
}

func (m *Model) visibleCount() int {
	if m.height < 10 {
		return 5
	}
	return m.height - 10
}

func (m *Model) openTargetPicker() {
	m.pickerTargets = []targets.Target{
		&targets.OpenCodeTarget{},
		&targets.OpenClawTarget{},
		&targets.HermesTarget{},
		&targets.PiTarget{},
	}
	m.pickerIndex = 0
	m.pickerMsg = ""
	m.pickerOpen = true
}

func (m *Model) handlePickerInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.pickerIndex--
		if m.pickerIndex < 0 {
			m.pickerIndex = len(m.pickerTargets) - 1
		}
	case "down", "j":
		m.pickerIndex++
		if m.pickerIndex >= len(m.pickerTargets) {
			m.pickerIndex = 0
		}
	case "enter":
		m.saveTargetConfig()
	case "esc", "q":
		m.pickerOpen = false
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) saveTargetConfig() {
	if m.pickerIndex < 0 || m.pickerIndex >= len(m.pickerTargets) {
		return
	}
	var current *models.Model
	if m.selected >= 0 && m.selected < len(m.registry.Snapshot()) {
		current = m.registry.Snapshot()[m.selected]
	}
	if current == nil {
		current = models.FindBestModel(m.registry.Snapshot())
	}
	if current == nil {
		m.pickerMsg = "no model selected"
		return
	}
	target := m.pickerTargets[m.pickerIndex]
	if err := target.Write(current.ID); err != nil {
		m.pickerMsg = "failed: " + err.Error()
	} else {
		m.pickerMsg = "saved " + current.ID + " to " + target.Name()
	}
	m.pickerOpen = false
}

func (m *Model) handleSearchInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searchActive = false
		m.openTargetPicker()
	case "esc":
		m.searchQuery = ""
		m.searchActive = false
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	}
	return m, nil
}

func (m *Model) handleSettingsInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// When editing a key, handle key entry / backspace / enter / esc
	if m.settingsKeyEdit {
		switch msg.String() {
		case "enter":
			// Save key
			if m.cfg != nil && m.settingsEditFor != "" {
				m.cfg.APIKeys[m.settingsEditFor] = m.settingsKeyBuf
				_ = config.Save(m.cfg)
			}
			m.settingsKeyEdit = false
			m.settingsKeyBuf = ""
			m.settingsEditFor = ""
		case "esc":
			m.settingsKeyEdit = false
			m.settingsKeyBuf = ""
			m.settingsEditFor = ""
		case "backspace":
			if len(m.settingsKeyBuf) > 0 {
				m.settingsKeyBuf = m.settingsKeyBuf[:len(m.settingsKeyBuf)-1]
			}
		default:
			s := msg.String()
			// Accept printable characters only
			if len(msg.Runes) == 1 && msg.Runes[0] >= 32 && msg.Runes[0] < 127 {
				m.settingsKeyBuf += s
			}
		}
		return m, nil
	}

	switch msg.String() {
	case "esc", "q":
		m.showSettings = false
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	case "up", "k":
		if m.settingsIndex > 0 {
			m.settingsIndex--
		}
	case "down", "j":
		providers := m.settingsProviders()
		if m.settingsIndex < len(providers)-1 {
			m.settingsIndex++
		}
	case " ":
		// Toggle provider enabled status
		providers := m.settingsProviders()
		if m.settingsIndex >= 0 && m.settingsIndex < len(providers) {
			name := providers[m.settingsIndex].Name
			if m.cfg != nil {
				pcfg := m.cfg.Providers[name]
				pcfg.Enabled = !pcfg.Enabled
				m.cfg.Providers[name] = pcfg
				_ = config.Save(m.cfg)
			}
		}
	case "enter":
		// Enter inline key editing mode
		providers := m.settingsProviders()
		if m.settingsIndex >= 0 && m.settingsIndex < len(providers) {
			p := providers[m.settingsIndex]
			m.settingsKeyEdit = true
			m.settingsEditFor = p.Name
			m.settingsKeyBuf = p.Key
		}
	case "t", "T":
		// Test ping for selected provider
		providers := m.settingsProviders()
		if m.settingsIndex >= 0 && m.settingsIndex < len(providers) {
			name := providers[m.settingsIndex].Name
			// Find a model from this provider and ping it
			for _, mdl := range m.registry.Snapshot() {
				if mdl.Provider == name && mdl.Status != "pending" {
					status, code, lat := pingModelNowTUI(mdl)
					if status == "up" {
						m.settingsMsg = fmt.Sprintf("%s: up (%d, %dms)", name, code, lat.Milliseconds())
					} else {
						m.settingsMsg = fmt.Sprintf("%s: %s (%d, %dms)", name, status, code, lat.Milliseconds())
					}
					return m, nil
				}
			}
			m.settingsMsg = name + ": no models to test"
		}
	case "d", "D":
		// Delete key for selected provider
		providers := m.settingsProviders()
		if m.settingsIndex >= 0 && m.settingsIndex < len(providers) {
			name := providers[m.settingsIndex].Name
			if m.cfg != nil {
				delete(m.cfg.APIKeys, name)
				_ = config.Save(m.cfg)
			}
		}
	case "o", "O":
		// Open signup page
		providers := m.settingsProviders()
		if m.settingsIndex >= 0 && m.settingsIndex < len(providers) {
			name := providers[m.settingsIndex].Name
			if url := signupURL(name); url != "" {
				_ = openBrowser(url)
				m.settingsMsg = "opened " + url
			} else {
				m.settingsMsg = "no signup URL for " + name
			}
		}
	}
	return m, nil
}

// signupURL returns the signup URL for a provider or empty string.
func signupURL(provider string) string {
	urls := map[string]string{
		"nvidia":       "https://build.nvidia.com/explore/discover",
		"groq":         "https://console.groq.com/keys",
		"cerebras":     "https://cloud.cerebras.ai/",
		"openrouter":   "https://openrouter.ai/keys",
		"googleai":     "https://aistudio.google.com/apikey",
		"opencode":     "https://opencode.ai",
		"codestral":    "https://console.mistral.ai/",
		"scaleway":     "https://console.scaleway.com/",
		"kilocode":     "https://kilocode.ai",
		"siliconflow":  "https://siliconflow.cn/",
		"baidu":        "https://console.bce.baidu.com/qianfan/",
		"alibabacloud": "https://dashscope.aliyun.com/",
		"tencent":      "https://console.cloud.tencent.com/hunyuan",
	}
	return urls[provider]
}

// pingModelNowTUI mirrors the server-side pingModelNow but uses the TUI's
// shared engine pool for consistency.
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

func (m *Model) cycleTierFilter() {
	tiers := []string{"All", "S+", "S", "A+", "A", "A-", "B+", "B", "C"}
	next := 0
	for i, v := range tiers {
		if v == m.tierFilter {
			next = (i + 1) % len(tiers)
			break
		}
	}
	m.tierFilter = tiers[next]
}

func (m *Model) cycleProviderFilter() {
	providers := []string{"All", "nvidia", "openrouter", "groq", "cerebras", "opencode", "googleai"}
	next := 0
	for i, v := range providers {
		if v == m.providerFilter {
			next = (i + 1) % len(providers)
			break
		}
	}
	m.providerFilter = providers[next]
}

func (m *Model) changeInterval(dir int) {
	m.intervalMs += dir
	if m.intervalMs < 500 {
		m.intervalMs = 500
	}
	if m.intervalMs > 10000 {
		m.intervalMs = 10000
	}
}

func (m *Model) settingsProviders() []SettingsProvider {
	providers := []SettingsProvider{
		{Name: "nvidia", Enabled: false},
		{Name: "groq", Enabled: false},
		{Name: "cerebras", Enabled: false},
		{Name: "openrouter", Enabled: false},
		{Name: "googleai", Enabled: false},
		{Name: "opencode", Enabled: false},
		{Name: "codestral", Enabled: false},
		{Name: "scaleway", Enabled: false},
		{Name: "kilocode", Enabled: false},
		{Name: "ollama", Enabled: false},
		{Name: "clawlabs", Enabled: false},
		{Name: "new-api", Enabled: false},
		{Name: "siliconflow", Enabled: false},
		{Name: "baidu", Enabled: false},
		{Name: "alibabacloud", Enabled: false},
		{Name: "tencent", Enabled: false},
		{Name: "kuaipao", Enabled: false},
	}
	if m.cfg != nil {
		for i := range providers {
			name := providers[i].Name
			if pcfg, ok := m.cfg.Providers[name]; ok {
				providers[i].Enabled = pcfg.Enabled
			}
			if key := config.ResolveAPIKey(name, m.cfg); key != "" {
				providers[i].Key = key
			}
		}
	}
	if m.registry != nil {
		for _, m := range m.registry.Snapshot() {
			for i := range providers {
				if providers[i].Name == m.Provider && m.Status == "up" {
					providers[i].TestStatus = "up"
				}
			}
		}
	}
	return providers
}

func (m *Model) renderTargetPicker() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Configure for target agent"))
	b.WriteString("\n\n")
	for i, target := range m.pickerTargets {
		marker := "  "
		if i == m.pickerIndex {
			marker = "> "
		}
		b.WriteString(fmt.Sprintf("%s %-16s %s\n", marker, target.Name(), target.ConfigPath()))
	}
	b.WriteString("\n  ↑↓/jk: navigate  Enter: save  ESC/q: back")
	if m.pickerMsg != "" {
		b.WriteString("\n\n  " + m.pickerMsg)
	}
	return b.String()
}

func Run(registry *models.Registry, cfg *config.Config) error {
	m := NewModel()
	m.SetRegistry(registry)
	m.SetConfig(cfg)

	if isFirstRun(cfg) {
		m.wizard = NewWizardModel(cfg)
		m.wizardActive = true
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}
