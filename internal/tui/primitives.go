package tui

import (
	"strings"
)

type TableCell struct {
	Text  string
	Width int
	Color string
}

func RenderCell(c TableCell) string {
	text := truncate(c.Text, c.Width)
	text = padRight(text, c.Width)
	if c.Color != "" {
		return Color(text, c.Color)
	}
	return text
}

func truncate(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 1 {
		return s[:1]
	}
	return s[:max-1] + "…"
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

func StatusDot(status string, httpCode string) string {
	switch status {
	case "up":
		return Color("*", Green)
	case "noauth":
		return Color("!", Yellow)
	case "forbidden":
		return Color("!", Red)
	case "ratelimit":
		return Color("~", BrightYellow)
	case "unavailable":
		return Color("#", Red)
	case "notfound":
		return Color("?", Red)
	case "timeout":
		return Color("o", Red)
	case "down":
		return Color("x", Red)
	default:
		return Color(".", Dim)
	}
}

func ProviderTag(state string) string {
	switch state {
	case "ready":
		return BgGreen + " READY " + Reset
	case "nokey":
		return BgYellow + " NO KEY " + Reset
	case "wrongkey":
		return BgRed + " WRONG KEY " + Reset
	case "off":
		return BgGrey + " OFF " + Reset
	default:
		return BgGrey + " ? " + Reset
	}
}

func Bar(value float64, max float64, width int) string {
	if max <= 0 {
		max = 1
	}
	if value < 0 {
		value = 0
	}
	filled := int(float64(width) * value / max)
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func BorderRow(width int) string {
	if width < 1 {
		width = 1
	}
	return "┌" + strings.Repeat("─", width) + "┐"
}

func BorderRowBottom(width int) string {
	if width < 1 {
		width = 1
	}
	return "└" + strings.Repeat("─", width) + "┘"
}

func SeparatorRow(width int) string {
	if width < 1 {
		width = 1
	}
	return "├" + strings.Repeat("─", width) + "┤"
}

func TierColor(tier string) string {
	switch tier {
	case "S+", "S":
		return BrightGreen
	case "A+", "A":
		return Green
	case "A-", "B+":
		return Cyan
	case "B":
		return Yellow
	default:
		return Dim
	}
}

func LatencyColor(ms float64) string {
	switch {
	case ms < 400:
		return Green
	case ms < 1000:
		return Cyan
	case ms < 3000:
		return Yellow
	default:
		return Red
	}
}

func UptimeColor(pct float64) string {
	switch {
	case pct >= 90:
		return Green
	case pct >= 70:
		return Yellow
	default:
		return Red
	}
}
