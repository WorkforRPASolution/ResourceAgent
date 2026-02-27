package collector

import (
	"context"
	"os"
	"runtime"
	"testing"
	"time"

	"resourceagent/internal/config"
)

func TestProcessWatchCollector_Name(t *testing.T) {
	c := NewProcessWatchCollector()
	if c.Name() != "ProcessWatch" {
		t.Errorf("Name() = %q, want %q", c.Name(), "ProcessWatch")
	}
}

func TestProcessWatchCollector_Configure(t *testing.T) {
	c := NewProcessWatchCollector()

	cfg := config.CollectorConfig{
		Enabled:            true,
		Interval:           60 * time.Second,
		RequiredProcesses:  []string{"mes.exe", "scada.exe"},
		ForbiddenProcesses: []string{"torrent.exe"},
	}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	if !c.Enabled() {
		t.Error("expected Enabled = true")
	}
	if c.Interval() != 60*time.Second {
		t.Errorf("Interval = %v, want 60s", c.Interval())
	}
}

func TestProcessWatchCollector_EmptyConfig(t *testing.T) {
	c := NewProcessWatchCollector()
	cfg := config.CollectorConfig{Enabled: true}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx := context.Background()
	metric, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if metric != nil {
		t.Errorf("expected nil metric for empty watch lists, got %+v", metric)
	}
}

func TestProcessWatchCollector_CollectWithCurrentProcess(t *testing.T) {
	// Use "go" or the test binary name which we know is running
	var currentProc string
	if runtime.GOOS == "windows" {
		currentProc = "go.exe"
	} else {
		// The test binary process name varies; use a known running process
		currentProc = selfProcessName(t)
	}

	c := NewProcessWatchCollector()
	cfg := config.CollectorConfig{
		Enabled:            true,
		Interval:           60 * time.Second,
		RequiredProcesses:  []string{currentProc},
		ForbiddenProcesses: []string{"nonexistent_process_xyz_12345"},
	}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metric, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if metric.Type != "ProcessWatch" {
		t.Errorf("Type = %q, want %q", metric.Type, "ProcessWatch")
	}

	data, ok := metric.Data.(ProcessWatchData)
	if !ok {
		t.Fatalf("Data is not ProcessWatchData: %T", metric.Data)
	}

	if len(data.Statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d: %+v", len(data.Statuses), data.Statuses)
	}

	// Required process (current running) should be found
	reqStatus := data.Statuses[0]
	if reqStatus.Name != currentProc {
		t.Errorf("required name = %q, want %q", reqStatus.Name, currentProc)
	}
	if reqStatus.Type != "required" {
		t.Errorf("required type = %q, want %q", reqStatus.Type, "required")
	}
	if !reqStatus.Running {
		t.Errorf("required running = false, want true (process %q should be running)", currentProc)
	}
	if reqStatus.PID <= 0 {
		t.Errorf("required PID = %d, want > 0", reqStatus.PID)
	}

	// Forbidden process (nonexistent) should not be found
	forbStatus := data.Statuses[1]
	if forbStatus.Name != "nonexistent_process_xyz_12345" {
		t.Errorf("forbidden name = %q, want %q", forbStatus.Name, "nonexistent_process_xyz_12345")
	}
	if forbStatus.Type != "forbidden" {
		t.Errorf("forbidden type = %q, want %q", forbStatus.Type, "forbidden")
	}
	if forbStatus.Running {
		t.Error("forbidden running = true, want false")
	}
	if forbStatus.PID != 0 {
		t.Errorf("forbidden PID = %d, want 0", forbStatus.PID)
	}
}

func TestProcessWatchCollector_CollectWithoutConfigure(t *testing.T) {
	// Simulates the case where Monitor.json doesn't have a ProcessWatch entry:
	// DefaultRegistry registers the collector but Configure() is never called.
	// This must NOT panic (nil pointer dereference on matchers).
	c := NewProcessWatchCollector()
	// Do NOT call Configure — this is the bug scenario

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metric, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect without Configure should not fail: %v", err)
	}
	if metric != nil {
		t.Errorf("expected nil metric without Configure (empty lists), got %+v", metric)
	}
}

func TestProcessWatchCollector_MetricType(t *testing.T) {
	c := NewProcessWatchCollector()
	cfg := config.CollectorConfig{
		Enabled:           true,
		RequiredProcesses: []string{"dummy"},
	}
	c.Configure(cfg)

	metric, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if metric.Type != "ProcessWatch" {
		t.Errorf("Type = %q, want %q", metric.Type, "ProcessWatch")
	}
}

// selfProcessName returns the name of the current process (the test binary).
func selfProcessName(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	// Extract just the filename
	for i := len(exe) - 1; i >= 0; i-- {
		if exe[i] == '/' || exe[i] == '\\' {
			return exe[i+1:]
		}
	}
	return exe
}
