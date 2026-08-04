package models

import "testing"

func TestSnapshotIsDeepCopy(t *testing.T) {
	registry := NewRegistry()
	registry.Add(&Model{
		ID:     "provider/model",
		Status: "up",
		Pings:  []PingEntry{{Latency: 100, HTTPCode: "200", Status: "up", At: 1}},
		Tags:   []string{"coding"},
	})

	snapshot := registry.Snapshot()
	if len(snapshot) != 1 {
		t.Fatalf("expected 1 snapshot model, got %d", len(snapshot))
	}

	// Mutating the snapshot must not affect the live model.
	snapshot[0].Status = "down"
	snapshot[0].Pings = append(snapshot[0].Pings, PingEntry{Latency: 200})
	snapshot[0].Tags = append(snapshot[0].Tags, "general")

	live := registry.Get("provider/model")
	if live.Status != "up" {
		t.Error("snapshot mutation leaked into live model status")
	}
	if len(live.Pings) != 1 {
		t.Errorf("snapshot mutation leaked into live pings: %d entries", len(live.Pings))
	}
	if len(live.Tags) != 1 {
		t.Errorf("snapshot mutation leaked into live tags: %v", live.Tags)
	}
}

func TestUpdateModelUnderLock(t *testing.T) {
	registry := NewRegistry()
	registry.Add(&Model{ID: "provider/model", Status: "up"})

	registry.UpdateModel("provider/model", func(m *Model) {
		m.Status = "down"
		m.FailStreak = 3
	})

	m := registry.Get("provider/model")
	if m.Status != "down" || m.FailStreak != 3 {
		t.Errorf("UpdateModel should apply changes, got status=%s streak=%d", m.Status, m.FailStreak)
	}

	registry.UpdateModel("does-not-exist", func(m *Model) { m.Status = "down" })
}

func TestBanModel(t *testing.T) {
	registry := NewRegistry()
	registry.Add(&Model{ID: "nvidia/deepseek-ai/deepseek-v3.2"})
	registry.Add(&Model{ID: "openrouter/deepseek/deepseek-v3.2"})
	registry.Add(&Model{ID: "nvidia/other-model"})

	// Ban by exact ID
	if count := registry.BanModel("nvidia/deepseek-ai/deepseek-v3.2"); count != 1 {
		t.Errorf("exact ban should hit 1 model, got %d", count)
	}
	// Ban by group short name
	if count := registry.BanModel("other-model"); count != 1 {
		t.Errorf("group ban should hit 1 model, got %d", count)
	}

	if !registry.Get("nvidia/deepseek-ai/deepseek-v3.2").Banned {
		t.Error("exact-banned model should be marked")
	}
	if !registry.Get("nvidia/other-model").Banned {
		t.Error("group-banned model should be marked")
	}
	if registry.Get("openrouter/deepseek/deepseek-v3.2").Banned {
		t.Error("unrelated model must not be banned")
	}

	if count := registry.UnbanModel("other-model"); count != 1 {
		t.Errorf("unban should hit 1 model, got %d", count)
	}
	if registry.Get("nvidia/other-model").Banned {
		t.Error("unbanned model should be unmarked")
	}
}

func TestFlagCodingOnly(t *testing.T) {
	registry := NewRegistry()
	if registry.CodingOnlyEnabled() {
		t.Error("codingOnly should default to false")
	}
	registry.FlagCodingOnly(true)
	if !registry.CodingOnlyEnabled() {
		t.Error("codingOnly should be enabled after FlagCodingOnly(true)")
	}
}
