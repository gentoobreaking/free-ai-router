package tui

import (
	"fmt"
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
	registry    *models.Registry
	cfg         *config.Config
	engine      *ping.Engine
	width       int
	height      int
	selected    int
	searchQuery string
	searchActive bool
	sortKey     string
	sortReverse bool
	tierFilter  string
	providerFilter string
	codingOnly  bool
	intervalMs  int
	showSettings bool
	settingsIndex int
	settingsKeyEdit bool
	settingsKeyBuf string
	showHelp    bool
	pickerOpen  bool
	pickerIndex int
	pickerMsg   string
	pickerTargets []targets.Target
	quit        bool
	paused      bool
	pauseUntil  time.Time
	pauseMs     time.Duration
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
}

func (m *Model) SetConfig(cfg *config.Config) {
	m.cfg = cfg
	m.pauseMs = time.Duration(cfg.UI.ScrollSortPauseMs) * time.Millisecond
}

func (m *Model) Init() tea.Cmd {
	return tea.EnterAltScreen
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

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
	if m.registry == nil {
		return lipgloss.NewStyle().Align(lipgloss.Center).Render("Loading...")
	}

	if m.showHelp {
		return RenderHelp()
	}
	if m.showSettings {
		return RenderSettings(m.settingsProviders())
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
	switch msg.String() {
	case "esc", "q":
		m.showSettings = false
	case "ctrl+c":
		m.quit = true
		return m, tea.Quit
	}
	return m, nil
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
	p := tea.NewProgram(m, tea.WithAltScreen())
	_, err := p.Run()
	return err
}