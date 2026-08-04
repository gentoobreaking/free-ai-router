package tui

import (
	"fmt"
	"strings"

	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

type RenderOptions struct {
	Models         []*models.Model
	SelectedIndex  int
	SearchQuery    string
	SearchActive   bool
	TotalCount     int
	SortKey        string
	SortReverse    bool
	TierFilter     string
	ProviderFilter string
	CodingOnly     bool
	IntervalMs     int
	Width          int
	Height         int
}

type Renderer struct {
	builder strings.Builder
}

func NewRenderer() *Renderer {
	return &Renderer{}
}

func (r *Renderer) Render(opts *RenderOptions) string {
	r.builder.Reset()
	r.builder.WriteString(CursorHome())

	r.renderHeader(opts)
	r.renderTable(opts)
	r.renderFooter(opts)

	return r.builder.String()
}

func (r *Renderer) renderHeader(opts *RenderOptions) {
	width := opts.Width
	if width < 60 {
		width = 60
	}

	r.builder.WriteString(BorderRow(width - 2))
	r.builder.WriteString("\n")

	r.builder.WriteString("│")
	r.builder.WriteString(Bold)
	r.builder.WriteString(" freemodel-router ")
	r.builder.WriteString(Reset)

	if opts.CodingOnly {
		r.builder.WriteString(ProviderTag("ready"))
		r.builder.WriteString(" ")
	}

	remaining := width - 2 - len(" freemodel-router ") - 2
	if opts.CodingOnly {
		remaining -= len(ProviderTag("ready")) + 1
	}
	r.builder.WriteString(strings.Repeat("─", remaining))
	r.builder.WriteString("│\n")

	r.builder.WriteString("│")
	r.builder.WriteString(" Model Search  ")
	r.builder.WriteString("/" + opts.SearchQuery)
	spaces := width - 2 - len(" Model Search  ") - len("/"+opts.SearchQuery) - len("  ") - len(fmt.Sprintf("%d/%d models", len(opts.Models), opts.TotalCount))
	if spaces < 1 {
		spaces = 1
	}
	r.builder.WriteString(strings.Repeat(" ", spaces))
	r.builder.WriteString(fmt.Sprintf("%d/%d models", len(opts.Models), opts.TotalCount))
	r.builder.WriteString(" │\n")

	if opts.SelectedIndex >= 0 && opts.SelectedIndex < len(opts.Models) {
		m := opts.Models[opts.SelectedIndex]
		r.builder.WriteString("│ Selected: ")
		r.builder.WriteString(Color(m.ID, BrightCyan))
		r.builder.WriteString("  ")
		r.builder.WriteString(fmt.Sprintf("SWE:%.1f%%", m.QualityScore*100))
		if hasTag(m.Tags, "coding") {
			r.builder.WriteString("  ")
			r.builder.WriteString(Color("Code:✓", Green))
		}
		padding := width - 2 - len("│ Selected: ") - len(Color(m.ID, BrightCyan)) - len("  ") - len(fmt.Sprintf("SWE:%.1f%%", m.QualityScore*100))
		if hasTag(m.Tags, "coding") {
			padding -= len("  ") + len(Color("Code:✓", Green))
		}
		if padding > 0 {
			r.builder.WriteString(strings.Repeat(" ", padding))
		}
		r.builder.WriteString("│\n")
	}

	r.builder.WriteString(SeparatorRow(width - 2))
	r.builder.WriteString("\n")
}

func (r *Renderer) renderTable(opts *RenderOptions) {
	width := opts.Width
	if width < 60 {
		width = 60
	}

	r.builder.WriteString(BorderRow(width - 2))
	r.builder.WriteString("\n")

	header := fmt.Sprintf("│ %-4s │ %-6s │ %-13s │ %-34s │ %-7s │ %-6s │ %8s │ %8s │ %6s │ %-16s │\n",
		"#", "Tier", "Provider", "Model", "Ctx", "Bench", "Avg", "Lat", "Up%", "Verdict")
	r.builder.WriteString(Bold)
	r.builder.WriteString(header)
	r.builder.WriteString(Reset)

	r.builder.WriteString(SeparatorRow(width - 2))
	r.builder.WriteString("\n")

	visibleRows := opts.Height - 14
	if visibleRows < 5 {
		visibleRows = 5
	}

	for i := 0; i < len(opts.Models) && i < visibleRows; i++ {
		m := opts.Models[i]
		marker := " "
		if i == opts.SelectedIndex {
			marker = ">"
		}

		verdict := ping.GetVerdict(m)
		bench := ""
		if m.QualityScore > 0 {
			bench = fmt.Sprintf("%.2f", m.QualityScore)
		}

		row := fmt.Sprintf("│ %s%-3d │ %s │ %-13s │ %-34s │ %-7s │ %-6s │ %8.0f │ %8.0f │ %5.1f%% │ %s │\n",
			marker,
			i+1,
			coloredCell(m.Tier, 6, TierColor(m.Tier)),
			truncateStr(m.Provider, 13),
			truncateStr(m.Label, 34),
			truncateStr(m.Context, 7),
			bench,
			m.AvgLatency,
			m.LatestPing,
			m.Uptime,
			coloredCell(verdict, 16, verdictColor(verdict)))

		if !opts.CodingOnly || hasTag(m.Tags, "coding") {
			r.builder.WriteString(row)
		} else {
			r.builder.WriteString(Dim)
			r.builder.WriteString(row)
			r.builder.WriteString(Reset)
		}
	}

	r.builder.WriteString(BorderRowBottom(width - 2))
	r.builder.WriteString("\n")
}

func (r *Renderer) renderFooter(opts *RenderOptions) {
	width := opts.Width
	if width < 60 {
		width = 60
	}

	r.builder.WriteString(SeparatorRow(width - 2))
	r.builder.WriteString("\n")

	r.builder.WriteString("│")
	r.builder.WriteString(Dim)
	if opts.SearchActive {
		r.builder.WriteString(" SEARCHING — type to filter, Enter: configure, ESC: clear ")
	} else {
		r.builder.WriteString(" ↑↓/jk:nav  PgUp/PgDn:page  /:search  Enter:configure  A:key  P:settings  ?:help  q:quit")
	}
	r.builder.WriteString(Reset)
	padding := width - 2 - len("│")
	if opts.SearchActive {
		padding = width - 2 - len("│ SEARCHING — type to filter, Enter: configure, ESC: clear ")
	} else {
		padding = width - 2 - len("│ ↑↓/jk:nav  PgUp/PgDn:page  /:search  Enter:configure  A:key  P:settings  ?:help  q:quit")
	}
	if padding > 0 {
		r.builder.WriteString(strings.Repeat(" ", padding))
	}
	r.builder.WriteString("│\n")

	sortLine := fmt.Sprintf(" sort:%s%s  tier:%s  provider:%s  interval:%dms  codingOnly:%v",
		opts.SortKey,
		map[bool]string{true: "↓", false: "↑"}[opts.SortReverse],
		opts.TierFilter,
		opts.ProviderFilter,
		opts.IntervalMs,
		opts.CodingOnly)
	r.builder.WriteString("│")
	r.builder.WriteString(Dim)
	r.builder.WriteString(sortLine)
	r.builder.WriteString(Reset)
	padding = width - 2 - len("│") - len(sortLine)
	if padding > 0 {
		r.builder.WriteString(strings.Repeat(" ", padding))
	}
	r.builder.WriteString("│\n")

	r.builder.WriteString(BorderRowBottom(width - 2))
	r.builder.WriteString("\n")
}

func coloredCell(text string, width int, color string) string {
	truncated := truncate(text, width)
	if len(truncated) < width {
		truncated += strings.Repeat(" ", width-len(truncated))
	}
	return color + truncated + Reset
}

func verdictColor(v string) string {
	switch v {
	case "Perfect", "Normal":
		return Color(v, Green)
	case "Slow":
		return Color(v, Yellow)
	case "Very Slow", "Unusable":
		return Color(v, Red)
	case "Overloaded":
		return Color(v, BrightYellow)
	case "Unstable", "Not Active":
		return Color(v, Red)
	default:
		return Color(v, Dim)
	}
}

func truncateStr(s string, max int) string {
	return truncate(s, max)
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func RenderSettings(providers []SettingsProvider) string {
	var b strings.Builder
	b.WriteString(CursorHome())
	b.WriteString(Bold + "free-router Settings" + Reset + "\n")
	b.WriteString(Dim + "  ↑↓:navigate  Enter:edit key  Space:toggle  T:test  D:delete  ESC/Q:back" + Reset + "\n\n")

	for _, p := range providers {
		state := "[OFF]"
		color := Dim
		if p.Enabled {
			state = "[ON]"
			color = Green
		}
		key := "(no key)"
		if p.Key != "" {
			key = maskKey(p.Key)
		}
		status := ""
		if p.TestStatus != "" {
			status = p.TestStatus
		}
		b.WriteString(fmt.Sprintf("  %s %s %-22s %-20s %s\n",
			Color(state, color),
			Color(p.Name, BrightCyan),
			p.Name,
			key,
			status))
	}
	return b.String()
}

type SettingsProvider struct {
	Name       string
	Enabled    bool
	Key        string
	TestStatus string
}

func maskKey(key string) string {
	if len(key) <= 6 {
		return "****"
	}
	return key[:4] + "...****"
}

func RenderHelp() string {
	var b strings.Builder
	b.WriteString(CursorHome())
	b.WriteString(Bold + "freemodel-router Help" + Reset + "\n\n")

	b.WriteString(Bold + "Keyboard Shortcuts\n" + Reset)
	shortcuts := [][2]string{
		{"↑↓ / j k", "Navigate models"},
		{"PgUp / PgDn", "Page up/down"},
		{"g / G", "Jump to top / bottom"},
		{"/", "Toggle search (Enter configures target, ESC clears)"},
		{"Enter", "Configure current model for a target agent"},
		{"A", "Quick API key add/change"},
		{"R", "Edit API key for rejected provider"},
		{"P", "Settings screen"},
		{"T", "Cycle tier filter: All → S+ → S → A+ → ..."},
		{"C", "Toggle coding-only filter"},
		{"W / X", "Decrease / increase ping interval"},
		{"N", "Cycle provider filter"},
		{"0-9", "Sort by column (press again to reverse)"},
		{"?", "Help overlay"},
		{"q / Ctrl+C", "Quit"},
	}
	for _, s := range shortcuts {
		b.WriteString(fmt.Sprintf("  %-24s %s\n", s[0], s[1]))
	}

	b.WriteString("\n" + Bold + "Sort Columns\n" + Reset)
	cols := [][2]string{
		{"0", "Priority (default): status → tier → avg → uptime → provider → model"},
		{"1", "Tier"},
		{"2", "Provider"},
		{"3", "Model"},
		{"4", "Avg latency (lowest first)"},
		{"5", "Latest ping (fastest first)"},
		{"6", "Uptime % (highest first)"},
		{"7", "Context window (smallest first)"},
		{"8", "Verdict (best to worst)"},
		{"9", "Intelligence (highest first)"},
	}
	for _, c := range cols {
		b.WriteString(fmt.Sprintf("  %-24s %s\n", c[0], c[1]))
	}

	b.WriteString("\n" + Bold + "SWE-bench Tiers\n" + Reset)
	tiers := [][2]string{
		{"S+", ">= 70% — Elite frontier"},
		{"S", "60-70% — Excellent"},
		{"A+", "50-60% — Great"},
		{"A", "40-50% — Good"},
		{"A-", "35-40% — Decent"},
		{"B+", "30-35% — Average"},
		{"B", "20-30% — Below average"},
		{"C", "< 20% — Lightweight / edge"},
	}
	for _, t := range tiers {
		b.WriteString(fmt.Sprintf("  %-24s %s\n", Color(t[0], TierColor(t[0])), t[1]))
	}

	b.WriteString("\n" + Bold + "Verdicts\n" + Reset)
	verdicts := [][2]string{
		{"✓ Perfect", "Avg < 400ms"},
		{"✓ Normal", "Avg < 1000ms"},
		{"x Slow", "Avg < 3000ms"},
		{"x Very Slow", "Avg < 5000ms"},
		{"x Unusable", "Avg >= 5000ms"},
		{"x Overloaded", "HTTP 429"},
		{"x Unstable", "Was up, now failing"},
		{"x Not Active", "Never responded"},
		{"- Pending", "Waiting for first success"},
	}
	for _, v := range verdicts {
		b.WriteString(fmt.Sprintf("  %-24s %s\n", v[0], v[1]))
	}

	b.WriteString("\n  q: back\n")
	return b.String()
}
