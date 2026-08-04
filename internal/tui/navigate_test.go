package tui

import (
	"testing"
	"time"
)

// TestNavigateUsesConfiguredPause: navigate() must pause re-sorting for the
// configured ScrollSortPauseMs, not a hardcoded value (T038).
func TestNavigateUsesConfiguredPause(t *testing.T) {
	tui := &TUI{pauseMs: 500 * time.Millisecond, height: 40}
	tui.navigate(1)

	if tui.selected != 1 {
		t.Errorf("selected = %d, want 1", tui.selected)
	}
	if !tui.paused {
		t.Error("navigate should pause rendering")
	}
	wait := time.Until(tui.pauseUntil)
	if wait > 650*time.Millisecond || wait < 350*time.Millisecond {
		t.Errorf("pause duration = %v, want ~500ms", wait)
	}
	if !tui.renderPending.Load() {
		t.Error("navigate should mark render pending")
	}
}

// TestNavigateDefaultsTo1500ms: without config, navigate() uses the default
// 1500ms pause.
func TestNavigateDefaultsTo1500ms(t *testing.T) {
	tui := New(nil)
	tui.navigate(1)

	wait := time.Until(tui.pauseUntil)
	if wait > 1650*time.Millisecond || wait < 1350*time.Millisecond {
		t.Errorf("pause duration = %v, want ~1500ms", wait)
	}
}

// TestNewAppliesScrollSortPauseMs: New() must honor the config value.
func TestNewAppliesScrollSortPauseMs(t *testing.T) {
	tui := New(&Config{ScrollSortPauseMs: 250})
	if tui.pauseMs != 250*time.Millisecond {
		t.Errorf("pauseMs = %v, want 250ms", tui.pauseMs)
	}
}

// TestNewDefaultPause: New(nil) or zero config keeps the default.
func TestNewDefaultPause(t *testing.T) {
	if got := New(nil).pauseMs; got != defaultScrollSortPause {
		t.Errorf("pauseMs = %v, want default %v", got, defaultScrollSortPause)
	}
	if got := New(&Config{ScrollSortPauseMs: 0}).pauseMs; got != defaultScrollSortPause {
		t.Errorf("pauseMs = %v, want default %v", got, defaultScrollSortPause)
	}
}
