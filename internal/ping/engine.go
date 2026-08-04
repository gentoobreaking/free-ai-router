package ping

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/freemodel/router/internal/models"
)

const (
	InitialConcurrency = 64
	SteadyConcurrency  = 20
	InitialTimeout     = 2500 * time.Millisecond
	SteadyTimeout      = 6 * time.Second
	DefaultInterval    = 2 * time.Second
	HistoryCap         = 100
)

type Result struct {
	ModelID  string
	Latency  time.Duration
	HTTPCode int
	Status   string
	Err      error
}

type TransportPool struct {
	mu         sync.Mutex
	transports map[string]*http.Transport
}

func NewTransportPool() *TransportPool {
	return &TransportPool{
		transports: make(map[string]*http.Transport),
	}
}

func (tp *TransportPool) Get(host string) *http.Transport {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	if t, ok := tp.transports[host]; ok {
		return t
	}
	t := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}
	tp.transports[host] = t
	return t
}

type Engine struct {
	pool     *TransportPool
	interval time.Duration
	mu       sync.RWMutex
	stop     chan struct{}
	onUpdate func()
	running  bool
	epoch    int64
	models   map[string]*models.Model
	registry *models.Registry
}

func NewEngine(onUpdate func()) *Engine {
	return &Engine{
		pool:     NewTransportPool(),
		interval: DefaultInterval,
		stop:     make(chan struct{}),
		onUpdate: onUpdate,
		models:   make(map[string]*models.Model),
	}
}

func (e *Engine) SetInterval(d time.Duration) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.interval = d
	e.epoch++
}

func (e *Engine) Interval() time.Duration {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.interval
}

func (e *Engine) Epoch() int64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.epoch
}

func (e *Engine) BumpEpoch() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.epoch++
}

func (e *Engine) SetModels(list []*models.Model) {
	e.mu.Lock()
	defer e.mu.Unlock()
	m := make(map[string]*models.Model, len(list))
	for _, model := range list {
		m[model.ID] = model
	}
	e.models = m
}

// SetRegistry associates the registry whose write lock protects model state
// (spec §16.3). When set, all ping results and backoff counters are applied
// under the registry lock.
func (e *Engine) SetRegistry(registry *models.Registry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.registry = registry
}

func (e *Engine) SetPool(pool *TransportPool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if pool != nil {
		e.pool = pool
	}
}

// apply routes a ping result through the registry write lock when available.
func (e *Engine) apply(m *models.Model, r Result) {
	e.mu.RLock()
	registry := e.registry
	e.mu.RUnlock()

	if registry != nil {
		registry.UpdateModel(m.ID, func(x *models.Model) { applyResultMut(x, r) })
		return
	}
	applyResultMut(m, r)
}

func (e *Engine) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running {
		close(e.stop)
		e.running = false
	}
}

func (e *Engine) Start() {
	e.mu.Lock()
	if e.running {
		e.mu.Unlock()
		return
	}
	e.running = true
	e.stop = make(chan struct{})
	stop := e.stop
	e.mu.Unlock()

	go func() {
		e.PingAllOnce(true)
		for {
			e.PingAllOnce(false)
			e.mu.RLock()
			interval := e.interval
			e.mu.RUnlock()

			select {
			case <-stop:
				return
			case <-time.After(interval):
			}
		}
	}()
}

func (e *Engine) PingAllOnce(initial bool) {
	e.mu.RLock()
	modelsCopy := make([]*models.Model, 0, len(e.models))
	for _, m := range e.models {
		modelsCopy = append(modelsCopy, m)
	}
	e.mu.RUnlock()

	concurrency := SteadyConcurrency
	timeout := SteadyTimeout
	if initial {
		concurrency = InitialConcurrency
		timeout = InitialTimeout
	}

	work := make(chan *models.Model)
	go func() {
		for _, m := range modelsCopy {
			if shouldSkip(m) {
				e.markSkipped(m)
				continue
			}
			work <- m
		}
		close(work)
	}()

	var wg sync.WaitGroup
	sem := make(chan struct{}, concurrency)

	for m := range work {
		sem <- struct{}{}
		wg.Add(1)
		go func(m *models.Model) {
			defer wg.Done()
			defer func() { <-sem }()
			e.pingOne(m, timeout)
		}(m)
	}
	wg.Wait()

	if e.onUpdate != nil {
		e.onUpdate()
	}
}

func skipRoundsFor(failures int) int {
	switch {
	case failures >= 7:
		return 16
	case failures >= 6:
		return 8
	case failures >= 5:
		return 4
	case failures >= 4:
		return 2
	case failures >= 3:
		return 1
	default:
		return 0
	}
}

// shouldSkip is a pure predicate: a model with >= 3 consecutive failures is
// skipped for up to backoffRounds(failures) rounds. The counter is advanced
// by the scheduler (markSkipped) under the registry lock, never here.
func shouldSkip(m *models.Model) bool {
	if m.FailStreak < 3 {
		return false
	}
	return m.SkippedRounds < skipRoundsFor(m.FailStreak)
}

// markSkipped advances the skipped-round counter under the registry lock.
func (e *Engine) markSkipped(m *models.Model) {
	e.mu.RLock()
	registry := e.registry
	e.mu.RUnlock()

	if registry != nil {
		registry.UpdateModel(m.ID, func(x *models.Model) { x.SkippedRounds++ })
		return
	}
	m.SkippedRounds++
}

func (e *Engine) pingOne(m *models.Model, timeout time.Duration) {
	if m.Endpoint == "" {
		e.apply(m, Result{ModelID: m.ID, Status: "down", Err: errNoEndpoint})
		return
	}

	start := time.Now()

	body := `{"model":"` + m.UpstreamModelID + `","messages":[{"role":"user","content":"ping"}],"max_tokens":1}`
	req, err := http.NewRequest(http.MethodPost, m.Endpoint, strings.NewReader(body))
	if err != nil {
		e.apply(m, Result{ModelID: m.ID, Status: "down", Err: err})
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if m.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+m.APIKey)
	}

	client := &http.Client{
		Transport: e.pool.Get(m.ProviderHost),
		Timeout:   timeout,
	}

	resp, err := client.Do(req)
	if err != nil {
		elapsed := time.Since(start)
		status := "down"
		if isTimeout(err) {
			status = "timeout"
		}
		e.apply(m, Result{ModelID: m.ID, Latency: elapsed, HTTPCode: 0, Status: status, Err: err})
		return
	}

	elapsed := time.Since(start)
	_, _ = io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	e.apply(m, Result{
		ModelID:  m.ID,
		Latency:  elapsed,
		HTTPCode: resp.StatusCode,
		Status:   StatusFromCode(resp.StatusCode),
	})
}

func isTimeout(err error) bool {
	if err == nil {
		return false
	}
	if ne, ok := err.(interface{ Timeout() bool }); ok {
		return ne.Timeout()
	}
	return false
}

func StatusFromCode(code int) string {
	switch code {
	case 200:
		return "up"
	case 401:
		return "noauth"
	case 403:
		return "forbidden"
	case 404:
		return "notfound"
	case 429:
		return "ratelimit"
	case 503:
		return "unavailable"
	default:
		if code >= 500 {
			return "down"
		}
		return "up"
	}
}
