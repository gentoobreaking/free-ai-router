package ping

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/freemodel/router/internal/models"
)

// TestPingAllOnceProducerSkipPath exercises the scheduler skip path (growing
// FailStreak → shouldSkip/markSkipped) under concurrent Snapshot reads, so
// -race validates that the producer reads skip state under the registry lock
// (T049).
func TestPingAllOnceProducerSkipPath(t *testing.T) {
	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	closedURL := closed.URL
	closed.Close()

	registry := models.NewRegistry()
	for i := 0; i < 8; i++ {
		registry.Add(&models.Model{
			ID: fmt.Sprintf("p%d/m%d", i%2, i), Provider: "p",
			Status: "pending", Endpoint: closedURL, UpstreamModelID: "m",
		})
	}

	engine := NewEngine(nil)
	engine.SetRegistry(registry)
	engine.SetModels(registry.GetAll())

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			engine.PingAllOnce(false)
		}
	}()

	// Give the loop time to accumulate FailStreak >= 3 so the skip path runs.
	time.Sleep(150 * time.Millisecond)
	for i := 0; i < 30; i++ {
		_ = registry.Snapshot()
		time.Sleep(5 * time.Millisecond)
	}
	close(stop)
	<-done
	engine.Stop()
}
