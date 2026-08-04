package router

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

const MaxLogEntries = 200

type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Model     string    `json:"model"`
	Provider  string    `json:"provider"`
	Status    int       `json:"status"`
	Duration  time.Duration `json:"durationMs"`
	TTFB      time.Duration `json:"ttfbMs"`
	Retries   []int     `json:"retries,omitempty"`
}

type Logger struct {
	mu      sync.Mutex
	enabled bool
	entries []LogEntry
	path    string
}

func NewLogger(enabled bool) *Logger {
	l := &Logger{
		enabled: enabled,
		entries: make([]LogEntry, 0, MaxLogEntries),
	}
	home, err := os.UserHomeDir()
	if err == nil {
		l.path = home + "/.freemodel-router-logs.json"
	}
	return l
}

func (l *Logger) Log(entry *LogEntry) {
	if !l.enabled {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	l.entries = append(l.entries, *entry)
	if len(l.entries) > MaxLogEntries {
		l.entries = l.entries[len(l.entries)-MaxLogEntries:]
	}

	if l.path != "" {
		data, err := json.Marshal(l.entries)
		if err == nil {
			_ = os.WriteFile(l.path, data, 0600)
		}
	}
}

func (l *Logger) Recent() []LogEntry {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := make([]LogEntry, len(l.entries))
	copy(result, l.entries)
	return result
}

func (l *Logger) Enabled() bool {
	return l.enabled
}
