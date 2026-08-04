//go:build windows

package tui

import (
	"os"
	"os/signal"
)

func setupSignals(sigCh chan os.Signal) {
	signal.Notify(sigCh, os.Interrupt)
}

func handleSignal(t *TUI, sig os.Signal) {
	t.quit = true
}