package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/targets"
)

// PickerScreen manages the target-agent selection / "configure" view.
type PickerScreen struct {
	targets []targets.Target
	index   int
	message string
	model   *models.Model
}

func NewPickerScreen(mdl *models.Model) *PickerScreen {
	return &PickerScreen{
		targets: PickerTargets(),
		model:   mdl,
	}
}

func (p *PickerScreen) SetModel(mdl *models.Model) { p.model = mdl }

// HandleKey returns (cmd, keepOpen). keepOpen=false means return to table.
func (p *PickerScreen) HandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	switch msg.String() {
	case "up", "k":
		p.index--
		if p.index < 0 {
			p.index = len(p.targets) - 1
		}
	case "down", "j":
		p.index++
		if p.index >= len(p.targets) {
			p.index = 0
		}
	case "enter":
		p.saveTargetConfig()
		return nil, false
	case "esc", "q":
		return nil, false
	case "ctrl+c":
		return tea.Quit, false
	}
	return nil, true
}

func (p *PickerScreen) saveTargetConfig() {
	if p.index < 0 || p.index >= len(p.targets) {
		return
	}
	if p.model == nil {
		p.message = "no model selected"
		return
	}
	target := p.targets[p.index]
	if err := target.Write(p.model.ID); err != nil {
		p.message = "failed: " + err.Error()
	} else {
		p.message = "saved " + p.model.ID + " to " + target.Name()
	}
}

func (p *PickerScreen) View() string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Configure for target agent"))
	b.WriteString("\n\n")
	for i, target := range p.targets {
		marker := "  "
		if i == p.index {
			marker = "> "
		}
		b.WriteString(fmt.Sprintf("%s %-16s %s\n", marker, target.Name(), target.ConfigPath()))
	}
	b.WriteString("\n  ↑↓/jk: navigate  Enter: save  ESC/q: back")
	if p.message != "" {
		b.WriteString("\n\n  " + p.message)
	}
	return b.String()
}
