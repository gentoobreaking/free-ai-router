package tui

import (
	"syscall"
	"testing"
)

// TestHandleSignalSIGWINCHResizes: terminal resize must re-render, not quit
// (T048).
func TestHandleSignalSIGWINCHResizes(t *testing.T) {
	tui := &TUI{height: 40, width: 80}
	tui.handleSignal(syscall.SIGWINCH)

	if tui.quit {
		t.Error("SIGWINCH must not quit the TUI")
	}
	if !tui.renderPending.Load() {
		t.Error("SIGWINCH should trigger a re-render")
	}
}

// TestHandleSignalQuits: SIGINT/SIGTERM terminate the TUI.
func TestHandleSignalQuits(t *testing.T) {
	for _, sig := range []syscall.Signal{syscall.SIGINT, syscall.SIGTERM} {
		tui := &TUI{}
		tui.handleSignal(sig)
		if !tui.quit {
			t.Errorf("%v should quit the TUI", sig)
		}
	}
}
