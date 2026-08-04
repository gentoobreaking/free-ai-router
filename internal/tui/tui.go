package tui

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"golang.org/x/term"

	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
)

const renderThrottle = 33 * time.Millisecond
const liveUpdateThrottle = 300 * time.Millisecond

type TUI struct {
	engine      *ping.Engine
	registry    *models.Registry
	renderer    *Renderer
	input       *Input
	models      []*models.Model
	selected    int
	searchQuery string
	sortKey     string
	sortReverse bool
	tierFilter  string
	providerFilter string
	codingOnly  bool
	intervalMs  int
	showSettings bool
	showHelp    bool
	width       int
	height      int
	blurred     bool
	renderPending bool
	lastRender  time.Time
	lastLiveUpdate time.Time
	quit        bool
	paused      bool
	pauseUntil  time.Time
	configPath  string
}

func New(cfg *Config) *TUI {
	t := &TUI{
		renderer:    NewRenderer(),
		input:       NewInput(),
		sortKey:     "0",
		intervalMs:  2000,
		codingOnly:  true,
		width:       120,
		height:      40,
		lastRender:  time.Now(),
		lastLiveUpdate: time.Now(),
	}
	t.engine = ping.NewEngine(t.onPingUpdate)
	return t
}

type Config struct {
	ScrollSortPauseMs int
	ForceClear        bool
	ConfigPath        string
}

func (t *TUI) SetRegistry(registry *models.Registry) {
	t.registry = registry
}

func (t *TUI) Run() error {
	fd := int(os.Stdin.Fd())
	oldState, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("failed to enter raw mode: %w", err)
	}
	defer term.Restore(fd, oldState)

	fmt.Print(EnterAltScreen() + EnableFocus() + CursorHide())
	defer fmt.Print(ExitAltScreen() + DisableFocus() + CursorShow())

	t.resize()

	sigCh := make(chan os.Signal, 4)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGWINCH)
	defer signal.Stop(sigCh)

	t.input.Start()
	t.engine.SetModels(t.registry.GetAll())
	t.engine.Start()

	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()

	for !t.quit {
		select {
		case ev := <-t.input.Channel():
			t.handleInput(ev)
		case <-sigCh:
			t.quit = true
		case <-tick.C:
			t.tick()
		}
	}

	t.engine.Stop()
	return nil
}

func (t *TUI) resize() {
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		t.width = w
		t.height = h
	}
	t.renderPending = true
}

func (t *TUI) onPingUpdate() {
	t.renderPending = true
}

func (t *TUI) tick() {
	now := time.Now()

	if t.paused && now.After(t.pauseUntil) {
		t.paused = false
	}

	if !t.blurred && t.renderPending {
		if now.Sub(t.lastRender) >= renderThrottle {
			t.render()
			t.lastRender = now
			t.renderPending = false
		}
	}

	if t.blurred {
		t.renderPending = true
	}
}

func (t *TUI) render() {
	if t.registry == nil {
		return
	}

	all := t.registry.GetAll()

	filtered := filterModels(all, t.codingOnly, t.tierFilter, t.providerFilter, t.searchQuery)

	models.SortModels(filtered, t.sortKey, t.sortReverse)

	if t.selected >= len(filtered) {
		t.selected = len(filtered) - 1
	}
	if t.selected < 0 {
		t.selected = 0
	}

	if t.showHelp {
		fmt.Print(RenderHelp())
		return
	}

	if t.showSettings {
		fmt.Print(RenderSettings(nil))
		return
	}

	opts := &RenderOptions{
		Models:         filtered,
		SelectedIndex:  t.selected,
		SearchQuery:    t.searchQuery,
		TotalCount:     len(all),
		SortKey:        t.sortKey,
		SortReverse:    t.sortReverse,
		TierFilter:     t.tierFilter,
		ProviderFilter: t.providerFilter,
		CodingOnly:     t.codingOnly,
		IntervalMs:     t.intervalMs,
		Width:          t.width,
		Height:         t.height,
	}

	out := t.renderer.Render(opts)
	fmt.Print(out)
}

func filterModels(list []*models.Model, codingOnly bool, tier, provider, query string) []*models.Model {
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

func (t *TUI) handleInput(ev InputEvent) {
	if t.showHelp {
		if ev.Key == KeyEsc || ev.Key == KeyRune && ev.Rune == 'q' {
			t.showHelp = false
			t.renderPending = true
		}
		return
	}

	if t.showSettings {
		t.handleSettingsInput(ev)
		return
	}

	switch ev.Key {
	case KeyUp:
		t.navigate(-1)
	case KeyDown:
		t.navigate(1)
	case KeyPageUp:
		t.navigate(-10)
	case KeyPageDown:
		t.navigate(10)
	case KeyHome:
		t.selected = 0
	case KeyEnd:
		t.selected = t.visibleCount()
	case KeyEnter:
		t.openTargetPicker()
	case KeyRune:
		switch ev.Rune {
		case 'q':
			t.quit = true
		case 'j':
			t.navigate(1)
		case 'k':
			t.navigate(-1)
		case 'g':
			t.selected = 0
			t.renderPending = true
		case 'G':
			t.selected = t.visibleCount()
			t.renderPending = true
		case '/':
			t.toggleSearch()
		case 'C':
			t.codingOnly = !t.codingOnly
			t.renderPending = true
		case 'T':
			t.cycleTierFilter()
		case 'N':
			t.cycleProviderFilter()
		case 'W':
			t.changeInterval(-1)
		case 'X':
			t.changeInterval(1)
		case 'P':
			t.showSettings = true
			t.renderPending = true
		case '?':
			t.showHelp = true
			t.renderPending = true
		case '0':
			t.setSort("0")
		case '1':
			t.setSort("tier")
		case '2':
			t.setSort("provider")
		case '3':
			t.setSort("model")
		case '4':
			t.setSort("avg")
		case '5':
			t.setSort("lat")
		case '6':
			t.setSort("uptime")
		case '7':
			t.setSort("context")
		case '8':
			t.setSort("verdict")
		case '9':
			t.setSort("intel")
		}
	case KeyCtrlC:
		t.quit = true
	case KeyFocusOut:
		t.blurred = true
	case KeyFocusIn:
		t.blurred = false
		t.renderPending = true
	}
}

func (t *TUI) navigate(delta int) {
	t.paused = true
	t.pauseUntil = time.Now().Add(1500 * time.Millisecond)
	t.selected += delta
	if t.selected < 0 {
		t.selected = 0
	}
	t.renderPending = true
}

func (t *TUI) visibleCount() int {
	return t.height - 10
}

func (t *TUI) toggleSearch() {
	// Search mode handled by main loop reading runes; simplified
	t.renderPending = true
}

func (t *TUI) cycleTierFilter() {
	tiers := []string{"All", "S+", "S", "A+", "A", "A-", "B+", "B", "C"}
	next := 0
	for i, v := range tiers {
		if v == t.tierFilter {
			next = (i + 1) % len(tiers)
			break
		}
	}
	t.tierFilter = tiers[next]
	t.renderPending = true
}

func (t *TUI) cycleProviderFilter() {
	providers := []string{"All", "nvidia", "openrouter", "groq", "cerebras", "opencode", "googleai"}
	next := 0
	for i, v := range providers {
		if v == t.providerFilter {
			next = (i + 1) % len(providers)
			break
		}
	}
	t.providerFilter = providers[next]
	t.renderPending = true
}

func (t *TUI) changeInterval(dir int) {
	if dir < 0 {
		if t.intervalMs > 500 {
			t.intervalMs -= 500
		}
	} else {
		t.intervalMs += 500
		if t.intervalMs > 10000 {
			t.intervalMs = 10000
		}
	}
	t.engine.SetInterval(time.Duration(t.intervalMs) * time.Millisecond)
	t.renderPending = true
}

func (t *TUI) setSort(key string) {
	if t.sortKey == key {
		t.sortReverse = !t.sortReverse
	} else {
		t.sortKey = key
		t.sortReverse = false
	}
	t.renderPending = true
}

func (t *TUI) openTargetPicker() {
	t.renderPending = true
}

func (t *TUI) handleSettingsInput(ev InputEvent) {
	if ev.Key == KeyEsc || ev.Key == KeyRune && (ev.Rune == 'q' || ev.Rune == 'Q') {
		t.showSettings = false
		t.renderPending = true
	}
}

func containsLower(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func stringsToLower(s string) string {
	return lowerString(s)
}

func lowerString(s string) string {
	b := []byte(s)
	for i := 0; i < len(b); i++ {
		if b[i] >= 'A' && b[i] <= 'Z' {
			b[i] += 32
		}
	}
	return string(b)
}
