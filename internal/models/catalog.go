package models

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/freemodel/router/internal/providers"
)

type Model struct {
	ID              string
	Label           string
	Context         string
	Provider        string
	QualityScore    float64
	Tier            string
	Tags            []string
	Status          string
	AvgLatency      float64
	Uptime          float64
	LatestPing      float64
	Pings           []PingEntry
	HTTPCode        string
	Endpoint        string
	UpstreamModelID string
	ProviderHost    string
	APIKey          string
	Disabled        bool
	Banned          bool
	Excluded        bool
	FailStreak      int
	SkippedRounds   int
}

type PingEntry struct {
	Latency  float64
	HTTPCode string
	Status   string
	At       int64
}

type Registry struct {
	mu         sync.RWMutex
	models     map[string]*Model
	codingOnly bool
}

func NewRegistry() *Registry {
	return &Registry{
		models: make(map[string]*Model),
	}
}

func (r *Registry) Add(m *Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[m.ID] = m
}

func (r *Registry) Get(id string) *Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if m, ok := r.models[id]; ok {
		return m
	}
	return nil
}

func (r *Registry) GetAll() []*Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Model, 0, len(r.models))
	for _, m := range r.models {
		result = append(result, m)
	}
	return result
}

func (r *Registry) GetByProvider(provider string) []*Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []*Model
	for _, m := range r.models {
		if m.Provider == provider {
			result = append(result, m)
		}
	}
	return result
}

func (r *Registry) ReplaceAll(models []*Model) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models = make(map[string]*Model, len(models))
	for _, m := range models {
		r.models[m.ID] = m
	}
}

// Snapshot returns deep copies of all models so concurrent readers can
// render/select without racing against ping workers writing to live models.
func (r *Registry) Snapshot() []*Model {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]*Model, 0, len(r.models))
	for _, m := range r.models {
		result = append(result, copyModel(m))
	}
	return result
}

// UpdateModel applies fn to the live model under the registry write lock.
func (r *Registry) UpdateModel(id string, fn func(m *Model)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m, ok := r.models[id]; ok && fn != nil {
		fn(m)
	}
}

// WithModel invokes fn with the live model under the registry read lock.
func (r *Registry) WithModel(id string, fn func(m *Model)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if m, ok := r.models[id]; ok && fn != nil {
		fn(m)
	}
}

func copyModel(m *Model) *Model {
	cp := *m
	cp.Tags = append([]string(nil), m.Tags...)
	cp.Pings = append([]PingEntry(nil), m.Pings...)
	return &cp
}

func (r *Registry) FlagCodingOnly(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.codingOnly = enabled
}

func (r *Registry) CodingOnlyEnabled() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.codingOnly
}

// BanModel bans every model matching the ID exactly or by group short name.
// Returns the number of models banned.
func (r *Registry) BanModel(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, m := range r.models {
		if m.ID == id || ResolveGroup(m.ID) == id {
			if !m.Banned {
				m.Banned = true
				count++
			}
		}
	}
	return count
}

func (r *Registry) UnbanModel(id string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, m := range r.models {
		if m.ID == id || ResolveGroup(m.ID) == id {
			if m.Banned {
				m.Banned = false
				count++
			}
		}
	}
	return count
}

func parseContext(s string) string {
	return s
}

func (r *Registry) LoadFromSources(mgr *providers.Manager) {
	models := mgr.GetAllModels()
	var result []*Model
	for _, m := range models {
		provider := strings.SplitN(m.ID, "/", 2)[0]
		result = append(result, &Model{
			ID:       m.ID,
			Label:    m.Label,
			Context:  parseContext(m.Context),
			Provider: provider,
			Status:   "pending",
		})
	}
	r.ReplaceAll(result)
}

func canonicalizeID(id string, aliases map[string]string) string {
	short := strings.TrimPrefix(id, "openrouter/")
	if canonical, ok := aliases[short]; ok {
		return canonical
	}
	return short
}

func ComputeTier(score float64) string {
	switch {
	case score >= 0.70:
		return "S+"
	case score >= 0.60:
		return "S"
	case score >= 0.50:
		return "A+"
	case score >= 0.40:
		return "A"
	case score >= 0.35:
		return "A-"
	case score >= 0.30:
		return "B+"
	case score >= 0.20:
		return "B"
	default:
		return "C"
	}
}

func LoadAliases(path string) (map[string]string, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var aliases map[string]string
	if err := json.Unmarshal(data, &aliases); err != nil {
		return nil, err
	}
	return aliases, nil
}

func LoadScores(path string) (map[string]float64, error) {
	data, err := readFile(path)
	if err != nil {
		return nil, err
	}
	var scores map[string]float64
	if err := json.Unmarshal(data, &scores); err != nil {
		return nil, err
	}
	return scores, nil
}

func ResolveGroup(id string) string {
	parts := strings.Split(id, "/")
	if len(parts) >= 2 {
		return parts[len(parts)-1]
	}
	return id
}

func FindByGroup(registry *Registry, group string) []*Model {
	all := registry.GetAll()
	var result []*Model
	for _, m := range all {
		if ResolveGroup(m.ID) == group {
			result = append(result, m)
		}
	}
	return result
}

func SortModels(list []*Model, sortKey string, reverse bool) {
	less := func(i, j int) bool {
		mi, mj := list[i], list[j]
		switch sortKey {
		case "tier":
			return tierRank(mi.Tier) < tierRank(mj.Tier)
		case "provider":
			return mi.Provider < mj.Provider
		case "model":
			return mi.Label < mj.Label
		case "avg":
			return mi.AvgLatency < mj.AvgLatency
		case "lat":
			return mi.LatestPing < mj.LatestPing
		case "uptime":
			return mi.Uptime > mj.Uptime
		case "context":
			return parseContextValue(mi.Context) < parseContextValue(mj.Context)
		default:
			return mi.Status == "up" && mj.Status != "up"
		}
	}
	if reverse {
		sort.SliceStable(list, func(i, j int) bool { return less(j, i) })
	} else {
		sort.SliceStable(list, func(i, j int) bool { return less(i, j) })
	}
}

func tierRank(tier string) int {
	ranks := map[string]int{"S+": 8, "S": 7, "A+": 6, "A": 5, "A-": 4, "B+": 3, "B": 2, "C": 1}
	if r, ok := ranks[tier]; ok {
		return r
	}
	return 0
}

func parseContextValue(s string) float64 {
	s = strings.TrimSuffix(s, "k")
	s = strings.TrimSuffix(s, "K")
	var val float64
	for _, r := range s {
		if r >= '0' && r <= '9' {
			val = val*10 + float64(r-'0')
		}
	}
	return val
}

func FilterByTier(models []*Model, tier string) []*Model {
	if tier == "" || tier == "All" {
		return models
	}
	var result []*Model
	for _, m := range models {
		if m.Tier == tier {
			result = append(result, m)
		}
	}
	return result
}

func FilterByProvider(models []*Model, provider string) []*Model {
	if provider == "" || provider == "All" {
		return models
	}
	var result []*Model
	for _, m := range models {
		if m.Provider == provider {
			result = append(result, m)
		}
	}
	return result
}

func FilterBySearch(models []*Model, query string) []*Model {
	if query == "" {
		return models
	}
	query = strings.ToLower(query)
	var result []*Model
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), query) ||
			strings.Contains(strings.ToLower(m.Label), query) ||
			strings.Contains(strings.ToLower(m.Provider), query) {
			result = append(result, m)
		}
	}
	return result
}

func FindBestModel(models []*Model) *Model {
	var best *Model
	bestQoS := -1.0
	for _, m := range models {
		if m.Status != "up" {
			continue
		}
		qos := ComputeQoS(m)
		if qos > bestQoS {
			bestQoS = qos
			best = m
		}
	}
	return best
}

func ComputeQoS(m *Model) float64 {
	availability := 1.0
	switch {
	case m.Uptime >= 95:
		availability = 1.0
	case m.Uptime >= 85:
		availability = 0.9
	case m.Uptime >= 70:
		availability = 0.6
	default:
		availability = 0.2
	}
	pingTieBreaker := 0.0
	if m.AvgLatency > 0 {
		pingTieBreaker = (1000 - m.AvgLatency) / 1000
		if pingTieBreaker < 0 {
			pingTieBreaker = 0
		}
	}
	return m.QualityScore*availability + pingTieBreaker
}
