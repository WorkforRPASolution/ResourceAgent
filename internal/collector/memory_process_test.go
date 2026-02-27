package collector

import (
	"context"
	"testing"
	"time"

	"resourceagent/internal/config"
)

func TestMemoryProcessCollector_Collect_WithWatchProcesses(t *testing.T) {
	collector := NewMemoryProcessCollector()

	cfg := config.CollectorConfig{
		Enabled:        true,
		Interval:       30 * time.Second,
		TopN:           5,
		WatchProcesses: []string{"go", "zsh"},
	}
	if err := collector.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metric, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if metric.Type != "MemoryProcess" {
		t.Errorf("Type = %q, want %q", metric.Type, "MemoryProcess")
	}

	data, ok := metric.Data.(ProcessMemoryData)
	if !ok {
		t.Fatalf("Data is not ProcessMemoryData")
	}

	if len(data.Processes) == 0 {
		t.Fatal("No processes collected")
	}

	watchedCount := 0
	for _, p := range data.Processes {
		if p.Watched {
			watchedCount++
			t.Logf("Watched process: PID=%d, Name=%s, Mem=%.2f%%, RSS=%d", p.PID, p.Name, p.MemoryPercent, p.RSS)
		}
	}

	// Verify watched processes are collected
	if watchedCount == 0 {
		t.Error("Expected at least one watched process (go or zsh)")
	}

	t.Logf("Total processes: %d, Watched: %d", len(data.Processes), watchedCount)
	for i, p := range data.Processes {
		t.Logf("  [%d] PID=%d, Name=%s, Mem=%.2f%%, RSS=%d, Watched=%v", i, p.PID, p.Name, p.MemoryPercent, p.RSS, p.Watched)
	}
}

func TestMemoryProcessCollector_Collect_WatchedFirst(t *testing.T) {
	collector := NewMemoryProcessCollector()

	cfg := config.CollectorConfig{
		Enabled:        true,
		Interval:       30 * time.Second,
		TopN:           10,
		WatchProcesses: []string{"go"},
	}
	if err := collector.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metric, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	data := metric.Data.(ProcessMemoryData)

	firstNonWatchedIdx := -1
	lastWatchedIdx := -1
	for i, p := range data.Processes {
		if p.Watched {
			lastWatchedIdx = i
		} else if firstNonWatchedIdx == -1 {
			firstNonWatchedIdx = i
		}
	}

	if lastWatchedIdx != -1 && firstNonWatchedIdx != -1 && lastWatchedIdx > firstNonWatchedIdx {
		t.Errorf("Watched processes should come first: lastWatchedIdx=%d, firstNonWatchedIdx=%d",
			lastWatchedIdx, firstNonWatchedIdx)
	}
}

func TestMemoryProcessCollector_Collect_NoWatchProcesses(t *testing.T) {
	collector := NewMemoryProcessCollector()

	cfg := config.CollectorConfig{
		Enabled:  true,
		Interval: 30 * time.Second,
		TopN:     5,
	}
	if err := collector.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metric, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	data := metric.Data.(ProcessMemoryData)

	for _, p := range data.Processes {
		if p.Watched {
			t.Errorf("Process %s has Watched=true but no watch list configured", p.Name)
		}
	}

	if len(data.Processes) > 5 {
		t.Errorf("Got %d processes, want at most %d", len(data.Processes), 5)
	}
}
