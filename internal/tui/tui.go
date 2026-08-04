package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

// Screen kind constants for routing.
type screenKind int

const (
	screenTable    screenKind = iota // main model table
	screenSettings                   // provider config
	screenPicker                     // target agent picker
	screenHelp                       // help overlay
)

// Model is the top-level Bubble Tea model that owns shared state
// (registry, config, engine) and routes to sub-screens.
type Model struct {
	registry *models.Registry
	cfg      *config.Config
	engine   *ping.Engine
	width    int
	height   int

	// Active screen
	screen screenKind

	// Sub-screens
	table    *TableScreen
	settings *SettingsScreen
	picker   *PickerScreen

	// First-run wizard
	wizardActive bool
	wizard       *WizardModel

	quit bool
}

func NewModel() *Model {
	return &Model{
		table: NewTableScreen(),
	}
}

func (m *Model) SetRegistry(registry *models.Registry) {
	m.registry = registry
	m.table.SetRegistry(registry)
	if m.settings != nil {
		m.settings.SetConfig(m.cfg)
	}

	m.engine = ping.NewEngine(nil)
	m.engine.SetRegistry(registry)
	m.engine.SetModels(registry.Snapshot())
	m.engine.Start()

	m.table.SetEngine(m.engine)
}

func (m *Model) SetConfig(cfg *config.Config) {
	m.cfg = cfg
	m.table.SetConfig(cfg)
	if m.settings != nil {
		m.settings.SetConfig(cfg)
	}
}

type tickMsg time.Time

func (m *Model) Init() tea.Cmd {
	return tea.Sequence(tea.EnterAltScreen, tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg { return tickMsg(time.Now()) }))
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Delegate to wizard when active.
	if m.wizardActive && m.wizard != nil {
		wm, cmd := m.wizard.Update(msg)
		newWiz := wm.(*WizardModel)
		m.wizard = newWiz
		if newWiz.step == WizDone {
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
		m.table.SetSize(msg.Width, msg.Height)
		return m, nil

	case tickMsg:
		return m, tea.Tick(500*time.Millisecond, func(_ time.Time) tea.Msg { return tickMsg(time.Now()) })

	case tea.KeyMsg:
		// Global quit
		if msg.String() == "ctrl+c" || (msg.String() == "q" && m.screen == screenTable) {
			m.quit = true
			return m, tea.Quit
		}

		switch m.screen {
		case screenSettings:
			cmd, keep := m.settings.HandleKey(msg)
			if !keep {
				m.screen = screenTable
			}
			return m, cmd

		case screenPicker:
			cmd, keep := m.picker.HandleKey(msg)
			if !keep {
				m.screen = screenTable
			}
			return m, cmd

		case screenHelp:
			if msg.String() == "q" || msg.String() == "esc" {
				m.screen = screenTable
			}
			return m, nil

		default: // screenTable
			cmd, screen := m.table.HandleKey(msg)
			switch screen {
			case "settings":
				m.settings = NewSettingsScreen(m.cfg)
				m.screen = screenSettings
			case "picker":
				m.picker = NewPickerScreen(m.table.SelectedModel())
				m.screen = screenPicker
			case "help":
				m.screen = screenHelp
			case "quit":
				m.quit = true
				return m, tea.Quit
			}
			return m, cmd
		}
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

	switch m.screen {
	case screenSettings:
		return m.settings.View()
	case screenPicker:
		return m.picker.View()
	case screenHelp:
		return RenderHelp()
	default: // screenTable
		return m.table.View()
	}
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
