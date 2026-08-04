package tui

import (
	"strings"
	"testing"

	"github.com/freemodel/router/internal/models"
)

func TestRenderLayoutContainsKeyElements(t *testing.T) {
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

	out := RenderLayout(opts)

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
	if strings.Contains(out, "\x00") {
		t.Error("output should not contain null bytes")
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

func TestRenderSettings(t *testing.T) {
	providers := []SettingsProvider{
		{Name: "openai", Enabled: true, Key: "sk-test"},
		{Name: "anthropic", Enabled: true, Key: "sk-ant-test"},
	}
	out := RenderSettings(providers)
	if !strings.Contains(out, "openai") {
		t.Error("settings should contain provider name")
	}
	if !strings.Contains(out, "anthropic") {
		t.Error("settings should contain provider name")
	}
}

func TestRenderLayoutCodingOnlyFilter(t *testing.T) {
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

	out := RenderLayout(opts)

	if !strings.Contains(out, "Coding Model") {
		t.Error("coding model should appear when coding-only filter is on")
	}
}

func TestRenderLayoutFooter(t *testing.T) {
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
	out := RenderLayout(opts)
	if !strings.Contains(out, "q:quit") {
		t.Error("footer should show keybindings")
	}
}