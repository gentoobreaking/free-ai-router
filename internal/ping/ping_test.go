package ping

import (
	"testing"

	"github.com/freemodel/router/internal/models"
)

func TestGetAvg(t *testing.T) {
	m := &models.Model{}
	m.Pings = []models.PingEntry{
		{HTTPCode: "200", Latency: 100},
		{HTTPCode: "200", Latency: 200},
		{HTTPCode: "429", Latency: 9999},
	}

	avg := GetAvg(m)
	if avg != 150 {
		t.Errorf("avg should be 150 (only 200s), got %f", avg)
	}
}

func TestGetAvgEmpty(t *testing.T) {
	m := &models.Model{}
	if avg := GetAvg(m); avg != 0 {
		t.Errorf("empty avg should be 0, got %f", avg)
	}
}

func TestGetUptime(t *testing.T) {
	m := &models.Model{}
	m.Pings = []models.PingEntry{
		{HTTPCode: "200"},
		{HTTPCode: "200"},
		{HTTPCode: "500"},
		{HTTPCode: "200"},
	}
	if up := GetUptime(m); up != 75 {
		t.Errorf("uptime should be 75, got %f", up)
	}
}

func TestGetVerdict(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		avg     float64
		http    string
		latency float64
		want    string
	}{
		{"perfect", "up", 300, "200", 250, "Perfect"},
		{"normal", "up", 500, "200", 450, "Normal"},
		{"slow", "up", 2000, "200", 1500, "Slow"},
		{"very slow", "up", 4000, "200", 3500, "Very Slow"},
		{"unusable", "up", 6000, "200", 5500, "Unusable"},
		{"ratelimit", "ratelimit", 0, "429", 0, "Overloaded"},
		{"pending", "pending", 0, "", 0, "Pending"},
		{"down", "down", 0, "500", 0, "Not Active"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &models.Model{
				Status:     tt.status,
				AvgLatency: tt.avg,
				HTTPCode:   tt.http,
			}
			if tt.latency > 0 {
				m.LatestPing = tt.latency
			}
			if got := GetVerdict(m); got != tt.want {
				t.Errorf("verdict = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStatusFromCode(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{200, "up"},
		{401, "noauth"},
		{403, "forbidden"},
		{404, "notfound"},
		{429, "ratelimit"},
		{503, "unavailable"},
		{500, "down"},
		{502, "down"},
	}
	for _, tt := range tests {
		if got := StatusFromCode(tt.code); got != tt.want {
			t.Errorf("StatusFromCode(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestShouldSkip(t *testing.T) {
	m := &models.Model{}
	// 2 failures: should not skip
	m.FailStreak = 2
	if shouldSkip(m) {
		t.Error("2 failures should not trigger backoff")
	}

	// 3 failures: skip allowed for 1 round (pure predicate)
	m.FailStreak = 3
	if !shouldSkip(m) {
		t.Error("3 failures should allow backoff")
	}
	// after the scheduler advanced one skipped round, not allowed again
	m.SkippedRounds = 1
	if shouldSkip(m) {
		t.Error("should not skip more than backoffRounds consecutive rounds")
	}

	// success resets the streak
	applyResultMut(m, Result{ModelID: "test/model", Latency: 100000000, HTTPCode: 200, Status: "up"})
	if m.FailStreak != 0 {
		t.Errorf("success should reset FailStreak, got %d", m.FailStreak)
	}
	if shouldSkip(m) {
		t.Error("reset model should not be skipped")
	}
}

func TestSkipRoundsFor(t *testing.T) {
	tests := []struct {
		failures int
		want     int
	}{
		{2, 0},
		{3, 1},
		{4, 2},
		{5, 4},
		{6, 8},
		{7, 16},
	}
	for _, tt := range tests {
		if got := skipRoundsFor(tt.failures); got != tt.want {
			t.Errorf("skipRoundsFor(%d) = %d, want %d", tt.failures, got, tt.want)
		}
	}
}

func TestApplyResult(t *testing.T) {
	m := &models.Model{ID: "test/model"}
	applyResultMut(m, Result{ModelID: "test/model", Latency: 1500000000, HTTPCode: 200, Status: "up"})

	if m.Status != "up" {
		t.Errorf("status should be up, got %s", m.Status)
	}
	if len(m.Pings) != 1 {
		t.Errorf("should have 1 ping entry, got %d", len(m.Pings))
	}
	if m.AvgLatency != 1500 {
		t.Errorf("avg latency should be 1500ms, got %f", m.AvgLatency)
	}
	if m.HTTPCode != "200" {
		t.Errorf("httpCode should be 200, got %s", m.HTTPCode)
	}
}

func TestApplyResultFailure(t *testing.T) {
	m := &models.Model{ID: "test/model"}
	applyResultMut(m, Result{ModelID: "test/model", HTTPCode: 429, Status: "ratelimit"})

	if m.Status != "ratelimit" {
		t.Errorf("status should be ratelimit, got %s", m.Status)
	}
	if m.HTTPCode != "429" {
		t.Errorf("httpCode should be 429, got %s", m.HTTPCode)
	}
}
