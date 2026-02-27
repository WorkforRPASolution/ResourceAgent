package collector

import (
	"context"
	"testing"
	"time"
)

func TestUptimeCollector_Collect(t *testing.T) {
	c := NewUptimeCollector()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	metric, err := c.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if metric.Type != "Uptime" {
		t.Errorf("Type = %q, want %q", metric.Type, "Uptime")
	}

	data, ok := metric.Data.(UptimeData)
	if !ok {
		t.Fatalf("Data is not UptimeData: %T", metric.Data)
	}

	// Boot time should be in the past
	bootTime := time.Unix(data.BootTimeUnix, 0)
	if bootTime.After(time.Now()) {
		t.Errorf("BootTimeUnix is in the future: %v", bootTime)
	}
	if bootTime.Before(time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("BootTimeUnix too old: %v", bootTime)
	}

	// BootTimeStr should be non-empty and parseable
	if data.BootTimeStr == "" {
		t.Error("BootTimeStr is empty")
	}
	parsed, err := time.ParseInLocation("2006-01-02T15:04:05", data.BootTimeStr, time.Local)
	if err != nil {
		t.Errorf("BootTimeStr not parseable: %v", err)
	}
	if parsed.Unix() != data.BootTimeUnix {
		t.Errorf("BootTimeStr (%d) != BootTimeUnix (%d)", parsed.Unix(), data.BootTimeUnix)
	}

	// Uptime should be positive
	if data.UptimeMinutes <= 0 {
		t.Errorf("UptimeMinutes = %f, want > 0", data.UptimeMinutes)
	}

	t.Logf("Boot: %s, Uptime: %.1f minutes", data.BootTimeStr, data.UptimeMinutes)
}

func TestUptimeCollector_Configure(t *testing.T) {
	c := NewUptimeCollector()

	if c.Name() != "Uptime" {
		t.Errorf("Name = %q, want %q", c.Name(), "Uptime")
	}

	// Default interval is 10s (BaseCollector default)
	if c.Interval() != 10*time.Second {
		t.Errorf("default Interval = %v, want 10s", c.Interval())
	}
}
