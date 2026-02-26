package collector

import (
	"context"
	"testing"
	"time"

	"resourceagent/internal/config"
)

func TestCPUProcessCollector_Collect_WithWatchProcesses(t *testing.T) {
	collector := NewCPUProcessCollector()

	// Configure with watch_processes (use user-accessible process names)
	cfg := config.CollectorConfig{
		Enabled:        true,
		Interval:       30 * time.Second,
		TopN:           5,
		WatchProcesses: []string{"go", "zsh"}, // processes we can access without root
	}
	if err := collector.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Warmup call (populates baselines, returns nil)
	warmup, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Warmup Collect failed: %v", err)
	}
	if warmup != nil {
		t.Error("First call should return nil for warmup")
	}

	// Real collection
	metric, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if metric.Type != "cpu_process" {
		t.Errorf("Type = %q, want %q", metric.Type, "cpu_process")
	}

	data, ok := metric.Data.(ProcessCPUData)
	if !ok {
		t.Fatalf("Data is not ProcessCPUData")
	}

	if len(data.Processes) == 0 {
		t.Fatal("No processes collected")
	}

	// Check that watched processes have Watched=true
	watchedCount := 0
	for _, p := range data.Processes {
		if p.Watched {
			watchedCount++
			t.Logf("Watched process: PID=%d, Name=%s, CPU=%.2f%%", p.PID, p.Name, p.CPUPercent)
		}
	}

	// Verify watched processes are collected
	if watchedCount == 0 {
		t.Error("Expected at least one watched process (go or zsh)")
	}

	// Verify CPU% is normalized to 0-100% range
	for _, p := range data.Processes {
		if p.CPUPercent > 100.0 {
			t.Errorf("CPU%% = %.2f for %s, should be <= 100.0 (normalized)", p.CPUPercent, p.Name)
		}
	}

	// Log all collected processes
	t.Logf("Total processes: %d, Watched: %d", len(data.Processes), watchedCount)
	for i, p := range data.Processes {
		t.Logf("  [%d] PID=%d, Name=%s, CPU=%.2f%%, Watched=%v", i, p.PID, p.Name, p.CPUPercent, p.Watched)
	}
}

func TestCPUProcessCollector_Collect_WatchedFirst(t *testing.T) {
	collector := NewCPUProcessCollector()

	cfg := config.CollectorConfig{
		Enabled:        true,
		Interval:       30 * time.Second,
		TopN:           10,
		WatchProcesses: []string{"go"}, // should appear first if running
	}
	if err := collector.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Warmup call
	warmup, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Warmup Collect failed: %v", err)
	}
	if warmup != nil {
		t.Error("First call should return nil for warmup")
	}

	// Real collection
	metric, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	data := metric.Data.(ProcessCPUData)

	// Find first non-watched process index
	firstNonWatchedIdx := -1
	lastWatchedIdx := -1
	for i, p := range data.Processes {
		if p.Watched {
			lastWatchedIdx = i
		} else if firstNonWatchedIdx == -1 {
			firstNonWatchedIdx = i
		}
	}

	// All watched should come before non-watched
	if lastWatchedIdx != -1 && firstNonWatchedIdx != -1 && lastWatchedIdx > firstNonWatchedIdx {
		t.Errorf("Watched processes should come first: lastWatchedIdx=%d, firstNonWatchedIdx=%d",
			lastWatchedIdx, firstNonWatchedIdx)
	}
}

func TestCPUProcessCollector_Collect_NoWatchProcesses(t *testing.T) {
	collector := NewCPUProcessCollector()

	cfg := config.CollectorConfig{
		Enabled:  true,
		Interval: 30 * time.Second,
		TopN:     5,
		// No WatchProcesses
	}
	if err := collector.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Warmup call
	warmup, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Warmup Collect failed: %v", err)
	}
	if warmup != nil {
		t.Error("First call should return nil for warmup")
	}

	// Real collection
	metric, err := collector.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	data := metric.Data.(ProcessCPUData)

	// All processes should have Watched=false
	for _, p := range data.Processes {
		if p.Watched {
			t.Errorf("Process %s has Watched=true but no watch list configured", p.Name)
		}
	}

	// Should have at most TopN processes
	if len(data.Processes) > 5 {
		t.Errorf("Got %d processes, want at most %d", len(data.Processes), 5)
	}
}
