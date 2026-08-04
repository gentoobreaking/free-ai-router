package ping

import (
	"errors"
	"strconv"
	"time"

	"github.com/freemodel/router/internal/models"
)

var errNoEndpoint = errors.New("no endpoint configured for model")

// applyResultMut mutates model state from ping results. It is the low-level
// unlocked implementation; callers must hold the registry write lock when
// model state is shared (spec §16.3).
func applyResultMut(m *models.Model, r Result) {
	entry := models.PingEntry{
		Latency:  float64(r.Latency) / float64(time.Millisecond),
		HTTPCode: intToStr(r.HTTPCode),
		Status:   r.Status,
		At:       time.Now().Unix(),
	}

	if r.HTTPCode > 0 {
		m.HTTPCode = intToStr(r.HTTPCode)
	}

	m.Pings = append(m.Pings, entry)
	if len(m.Pings) > HistoryCap {
		m.Pings = m.Pings[len(m.Pings)-HistoryCap:]
	}

	if r.HTTPCode == 200 {
		m.LatestPing = entry.Latency
		m.Status = "up"
		m.FailStreak = 0
		m.SkippedRounds = 0
	} else {
		m.Status = r.Status
		m.FailStreak++
		m.SkippedRounds = 0
	}

	m.AvgLatency = GetAvg(m)
	m.Uptime = GetUptime(m)
}

func intToStr(n int) string {
	if n == 0 {
		return "ERR"
	}
	return strconv.Itoa(n)
}

func GetAvg(m *models.Model) float64 {
	var total float64
	var count int
	for _, p := range m.Pings {
		if p.HTTPCode == "200" {
			total += p.Latency
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / float64(count)
}

func GetUptime(m *models.Model) float64 {
	total := len(m.Pings)
	if total == 0 {
		return 0
	}
	var up int
	for _, p := range m.Pings {
		if p.HTTPCode == "200" {
			up++
		}
	}
	return float64(up) / float64(total) * 100
}

func GetVerdict(m *models.Model) string {
	if m.Status == "ratelimit" {
		return "Overloaded"
	}
	if m.Status == "pending" {
		return "Pending"
	}
	if m.Status != "up" {
		return "Not Active"
	}
	if m.AvgLatency == 0 {
		return "Pending"
	}
	switch {
	case m.AvgLatency < 400:
		return "Perfect"
	case m.AvgLatency < 1000:
		return "Normal"
	case m.AvgLatency < 3000:
		return "Slow"
	case m.AvgLatency < 5000:
		return "Very Slow"
	default:
		return "Unusable"
	}
}
