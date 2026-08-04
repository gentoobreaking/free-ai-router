package tui

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/freemodel/router/internal/models"
)

func TestNavigateMovesSelectionDown(t *testing.T) {
	m := NewModel()
	reg := models.NewRegistry()
	reg.Add(&models.Model{ID: "a/model1", Label: "Model 1", Provider: "a", Tier: "S+", Status: "up", Tags: []string{"coding"}})
	reg.Add(&models.Model{ID: "b/model2", Label: "Model 2", Provider: "b", Tier: "A+", Status: "up", Tags: []string{"general"}})
	m.SetRegistry(reg)
	m.table.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyDown}
	m.Update(msg)

	if m.table.selected != 1 {
		t.Errorf("selected = %d, want 1", m.table.selected)
	}
}

func TestNavigateMovesSelectionUp(t *testing.T) {
	m := NewModel()
	reg := models.NewRegistry()
	reg.Add(&models.Model{ID: "a/model1", Label: "Model 1", Provider: "a", Tier: "S+", Status: "up", Tags: []string{"coding"}})
	reg.Add(&models.Model{ID: "b/model2", Label: "Model 2", Provider: "b", Tier: "A+", Status: "up", Tags: []string{"general"}})
	m.SetRegistry(reg)
	m.table.selected = 1

	msg := tea.KeyMsg{Type: tea.KeyUp}
	m.Update(msg)

	if m.table.selected != 0 {
		t.Errorf("selected = %d, want 0", m.table.selected)
	}
}

func TestNavigatePausesForScrollSortPause(t *testing.T) {
	m := NewModel()
	m.table.pauseMs = 500 * time.Millisecond
	reg := models.NewRegistry()
	reg.Add(&models.Model{ID: "a/model1", Label: "Model 1", Provider: "a", Tier: "S+", Status: "up", Tags: []string{"coding"}})
	reg.Add(&models.Model{ID: "b/model2", Label: "Model 2", Provider: "b", Tier: "A+", Status: "up", Tags: []string{"general"}})
	m.SetRegistry(reg)
	m.table.selected = 0

	msg := tea.KeyMsg{Type: tea.KeyDown}
	m.Update(msg)

	if m.table.selected != 1 {
		t.Errorf("selected = %d, want 1", m.table.selected)
	}
	if !m.table.paused {
		t.Error("navigate should pause rendering")
	}
	wait := time.Until(m.table.pauseUntil)
	if wait > 650*time.Millisecond || wait < 350*time.Millisecond {
		t.Errorf("pause duration = %v, want ~500ms", wait)
	}
}

func TestNavigateDefaultPauseMs(t *testing.T) {
	m := NewModel()
	if m.table.pauseMs != defaultScrollSortPause {
		t.Errorf("pauseMs = %v, want default %v", m.table.pauseMs, defaultScrollSortPause)
	}
}

func TestNavigateWithCustomPauseMs(t *testing.T) {
	m := NewModel()
	m.table.pauseMs = 250 * time.Millisecond
	if m.table.pauseMs != 250*time.Millisecond {
		t.Errorf("pauseMs = %v, want 250ms", m.table.pauseMs)
	}
}
