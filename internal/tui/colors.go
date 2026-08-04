package tui

import "fmt"

const (
	Reset      = "\033[0m"
	Bold       = "\033[1m"
	Dim        = "\033[2m"
	Underline  = "\033[4m"
	Reverse    = "\033[7m"

	Black   = "\033[30m"
	Red     = "\033[31m"
	Green   = "\033[32m"
	Yellow  = "\033[33m"
	Blue    = "\033[34m"
	Magenta = "\033[35m"
	Cyan    = "\033[36m"
	White   = "\033[37m"

	BrightBlack   = "\033[90m"
	BrightRed     = "\033[91m"
	BrightGreen   = "\033[92m"
	BrightYellow  = "\033[93m"
	BrightBlue    = "\033[94m"
	BrightMagenta = "\033[95m"
	BrightCyan    = "\033[96m"
	BrightWhite   = "\033[97m"

	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"
	BgGrey    = "\033[100m"
)

func Color(s, color string) string {
	return color + s + Reset
}

func Colorize(text string, enabled bool) string {
	if !enabled {
		return text
	}
	return text
}

func CursorHome() string       { return "\033[H" }
func CursorHide() string       { return "\033[?25l" }
func CursorShow() string       { return "\033[?25h" }
func ClearScreen() string      { return "\033[2J" }
func ClearLine() string        { return "\033[2K" }
func EnterAltScreen() string   { return "\033[?1049h" }
func ExitAltScreen() string    { return "\033[?1049l" }
func EnableFocus() string      { return "\033[?1004h" }
func DisableFocus() string     { return "\033[?1004l" }
func MoveTo(row, col int) string {
	return fmt.Sprintf("\033[%d;%dH", row, col)
}
