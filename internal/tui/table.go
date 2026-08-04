package tui

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/providers"
	"github.com/freemodel/router/internal/targets"
)

const defaultScrollSortPause = 1500 * time.Millisecond

// TableScreen manages the main model-table view: navigation, sorting, filtering.
type TableScreen struct {
	registry       *models.Registry
	cfg            *config.Config
	engine         *ping.Engine
	selected       int
	searchQuery    string
	searchActive   bool
	sortKey        string
	sortReverse    bool
	tierFilter     string
	providerFilter string
	codingOnly     bool
	intervalMs     int
	paused         bool
	pauseUntil     time.Time
	pauseMs        time.Duration
	width          int
	height         int
}

func NewTableScreen() *TableScreen {
	return &TableScreen{
		sortKey:    "0",
		intervalMs: 2000,
		codingOnly: true,
		pauseMs:    defaultScrollSortPause,
	}
}

func (t *TableScreen) SetRegistry(reg *models.Registry) { t.registry = reg }
func (t *TableScreen) SetConfig(cfg *config.Config) {
	t.cfg = cfg
	t.pauseMs = time.Duration(cfg.UI.ScrollSortPauseMs) * time.Millisecond
}
func (t *TableScreen) SetEngine(engine *ping.Engine) { t.engine = engine }
func (t *TableScreen) SetSize(w, h int)              { t.width = w; t.height = h }

// HandleKey processes a key event and returns a command plus an optional screen
// transition name ("settings","picker","help","") or "quit".
func (t *TableScreen) HandleKey(msg tea.KeyMsg) (tea.Cmd, string) {
	if t.searchActive {
		return t.handleSearchInput(msg)
	}
	return t.handleTableInput(msg)
}

func (t *TableScreen) handleTableInput(msg tea.KeyMsg) (tea.Cmd, string) {
	switch msg.String() {
	case "up", "k":
		t.navigate(-1)
	case "down", "j":
		t.navigate(1)
	case "pageup":
		t.navigate(-10)
	case "pagedown":
		t.navigate(10)
	case "g":
		t.selected = 0
	case "G":
		t.selected = t.visibleCount() - 1
	case "enter":
		return nil, "picker"
	case "/":
		t.searchActive = true
	case "c", "C":
		t.codingOnly = !t.codingOnly
	case "t", "T":
		t.cycleTierFilter()
	case "n", "N":
		t.cycleProviderFilter()
	case "w":
		t.changeInterval(-500)
	case "x":
		t.changeInterval(500)
	case "p", "P":
		return nil, "settings"
	case "?":
		return nil, "help"
	}
	return nil, ""
}

func (t *TableScreen) handleSearchInput(msg tea.KeyMsg) (tea.Cmd, string) {
	switch msg.String() {
	case "enter":
		t.searchActive = false
		return nil, "picker"
	case "esc":
		t.searchQuery = ""
		t.searchActive = false
	}
	return nil, ""
}

func (t *TableScreen) navigate(delta int) {
	t.paused = true
	t.pauseUntil = time.Now().Add(t.pauseMs)
	t.selected += delta
	if t.selected < 0 {
		t.selected = 0
	}
}

func (t *TableScreen) visibleCount() int {
	if t.height < 10 {
		return 5
	}
	return t.height - 10
}

func (t *TableScreen) cycleTierFilter() {
	tiers := []string{"All", "S+", "S", "A+", "A", "A-", "B+", "B", "C"}
	next := 0
	for i, v := range tiers {
		if v == t.tierFilter {
			next = (i + 1) % len(tiers)
			break
		}
	}
	t.tierFilter = tiers[next]
}

func (t *TableScreen) cycleProviderFilter() {
	list := []string{"All"}
	seen := make(map[string]bool)
	for _, p := range providers.ProviderKeys() {
		list = append(list, p)
		seen[p] = true
	}
	if t.registry != nil {
		for _, model := range t.registry.Snapshot() {
			if !seen[model.Provider] {
				list = append(list, model.Provider)
				seen[model.Provider] = true
			}
		}
	}
	next := 0
	for i, v := range list {
		if v == t.providerFilter {
			next = (i + 1) % len(list)
			break
		}
	}
	t.providerFilter = list[next]
}

func (t *TableScreen) changeInterval(dir int) {
	t.intervalMs += dir
	if t.intervalMs < 500 {
		t.intervalMs = 500
	}
	if t.intervalMs > 10000 {
		t.intervalMs = 10000
	}
}

func (t *TableScreen) filteredModels() []*models.Model {
	return FilterModels(t.registry.Snapshot(), t.codingOnly, t.tierFilter, t.providerFilter, t.searchQuery)
}

// View returns the rendered table view.
func (t *TableScreen) View() string {
	return RenderLayout(&RenderOptions{
		Models:         t.filteredModels(),
		SelectedIndex:  t.selected,
		SearchQuery:    t.searchQuery,
		SearchActive:   t.searchActive,
		TotalCount:     len(t.registry.Snapshot()),
		SortKey:        t.sortKey,
		SortReverse:    t.sortReverse,
		TierFilter:     t.tierFilter,
		ProviderFilter: t.providerFilter,
		CodingOnly:     t.codingOnly,
		IntervalMs:     t.intervalMs,
		Width:          t.width,
		Height:         t.height,
	})
}

// SelectedModel returns the currently highlighted model, used by picker.
func (t *TableScreen) SelectedModel() *models.Model {
	snap := t.registry.Snapshot()
	if t.selected >= 0 && t.selected < len(snap) {
		return snap[t.selected]
	}
	return models.FindBestModel(snap)
}

// PickerTargets returns the standard target agent list.
func PickerTargets() []targets.Target {
	return []targets.Target{
		&targets.OpenCodeTarget{},
		&targets.OpenClawTarget{},
		&targets.HermesTarget{},
		&targets.PiTarget{},
	}
}
