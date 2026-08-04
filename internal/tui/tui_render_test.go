package tui

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/freemodel/router/internal/models"
)

func TestRenderTableLayout(t *testing.T) {
	r := NewRenderer()

	models := []*models.Model{
		{ID: "nvidia/deepseek-ai/deepseek-v3.2", Label: "DeepSeek V3.2", Provider: "nvidia", Tier: "S+", Context: "128k", Status: "up", AvgLatency: 342, LatestPing: 350, Uptime: 100, Tags: []string{"coding"}},
		{ID: "openrouter/moonshotai/kimi-k2.5", Label: "Kimi K2.5", Provider: "openrouter", Tier: "S+", Context: "128k", Status: "up", AvgLatency: 410, LatestPing: 400, Uptime: 95, Tags: []string{"coding"}},
	}

	opts := &RenderOptions{
		Models:        models,
		SelectedIndex: 0,
		TotalCount:    2,
		SortKey:       "0",
		TierFilter:    "All",
		CodingOnly:    true,
		IntervalMs:    2000,
		Width:         120,
		Height:        40,
	}

	out := r.Render(opts)

	if !strings.Contains(out, "DeepSeek V3.2") {
		t.Error("rendered output should contain model name")
	}
	if !strings.Contains(out, "S+") {
		t.Error("rendered output should contain tier")
	}
	if !strings.Contains(out, "nvidia") {
		t.Error("rendered output should contain provider")
	}
	if !strings.Contains(out, "Verdict") {
		t.Error("rendered output should contain column headers")
	}
	if !strings.Contains(out, "342") {
		t.Error("rendered output should contain avg latency")
	}
	if !strings.Contains(out, "models") {
		t.Error("rendered output should contain model count")
	}
	if strings.Contains(out, "\x00") {
		t.Error("output should not contain null bytes")
	}
}

func TestRenderCodingOnlyDimming(t *testing.T) {
	r := NewRenderer()

	models := []*models.Model{
		{ID: "a/coding-model", Label: "Coding Model", Provider: "a", Tier: "S+", Status: "up", Tags: []string{"coding"}},
		{ID: "b/general-model", Label: "General Model", Provider: "b", Tier: "A+", Status: "up", Tags: []string{"general"}},
	}

	opts := &RenderOptions{
		Models:        models,
		SelectedIndex: 0,
		TotalCount:    2,
		SortKey:       "0",
		CodingOnly:    true,
		IntervalMs:    2000,
		Width:         120,
		Height:        40,
	}

	out := r.Render(opts)

	// Both should appear (general one dimmed), per spec §4.2
	if !strings.Contains(out, "Coding Model") || !strings.Contains(out, "General Model") {
		t.Error("both models should appear in TUI")
	}
}

func TestRenderFooter(t *testing.T) {
	r := NewRenderer()
	opts := &RenderOptions{
		Models:         nil,
		SelectedIndex:  -1,
		SortKey:        "0",
		SortReverse:    false,
		TierFilter:     "All",
		ProviderFilter: "All",
		CodingOnly:     true,
		IntervalMs:     2000,
		Width:          120,
		Height:         40,
	}
	out := r.Render(opts)
	if !strings.Contains(out, "q:quit") {
		t.Error("footer should show keybindings")
	}
}

func TestRenderHelp(t *testing.T) {
	out := RenderHelp()
	if !strings.Contains(out, "Keyboard Shortcuts") {
		t.Error("help should list keyboard shortcuts")
	}
	if !strings.Contains(out, "C") || !strings.Contains(out, "coding-only") {
		t.Error("help should document coding-only filter")
	}
	if !strings.Contains(out, "Tiers") {
		t.Error("help should list tier definitions")
	}
	if !strings.Contains(out, "Verdicts") {
		t.Error("help should list verdicts")
	}
}

func TestStatusDot(t *testing.T) {
	if !strings.Contains(StatusDot("up", "200"), "*") {
		t.Error("up should show *")
	}
	if !strings.Contains(StatusDot("noauth", "401"), "!") {
		t.Error("noauth should show !")
	}
	if !strings.Contains(StatusDot("ratelimit", "429"), "~") {
		t.Error("ratelimit should show ~")
	}
	if !strings.Contains(StatusDot("down", "500"), "x") {
		t.Error("down should show x")
	}
	if !strings.Contains(StatusDot("pending", ""), ".") {
		t.Error("pending should show .")
	}
}

func TestRenderCellTruncation(t *testing.T) {
	cell := RenderCell(TableCell{Text: "This is a very long model name that should be truncated", Width: 10})
	if got := utf8.RuneCountInString(stripANSI(cell)); got != 10 {
		t.Errorf("cell should be truncated to 10 chars, got %d: %q", got, stripANSI(cell))
	}
}

func TestProviderTag(t *testing.T) {
	if !strings.Contains(ProviderTag("ready"), "READY") {
		t.Error("ready tag should show READY")
	}
	if !strings.Contains(ProviderTag("nokey"), "NO KEY") {
		t.Error("nokey tag should show NO KEY")
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r >= 'A' && r <= 'z' || r == '[' || r == 'm' || r == 'H' || r == 'h' || r == 'l' || r == 'J' {
				if r == 'm' || r == 'H' || r == 'h' || r == 'l' || r == 'J' {
					inEscape = false
				}
				continue
			}
			if r == ';' || r >= '0' && r <= '9' {
				continue
			}
			inEscape = false
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
