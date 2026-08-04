package providers

import (
	"fmt"
	"io"
	"os"
)

// LogLevel controls the verbosity of discovery logging.
type LogLevel int

const (
	LevelDebug  LogLevel = iota // detailed raw data
	LevelInfo                   // progress & results
	LevelWarn                   // anomalies only
	LevelSilent                 // nothing
)

// DiscoveryLogger receives structured log messages from the four-phase
// discovery pipeline. Implementations are free to write to stderr, a file,
// or structured loggers like slog.
type DiscoveryLogger interface {
	Info(format string, args ...interface{})
	Warn(format string, args ...interface{})
	Debug(format string, args ...interface{})
}

// defaultLogger writes to os.Stderr with a "[discovery]" prefix.
type defaultLogger struct {
	level LogLevel
	w     io.Writer
}

// NewDefaultLogger returns a DiscoveryLogger that writes to stderr.
func NewDefaultLogger(level LogLevel) DiscoveryLogger {
	return &defaultLogger{level: level, w: os.Stderr}
}

func (l *defaultLogger) Info(format string, args ...interface{}) {
	if l.level <= LevelInfo {
		fmt.Fprintf(l.w, "[discovery] "+format+"\n", args...)
	}
}

func (l *defaultLogger) Warn(format string, args ...interface{}) {
	if l.level <= LevelWarn {
		fmt.Fprintf(l.w, "[discovery] "+format+"\n", args...)
	}
}

func (l *defaultLogger) Debug(format string, args ...interface{}) {
	if l.level <= LevelDebug {
		fmt.Fprintf(l.w, "[discovery] "+format+"\n", args...)
	}
}

// loggerFor returns the Manager's logger or a nil-safe no-op logger.
func loggerFor(m *Manager) DiscoveryLogger {
	if m != nil && m.logger != nil {
		return m.logger
	}
	return &nilLogger{}
}

// nilLogger safely discards all log output when no logger is configured.
type nilLogger struct{}

func (n *nilLogger) Info(string, ...interface{})  {}
func (n *nilLogger) Warn(string, ...interface{})  {}
func (n *nilLogger) Debug(string, ...interface{}) {}
