package tui

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/term"

	"github.com/freemodel/router/internal/config"
	"github.com/freemodel/router/internal/models"
	"github.com/freemodel/router/internal/ping"
	"github.com/freemodel/router/internal/targets"
)

const renderThrottle = 33 * time.Millisecond
const defaultScrollSortPause = 1500 * time.Millisecond

type TUI struct {
	engine          *ping.Engine
	registry        *models.Registry
	renderer        *Renderer
	input           *Input
	models          []*models.Model
	selected        int
	searchQuery     string
	searchMode      bool
	sortKey         string
	sortReverse     bool
	tierFilter      string
	providerFilter  string
	codingOnly      bool
	intervalMs      int
	showSettings    bool
	settingsIndex   int
	settingsKeyEdit bool
	settingsKeyBuf  string
	settingsTestMsg string
	showHelp        bool
	width           int
	height          int
	blurred         bool
	renderPending   atomic.Bool
	lastRender      time.Time
	pauseMs         time.Duration
	quit            bool
	paused          bool
	pauseUntil      time.Time
	configPath      string
	cfg             *config.Config
	pickerOpen      bool
	pickerIndex     int
	pickerMsg       string
	pickerTargets   []targets.Target
}

func New(cfg *Config) *TUI {
	t := &TUI{
		renderer:   NewRenderer(),
		input:      NewInput(),
		sortKey:    "0",
		intervalMs: 2000,
		codingOnly: true,
		width:      120,
		height:     40,
		lastRender: time.Now(),
		pauseMs:    defaultScrollSortPause,
	}
	if cfg != nil && cfg.ScrollSortPauseMs > 0 {
		t.pauseMs = time.Duration(cfg.ScrollSortPauseMs) * time.Millisecond
	}
	t.engine = ping.NewEngine(t.onPingUpdate)
	return t
}

type Config struct {
	ScrollSortPauseMs int
}

func (t *TUI) SetRegistry(registry *models.Registry) {
	t.registry = registry
}

func (t *TUI) SetConfig(cfg *config.Config) {
	t.cfg = cfg
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
	setupSignals(sigCh)
	defer signal.Stop(sigCh)

	t.input.Start()
	t.engine.SetModels(t.registry.GetAll())
	if t.cfg == nil || t.cfg.AutoPingEnabled {
		t.engine.Start()
	}

	tick := time.NewTicker(33 * time.Millisecond)
	defer tick.Stop()

	for !t.quit {
		select {
		case ev := <-t.input.Channel():
			t.handleInput(ev)
		case sig := <-sigCh:
			t.handleSignal(sig)
		case <-tick.C:
			t.tick()
		}
	}

	t.engine.Stop()
	return nil
}

// handleSignal dispatches process signals: SIGWINCH resizes the terminal
// view; everything else (SIGINT/SIGTERM) quits the TUI.
func (t *TUI) handleSignal(sig os.Signal) {
	handleSignal(t, sig)
}

func (t *TUI) resize() {
	if w, h, err := term.GetSize(int(os.Stdin.Fd())); err == nil {
		t.width = w
		t.height = h
	}
	t.renderPending.Store(true)
}

func (t *TUI) onPingUpdate() {
	t.renderPending.Store(true)
}

func (t *TUI) tick() {
	now := time.Now()

	if t.paused && now.After(t.pauseUntil) {
		t.paused = false
	}

	if !t.blurred && t.renderPending.Load() {
		if now.Sub(t.lastRender) >= renderThrottle {
			t.render()
			t.lastRender = now
			t.renderPending.Store(false)
		}
	}

	if t.blurred {
		t.renderPending.Store(true)
	}
}

func (t *TUI) render() {
	if t.registry == nil {
		return
	}

	all := t.registry.Snapshot()

	if t.pickerOpen {
		fmt.Print(t.renderTargetPicker())
		return
	}

	filtered := filterModels(all, t.codingOnly, t.tierFilter, t.providerFilter, t.searchQuery)

	models.SortModels(filtered, t.sortKey, t.sortReverse)

	t.models = filtered

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
		fmt.Print(RenderSettings(t.settingsProviders()))
		return
	}

	opts := &RenderOptions{
		Models:         filtered,
		SelectedIndex:  t.selected,
		SearchQuery:    t.searchQuery,
		SearchActive:   t.searchMode,
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
			t.renderPending.Store(true)
		}
		return
	}

	if t.pickerOpen {
		t.handlePickerInput(ev)
		return
	}

	if t.showSettings {
		t.handleSettingsInput(ev)
		return
	}

	if t.searchMode {
		t.handleSearchInput(ev)
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
	case KeyBackspace:
		if t.searchQuery != "" {
			t.searchQuery = t.searchQuery[:len(t.searchQuery)-1]
			t.renderPending.Store(true)
		}
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
			t.renderPending.Store(true)
		case 'G':
			t.selected = t.visibleCount()
			t.renderPending.Store(true)
		case '/':
			t.searchMode = true
			t.renderPending.Store(true)
		case 'C':
			t.codingOnly = !t.codingOnly
			t.renderPending.Store(true)
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
			t.settingsIndex = 0
			t.renderPending.Store(true)
		case '?':
			t.showHelp = true
			t.renderPending.Store(true)
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
		t.renderPending.Store(true)
	}
}

// handleSearchInput: live filter while typing; Enter exits search and opens
// the target picker; ESC clears the query and exits (spec §6.8).
func (t *TUI) handleSearchInput(ev InputEvent) {
	switch ev.Key {
	case KeyEnter:
		t.searchMode = false
		t.renderPending.Store(true)
		t.openTargetPicker()
	case KeyEsc:
		t.searchQuery = ""
		t.searchMode = false
		t.renderPending.Store(true)
	case KeyBackspace:
		if t.searchQuery != "" {
			t.searchQuery = t.searchQuery[:len(t.searchQuery)-1]
			t.renderPending.Store(true)
		}
	case KeyCtrlC:
		t.quit = true
	case KeyRune:
		if ev.Rune >= 32 && ev.Rune != '/' {
			t.searchQuery += string(ev.Rune)
			t.renderPending.Store(true)
		}
	}
}

// openTargetPicker opens the configure-for-agent modal (spec §6.14).
func (t *TUI) openTargetPicker() {
	t.pickerTargets = []targets.Target{
		&targets.OpenCodeTarget{},
		&targets.OpenClawTarget{},
		&targets.HermesTarget{},
		&targets.PiTarget{},
	}
	t.pickerIndex = 0
	t.pickerMsg = ""
	t.pickerOpen = true
	t.renderPending.Store(true)
}

func (t *TUI) handlePickerInput(ev InputEvent) {
	switch ev.Key {
	case KeyUp, KeyRune:
		if ev.Key == KeyRune {
			switch ev.Rune {
			case 'k':
				t.pickerIndex--
			case 'j':
				t.pickerIndex++
			case 'q', 'Q':
				t.pickerOpen = false
				t.renderPending.Store(true)
				return
			default:
				return
			}
		} else {
			t.pickerIndex--
		}
		if t.pickerIndex < 0 {
			t.pickerIndex = len(t.pickerTargets) - 1
		}
		t.renderPending.Store(true)
	case KeyDown:
		t.pickerIndex++
		if t.pickerIndex >= len(t.pickerTargets) {
			t.pickerIndex = 0
		}
		t.renderPending.Store(true)
	case KeyEnter:
		t.saveTargetConfig()
	case KeyEsc:
		t.pickerOpen = false
		t.renderPending.Store(true)
	case KeyCtrlC:
		t.quit = true
	}
}

func (t *TUI) saveTargetConfig() {
	if t.pickerIndex < 0 || t.pickerIndex >= len(t.pickerTargets) {
		return
	}
	var current *models.Model
	if t.selected >= 0 && t.selected < len(t.models) {
		current = t.models[t.selected]
	}
	if current == nil {
		current = models.FindBestModel(t.registry.Snapshot())
	}
	if current == nil {
		t.pickerMsg = "no model selected"
		t.renderPending.Store(true)
		return
	}

	target := t.pickerTargets[t.pickerIndex]
	if err := target.Write(current.ID); err != nil {
		t.pickerMsg = "failed: " + err.Error()
	} else {
		t.pickerMsg = "saved " + current.ID + " to " + target.Name()
		binary := targetBinary(target.Name())
		if binary != "" && targets.IsInstalled(binary) {
			launchTarget(binary)
			t.pickerMsg += "; launched " + binary
		}
	}
	t.pickerOpen = false
	t.renderPending.Store(true)
}

func targetBinary(name string) string {
	switch name {
	case "OpenCode":
		return "opencode"
	case "OpenClaw":
		return "openclaw"
	case "Hermes Agent":
		return "hermes"
	case "Pi Agent":
		return "pi"
	default:
		return ""
	}
}

func (t *TUI) renderTargetPicker() string {
	var b string
	b += fmt.Sprintf("%sConfigure for target agent%s\n\n", Bold, Reset)
	for i, target := range t.pickerTargets {
		marker := "  "
		row := fmt.Sprintf("%s %-16s %s", target.Name(), Dim+target.ConfigPath()+Reset, "")
		if i == t.pickerIndex {
			marker = "> "
			row = BrightCyan + target.Name() + Reset + "  " + Dim + target.ConfigPath() + Reset
		} else {
			row = fmt.Sprintf("%s %-16s %s", target.Name(), Dim+target.ConfigPath()+Reset, "")
		}
		b += fmt.Sprintf("%s%s\n", marker, row)
	}
	b += "\n" + Dim + "  ↑↓/jk: navigate  Enter: save  ESC/q: back" + Reset + "\n"
	if t.pickerMsg != "" {
		b += "\n  " + t.pickerMsg + "\n"
	}
	return CursorHome() + b
}

func (t *TUI) navigate(delta int) {
	t.paused = true
	t.pauseUntil = time.Now().Add(t.pauseMs)
	t.selected += delta
	if t.selected < 0 {
		t.selected = 0
	}
	t.renderPending.Store(true)
}

func (t *TUI) visibleCount() int {
	return t.height - 10
}

// settingsProviders builds the provider list for the settings screen from
// config + live registry state (spec §6.13).
func (t *TUI) settingsProviders() []SettingsProvider {
	providers := []SettingsProvider{
		{Name: "nvidia", Enabled: false},
		{Name: "groq", Enabled: false},
		{Name: "cerebras", Enabled: false},
		{Name: "openrouter", Enabled: false},
		{Name: "googleai", Enabled: false},
		{Name: "opencode", Enabled: false},
		{Name: "codestral", Enabled: false},
		{Name: "scaleway", Enabled: false},
		{Name: "kilocode", Enabled: false},
		{Name: "ollama", Enabled: false},
	}

	if t.cfg != nil {
		for i := range providers {
			name := providers[i].Name
			if pcfg, ok := t.cfg.Providers[name]; ok {
				providers[i].Enabled = pcfg.Enabled
			}
			if key := config.ResolveAPIKey(name, t.cfg); key != "" {
				providers[i].Key = key
			}
		}
	}

	if t.registry != nil {
		for _, m := range t.registry.Snapshot() {
			for i := range providers {
				if providers[i].Name == m.Provider && m.Status == "up" {
					providers[i].TestStatus = "up"
				}
			}
		}
	}

	if t.settingsTestMsg != "" {
		if t.settingsIndex >= 0 && t.settingsIndex < len(providers) {
			providers[t.settingsIndex].TestStatus = t.settingsTestMsg
		}
	}

	return providers
}

// handleSettingsInput: ↑↓/jk navigate, Space toggle enabled, Enter edit key,
// T test ping, D delete key, ESC/Q back (spec §6.13). Changes persist to
// config immediately.
func (t *TUI) handleSettingsInput(ev InputEvent) {
	providers := t.settingsProviders()

	if t.settingsKeyEdit {
		switch ev.Key {
		case KeyEnter:
			if t.cfg != nil {
				key := stringsTrimSpace(t.settingsKeyBuf)
				if key != "" {
					name := providers[t.settingsIndex].Name
					t.cfg.APIKeys[name] = key
					if pcfg, ok := t.cfg.Providers[name]; ok {
						pcfg.Enabled = true
						t.cfg.Providers[name] = pcfg
					} else {
						t.cfg.Providers[name] = config.ProviderConfig{Enabled: true}
					}
					_ = config.Save(t.cfg)
				}
			}
			t.settingsKeyEdit = false
			t.settingsKeyBuf = ""
			t.renderPending.Store(true)
		case KeyEsc:
			t.settingsKeyEdit = false
			t.settingsKeyBuf = ""
			t.renderPending.Store(true)
		case KeyBackspace:
			if t.settingsKeyBuf != "" {
				t.settingsKeyBuf = t.settingsKeyBuf[:len(t.settingsKeyBuf)-1]
				t.renderPending.Store(true)
			}
		case KeyRune:
			if ev.Rune >= 32 {
				t.settingsKeyBuf += string(ev.Rune)
				t.renderPending.Store(true)
			}
		}
		return
	}

	if ev.Key == KeyEsc || ev.Key == KeyRune && (ev.Rune == 'q' || ev.Rune == 'Q') {
		t.showSettings = false
		t.settingsTestMsg = ""
		t.renderPending.Store(true)
		return
	}

	switch ev.Key {
	case KeyUp, KeyRune:
		if ev.Key == KeyRune {
			switch ev.Rune {
			case 'k':
				if t.settingsIndex > 0 {
					t.settingsIndex--
					t.renderPending.Store(true)
				}
			case 'j':
				if t.settingsIndex < len(providers)-1 {
					t.settingsIndex++
					t.renderPending.Store(true)
				}
			case ' ':
				if t.cfg != nil && t.settingsIndex < len(providers) {
					name := providers[t.settingsIndex].Name
					pcfg, ok := t.cfg.Providers[name]
					if !ok {
						pcfg = config.ProviderConfig{}
					}
					pcfg.Enabled = !pcfg.Enabled
					t.cfg.Providers[name] = pcfg
					_ = config.Save(t.cfg)
					t.renderPending.Store(true)
				}
			case 'T':
				if t.settingsIndex < len(providers) {
					t.settingsTestMsg = t.testProviderPing(providers[t.settingsIndex].Name)
					t.renderPending.Store(true)
				}
			case 'D':
				if t.cfg != nil && t.settingsIndex < len(providers) {
					name := providers[t.settingsIndex].Name
					delete(t.cfg.APIKeys, name)
					if pcfg, ok := t.cfg.Providers[name]; ok {
						pcfg.Enabled = false
						t.cfg.Providers[name] = pcfg
					}
					_ = config.Save(t.cfg)
					t.renderPending.Store(true)
				}
			}
		} else if t.settingsIndex > 0 {
			t.settingsIndex--
			t.renderPending.Store(true)
		}
	case KeyDown:
		if t.settingsIndex < len(providers)-1 {
			t.settingsIndex++
			t.renderPending.Store(true)
		}
	case KeyEnter:
		t.settingsKeyEdit = true
		t.settingsKeyBuf = ""
		t.renderPending.Store(true)
	}
}

// testProviderPing runs a single synchronous ping against the first model of
// the provider and reports the outcome.
func (t *TUI) testProviderPing(provider string) string {
	if t.registry == nil {
		return "no registry"
	}
	var model *models.Model
	for _, m := range t.registry.Snapshot() {
		if m.Provider == provider && m.Endpoint != "" {
			model = m
			break
		}
	}
	if model == nil {
		return "no models"
	}

	body := `{"model":"` + model.UpstreamModelID + `","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`
	req, err := http.NewRequest(http.MethodPost, model.Endpoint, stringsNewReader(body))
	if err != nil {
		return "err"
	}
	req.Header.Set("Content-Type", "application/json")
	if model.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+model.APIKey)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "fail"
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch resp.StatusCode {
	case 200:
		return "up"
	case 401:
		return "noauth"
	case 403:
		return "forbidden"
	case 429:
		return "ratelimit"
	default:
		return fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
}

// launchTarget best-effort launches the agent binary if installed.
func launchTarget(binary string) {
	if binary == "" {
		return
	}
	if _, err := exec.LookPath(binary); err != nil {
		return
	}
	_ = exec.Command(binary).Start()
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
	t.renderPending.Store(true)
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
	t.renderPending.Store(true)
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
	t.renderPending.Store(true)
}

func (t *TUI) setSort(key string) {
	if t.sortKey == key {
		t.sortReverse = !t.sortReverse
	} else {
		t.sortKey = key
		t.sortReverse = false
	}
	t.renderPending.Store(true)
}

func containsLower(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func stringsTrimSpace(s string) string {
	start := 0
	for start < len(s) {
		c := s[start]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			start++
			continue
		}
		break
	}
	end := len(s)
	for end > start {
		c := s[end-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
			continue
		}
		break
	}
	return s[start:end]
}

func stringsNewReader(s string) *strings.Reader {
	return strings.NewReader(s)
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
