package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

var (
	headerStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	tableStyle        = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	footerStyle       = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240")).Padding(0, 1)
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("208"))
	dimStyle          = lipgloss.NewStyle().Faint(true)
	cyanStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("45"))
	greenStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	yellowStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	redStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	brightGreen       = lipgloss.NewStyle().Foreground(lipgloss.Color("84"))
	brightCyan        = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	tierSPlus         = lipgloss.NewStyle().Foreground(lipgloss.Color("84"))
	tierS             = lipgloss.NewStyle().Foreground(lipgloss.Color("84"))
	tierAPlus         = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	tierA             = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	tierAMinus        = lipgloss.NewStyle().Foreground(lipgloss.Color("51"))
	tierBPlus         = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	tierB             = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	tierC             = lipgloss.NewStyle().Faint(true)
	verdictPerfect    = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	verdictNormal     = lipgloss.NewStyle().Foreground(lipgloss.Color("46"))
	verdictSlow       = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	verdictVerySlow   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	verdictUnusable   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	verdictOverloaded = lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	verdictUnstable   = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	verdictNotActive  = lipgloss.NewStyle().Faint(true)
	verdictPending    = lipgloss.NewStyle().Faint(true)
	dimText           = lipgloss.NewStyle().Faint(true)
	boldText          = lipgloss.NewStyle().Bold(true)
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

func RenderLayout(opts *RenderOptions) string {
	return lipgloss.JoinVertical(
		lipgloss.Left,
		renderHeader(opts),
		renderTable(opts),
		renderFooter(opts),
	)
}

func renderHeader(opts *RenderOptions) string {
	width := opts.Width
	if width < 60 {
		width = 60
	}

	title := titleStyle.Render(" freemodel-router ")
	tag := ""
	if opts.CodingOnly {
		tag = lipgloss.NewStyle().Background(lipgloss.Color("22")).Render(" READY ") + " "
	}

	headerContent := title + tag + "  Model Search  /" + opts.SearchQuery + "  " +
		fmt.Sprintf("%d/%d models", len(opts.Models), opts.TotalCount)

	header := lipgloss.NewStyle().
		Width(width).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Render(headerContent)

	selected := ""
	if opts.SelectedIndex >= 0 && opts.SelectedIndex < len(opts.Models) {
		m := opts.Models[opts.SelectedIndex]
		selected = fmt.Sprintf("│ Selected: %s  SWE:%.1f%%",
			cyanStyle.Render(m.ID), m.QualityScore*100)
		if hasTag(m.Tags, "coding") {
			selected += "  " + greenStyle.Render("Code:✓")
		}
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		selected,
	)
}

func renderTable(opts *RenderOptions) string {
	width := opts.Width
	if width < 60 {
		width = 60
	}

	var rows []string
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

		row := fmt.Sprintf("%s %-3d │ %s │ %-13s │ %-34s │ %-7s │ %-6s │ %8s │ %8s │ %6s │ %s │",
			marker,
			i+1,
			renderTier(m.Tier),
			m.Provider,
			truncateStr(m.Label, 34),
			m.Context,
			bench,
			fmt.Sprintf("%.0f", m.AvgLatency),
			fmt.Sprintf("%.0f", m.LatestPing),
			fmt.Sprintf("%.1f%%", m.Uptime),
			renderVerdict(verdict),
		)

		if !opts.CodingOnly || hasTag(m.Tags, "coding") {
			rows = append(rows, row)
		} else {
			rows = append(rows, dimStyle.Render(row))
		}
	}

	header := boldText.Render("│ #    │ Tier   │ Provider      │ Model                              │ Ctx    │ Bench  │      Avg │      Lat │    Up% │ Verdict          │")
	sep := lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Render("")

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		sep,
		strings.Join(rows, "\n"),
	)
}

func renderFooter(opts *RenderOptions) string {
	width := opts.Width
	if width < 60 {
		width = 60
	}

	var helpText string
	if opts.SearchActive {
		helpText = " SEARCHING — type to filter, Enter: configure, ESC: clear "
	} else {
		helpText = " ↑↓/jk:nav  PgUp/PgDn:page  /:search  Enter:configure  A:key  P:settings  ?:help  q:quit"
	}

	sortLine := fmt.Sprintf(" sort:%s%s  tier:%s  provider:%s  interval:%dms  codingOnly:%v",
		opts.SortKey,
		map[bool]string{true: "↓", false: "↑"}[opts.SortReverse],
		opts.TierFilter,
		opts.ProviderFilter,
		opts.IntervalMs,
		opts.CodingOnly)

	return lipgloss.JoinVertical(
		lipgloss.Left,
		dimStyle.Render("│ "+helpText+" │"),
		dimStyle.Render("│ "+sortLine+" │"),
	)
}

func renderTier(tier string) string {
	switch tier {
	case "S+", "S":
		return tierSPlus.Render(tier)
	case "A+", "A":
		return tierAPlus.Render(tier)
	case "A-", "B+":
		return tierAMinus.Render(tier)
	case "B":
		return tierB.Render(tier)
	default:
		return tierC.Render(tier)
	}
}

func renderVerdict(v string) string {
	switch v {
	case "Perfect", "Normal":
		return verdictPerfect.Render(v)
	case "Slow":
		return verdictSlow.Render(v)
	case "Very Slow", "Unusable":
		return verdictVerySlow.Render(v)
	case "Overloaded":
		return verdictOverloaded.Render(v)
	case "Unstable", "Not Active":
		return verdictUnstable.Render(v)
	default:
		return verdictPending.Render(v)
	}
}

func FilterModels(list []*models.Model, codingOnly bool, tier, provider, query string) []*models.Model {
	result := make([]*models.Model, 0, len(list))
	for _, m := range list {
		if codingOnly && !hasTag(m.Tags, "coding") {
			continue
		}
		if tier != "" && tier != "All" && m.Tier != tier {
			continue
		}
		if provider != "" && provider != "All" && m.Provider != provider {
			continue
		}
		if query != "" {
			lower := stringsToLower(query)
			if !containsLower(m.ID, lower) && !containsLower(m.Label, lower) && !containsLower(m.Provider, lower) {
				continue
			}
		}
		result = append(result, m)
	}
	return result
}

func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	// Find the last full rune within the limit to avoid breaking
	// multi-byte characters.
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	return string(runes[:max])
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func stringsToLower(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}

func containsLower(s, sub string) bool {
	return len(s) >= len(sub) && strings.Index(s, sub) >= 0
}

func RenderSettings(providers []SettingsProvider) string {
	var b strings.Builder
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("free-router Settings"))
	b.WriteString("\n")
	b.WriteString(dimStyle.Render("  ↑↓:navigate  Enter:edit key  Space:toggle  T:test  D:delete  ESC/Q:back"))
	b.WriteString("\n\n")

	for _, p := range providers {
		state := "[OFF]"
		color := lipgloss.NewStyle().Faint(true)
		if p.Enabled {
			state = "[ON]"
			color = greenStyle
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
			color.Render(state),
			brightCyan.Render(p.Name),
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
	b.WriteString(lipgloss.NewStyle().Bold(true).Render("freemodel-router Help"))
	b.WriteString("\n\n")

	b.WriteString(lipgloss.NewStyle().Bold(true).Render("Keyboard Shortcuts"))
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

	b.WriteString("\n" + boldText.Render("Sort Columns"))
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

	b.WriteString("\n" + boldText.Render("SWE-bench Tiers"))
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
		b.WriteString(fmt.Sprintf("  %-24s %s\n", renderTier(t[0]), t[1]))
	}

	b.WriteString("\n" + boldText.Render("Verdicts"))
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
