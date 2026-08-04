package ping

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/freemodel/router/internal/models"
)

// TestPingConcurrentWithReads runs the ping engine against mock upstreams
// while a reader goroutine snapshots the registry — must be race-free
// (spec §16.3). Run with `go test -race`.
func TestPingConcurrentWithReads(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer srv.Close()

	registry := models.NewRegistry()
	for i := 0; i < 8; i++ {
		registry.Add(&models.Model{
			ID:              fmt.Sprintf("provider-%d/model", i),
			Provider:        fmt.Sprintf("provider-%d", i),
			Status:          "pending",
			Endpoint:        srv.URL,
			UpstreamModelID: "model",
		})
	}

	engine := NewEngine(nil)
	engine.SetRegistry(registry)
	engine.SetModels(registry.GetAll())

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				for _, m := range registry.Snapshot() {
					_ = m.Status
					_ = m.AvgLatency
					_ = m.Uptime
					_ = m.FailStreak
					_ = m.SkippedRounds
					_ = len(m.Pings)
				}
			}
		}
	}()

	for round := 0; round < 5; round++ {
		engine.PingAllOnce(round == 0)
	}

	close(stop)
	wg.Wait()
	engine.Stop()

	for _, m := range registry.GetAll() {
		if m.Status != "up" {
			t.Errorf("model %s should be up after successful pings, got %s", m.ID, m.Status)
		}
	}
}

// TestPingAllFailConcurrentWithReads exercises the failure path (backoff
// counters) under concurrent reads.
func TestPingAllFailConcurrentWithReads(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := closed.URL
	closed.Close()

	registry := models.NewRegistry()
	for i := 0; i < 4; i++ {
		registry.Add(&models.Model{
			ID:              fmt.Sprintf("dead-%d/model", i),
			Provider:        fmt.Sprintf("dead-%d", i),
			Status:          "pending",
			Endpoint:        deadURL,
			UpstreamModelID: "model",
		})
	}

	engine := NewEngine(nil)
	engine.SetRegistry(registry)
	engine.SetModels(registry.GetAll())

	stop := make(chan struct{})
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				for _, m := range registry.Snapshot() {
					_ = m.Status
					_ = m.FailStreak
					_ = m.SkippedRounds
				}
			}
		}
	}()

	for round := 0; round < 4; round++ {
		engine.PingAllOnce(round == 0)
	}

	close(stop)
	wg.Wait()
	engine.Stop()

	for _, m := range registry.GetAll() {
		if m.FailStreak < 3 {
			t.Errorf("model %s should have accumulated failures, got %d", m.ID, m.FailStreak)
		}
	}
}
