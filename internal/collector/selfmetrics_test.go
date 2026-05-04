package collector

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"resourceagent/internal/config"
)

func runtimeIsHandleSupported() bool {
	return runtime.GOOS == "windows" || runtime.GOOS == "linux"
}

type mockRuntimeStats struct {
	goroutines int
	alloc      uint64
	sys        uint64
	rss        uint64
	rssErr     error
	handles    uint32
	handlesErr error
}

func (m *mockRuntimeStats) NumGoroutine() int                    { return m.goroutines }
func (m *mockRuntimeStats) AllocBytes() uint64                   { return m.alloc }
func (m *mockRuntimeStats) SysBytes() uint64                     { return m.sys }
func (m *mockRuntimeStats) ProcessRSSBytes() (uint64, error)     { return m.rss, m.rssErr }
func (m *mockRuntimeStats) ProcessHandleCount() (uint32, error)  { return m.handles, m.handlesErr }

type mockBufferStats struct {
	count   int64
	dropped int64
	hwm     int64
}

func (m *mockBufferStats) BufferStats() (int64, int64, int64) {
	return m.count, m.dropped, m.hwm
}

func TestSelfMetricsCollector_BasicCollect(t *testing.T) {
	stats := &mockRuntimeStats{
		goroutines: 42,
		alloc:      1024,
		sys:        2048,
		rss:        31457280,
		handles:    184,
	}
	bs := &mockBufferStats{count: 100, dropped: 5, hwm: 200}

	c := NewSelfMetricsCollector(stats, bs)
	md, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if md.Type != "SelfMetrics" {
		t.Errorf("Type = %q, want %q", md.Type, "SelfMetrics")
	}
	d, ok := md.Data.(SelfMetricsData)
	if !ok {
		t.Fatalf("Data type = %T, want SelfMetricsData", md.Data)
	}
	if d.GoroutineCount != 42 {
		t.Errorf("GoroutineCount = %d, want 42", d.GoroutineCount)
	}
	if d.RSSBytes != 31457280 {
		t.Errorf("RSSBytes = %d, want 31457280", d.RSSBytes)
	}
	if d.HeapAllocBytes != 1024 {
		t.Errorf("HeapAllocBytes = %d, want 1024", d.HeapAllocBytes)
	}
	if d.HeapSysBytes != 2048 {
		t.Errorf("HeapSysBytes = %d, want 2048", d.HeapSysBytes)
	}
	if d.HandleCount != 184 {
		t.Errorf("HandleCount = %d, want 184", d.HandleCount)
	}
	if d.BufferCount != 100 {
		t.Errorf("BufferCount = %d, want 100", d.BufferCount)
	}
	if d.BufferDroppedTotal != 5 {
		t.Errorf("BufferDroppedTotal = %d, want 5", d.BufferDroppedTotal)
	}
}

func TestSelfMetricsCollector_HandleProbeFailureSwallowed(t *testing.T) {
	stats := &mockRuntimeStats{
		goroutines: 5,
		handles:    0,
		handlesErr: errors.New("handle probe failed"),
	}
	c := NewSelfMetricsCollector(stats, nil)
	md, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect surfaced handle probe error: %v", err)
	}
	d := md.Data.(SelfMetricsData)
	if d.HandleCount != 0 {
		t.Errorf("HandleCount = %d, want 0 on probe failure", d.HandleCount)
	}
	if d.GoroutineCount != 5 {
		t.Errorf("other probes still emitted; GoroutineCount = %d, want 5", d.GoroutineCount)
	}
}

func TestSelfMetricsCollector_NilBufferStats(t *testing.T) {
	c := NewSelfMetricsCollector(&mockRuntimeStats{goroutines: 10}, nil)
	md, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	d := md.Data.(SelfMetricsData)
	if d.BufferCount != 0 {
		t.Errorf("BufferCount = %d, want 0", d.BufferCount)
	}
	if d.BufferDroppedTotal != 0 {
		t.Errorf("BufferDroppedTotal = %d, want 0", d.BufferDroppedTotal)
	}
	if d.GoroutineCount != 10 {
		t.Errorf("GoroutineCount = %d, want 10", d.GoroutineCount)
	}
}

func TestSelfMetricsCollector_RSSProbeFailureSwallowed(t *testing.T) {
	stats := &mockRuntimeStats{
		goroutines: 5,
		alloc:      100,
		rss:        0,
		rssErr:     errors.New("permission denied"),
	}
	c := NewSelfMetricsCollector(stats, nil)
	md, err := c.Collect(context.Background())
	if err != nil {
		t.Fatalf("Collect surfaced RSS probe error: %v", err)
	}
	d := md.Data.(SelfMetricsData)
	if d.RSSBytes != 0 {
		t.Errorf("RSSBytes = %d, want 0 on probe failure", d.RSSBytes)
	}
	if d.GoroutineCount != 5 {
		t.Errorf("other probes still emitted; GoroutineCount = %d, want 5", d.GoroutineCount)
	}
}

func TestSelfMetricsCollector_ConfigureSetsInterval(t *testing.T) {
	c := NewSelfMetricsCollector(&mockRuntimeStats{}, nil)
	if got := c.Interval(); got != 60*time.Second {
		t.Errorf("default Interval = %v, want 60s", got)
	}
	cfg := config.CollectorConfig{Enabled: true, Interval: 30 * time.Second}
	if err := c.Configure(cfg); err != nil {
		t.Fatalf("Configure returned error: %v", err)
	}
	if got := c.Interval(); got != 30*time.Second {
		t.Errorf("Interval after Configure = %v, want 30s", got)
	}
	if !c.Enabled() {
		t.Error("Enabled = false, want true")
	}
}

func TestDefaultRuntimeStats_Smoke(t *testing.T) {
	s := NewDefaultRuntimeStats()
	if g := s.NumGoroutine(); g <= 0 {
		t.Errorf("NumGoroutine = %d, want > 0", g)
	}
	if sys := s.SysBytes(); sys == 0 {
		t.Errorf("SysBytes = 0, want > 0")
	}
	if alloc := s.AllocBytes(); alloc == 0 {
		t.Errorf("AllocBytes = 0, want > 0")
	}
	rss, err := s.ProcessRSSBytes()
	if err != nil {
		t.Fatalf("ProcessRSSBytes returned error: %v", err)
	}
	if rss == 0 {
		t.Errorf("ProcessRSSBytes = 0, want > 0")
	}
	// HandleCount: Windows/Linux must report > 0; macOS/BSD stub returns 0.
	handles, err := s.ProcessHandleCount()
	if err != nil {
		t.Fatalf("ProcessHandleCount returned error: %v", err)
	}
	if runtimeIsHandleSupported() && handles == 0 {
		t.Errorf("ProcessHandleCount = 0 on supported platform, want > 0")
	}
}
