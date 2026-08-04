//go:build unix

package tui

import (
	"os"
	"os/signal"
	"syscall"
)

func setupSignals(sigCh chan os.Signal) {
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
}

func handleSignal(t *TUI, sig os.Signal) {
	if sig == syscall.SIGWINCH {
		t.resize()
		return
	}
	t.quit = true
}