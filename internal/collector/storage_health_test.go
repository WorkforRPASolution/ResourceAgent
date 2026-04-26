package collector

import (
	"context"
	"testing"
	"time"

	"resourceagent/internal/config"
)

func TestStorageHealthCollector_Name(t *testing.T) {
	c := NewStorageHealthCollector()
	if c.Name() != "StorageHealth" {
		t.Errorf("Name() = %q, want %q", c.Name(), "StorageHealth")
	}
}

func TestStorageHealthCollector_DefaultConfig(t *testing.T) {
	c := NewStorageHealthCollector()
	cfg := c.DefaultConfig()
	if cfg.Interval != 300*time.Second {
		t.Errorf("DefaultConfig().Interval = %v, want 300s", cfg.Interval)
	}
	if !cfg.Enabled {
		t.Error("DefaultConfig().Enabled = false, want true")
	}
}

func TestStorageHealthCollector_Configure(t *testing.T) {
	c := NewStorageHealthCollector()
	cfg := config.CollectorConfig{
		Enabled:  true,
		Interval: 600 * time.Second,
		Disks:    []string{"sda", "nvme0n1"},
	}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure failed: %v", err)
	}
	if c.Interval() != 600*time.Second {
		t.Errorf("Interval = %v, want 600s", c.Interval())
	}
	if !c.Enabled() {
		t.Error("Enabled = false after Configure(Enabled: true)")
	}
}

func TestStorageHealthCollector_Collect(t *testing.T) {
	c := NewStorageHealthCollector()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metric, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}
	if metric == nil {
		t.Fatal("metric is nil")
	}
	if metric.Type != "StorageHealth" {
		t.Errorf("Type = %q, want %q", metric.Type, "StorageHealth")
	}
	data, ok := metric.Data.(StorageHealthData)
	if !ok {
		t.Fatalf("Data is not StorageHealthData: %T", metric.Data)
	}
	t.Logf("Found %d disks", len(data.Disks))
	for _, d := range data.Disks {
		t.Logf("  %s: status=%s raw=%s type=%s", d.Name, d.Status, d.RawStatus, d.DiskType)
		switch d.Status {
		case "OK", "DEGRADED", "PRED_FAIL", "FAIL", "UNKNOWN":
			// valid
		default:
			t.Errorf("unexpected status %q for disk %s", d.Status, d.Name)
		}
	}
}

func TestStorageHealthCollector_CollectWithoutConfigure(t *testing.T) {
	c := NewStorageHealthCollector()
	// Collect without Configure — should not panic
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	metric, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect without Configure failed: %v", err)
	}
	if metric == nil {
		t.Fatal("metric is nil")
	}
}

func TestNormalizeHealthStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// OK
		{"OK", "OK"},
		{"Ok", "OK"},
		{"ok", "OK"},
		{"PASSED", "OK"},
		{"Passed", "OK"},
		{"passed", "OK"},

		// DEGRADED
		{"Degraded", "DEGRADED"},
		{"DEGRADED", "DEGRADED"},
		{"Stressed", "DEGRADED"},
		{"STRESSED", "DEGRADED"},

		// PRED_FAIL
		{"Pred Fail", "PRED_FAIL"},
		{"PRED FAIL", "PRED_FAIL"},
		{"pred fail", "PRED_FAIL"},

		// FAIL
		{"FAILED!", "FAIL"},
		{"FAILED", "FAIL"},
		{"Failed", "FAIL"},
		{"Error", "FAIL"},
		{"ERROR", "FAIL"},
		{"NonRecover", "FAIL"},
		{"NONRECOVER", "FAIL"},
		{"Lost Comm", "FAIL"},
		{"LOST COMM", "FAIL"},
		{"No Contact", "FAIL"},
		{"NO CONTACT", "FAIL"},

		// UNKNOWN
		{"", "UNKNOWN"},
		{"Unknown", "UNKNOWN"},
		{"UNKNOWN", "UNKNOWN"},
		{"  ", "UNKNOWN"},
		{"  OK  ", "OK"},             // trimmed → OK
		{" Pred Fail ", "PRED_FAIL"}, // trimmed → PRED_FAIL
	}

	for _, tt := range tests {
		got := normalizeHealthStatus(tt.input)
		if got != tt.expected {
			t.Errorf("normalizeHealthStatus(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
