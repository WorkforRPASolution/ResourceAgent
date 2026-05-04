package collector

import (
	"context"
	"os"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"resourceagent/internal/config"
)

// RuntimeStatsProvider abstracts runtime/process introspection so tests can
// inject deterministic values for goroutine count, heap stats, RSS, and
// OS handle/fd count.
type RuntimeStatsProvider interface {
	NumGoroutine() int
	AllocBytes() uint64
	SysBytes() uint64
	ProcessRSSBytes() (uint64, error)
	// ProcessHandleCount returns the OS-level resource handle count for the
	// current process. On Windows this is the kernel HANDLE count
	// (GetProcessHandleCount). On Linux it is the fd count from /proc/self/fd.
	// macOS returns 0 (dev environment only). Returns 0 + error on probe failure.
	ProcessHandleCount() (uint32, error)
}

// BufferStatsProvider mirrors sender.BufferStatsProvider so that
// SelfMetricsCollector can read transport-level buffer stats without
// importing the sender package (avoids an import cycle: sender already
// depends on collector). main.go performs the type wiring.
type BufferStatsProvider interface {
	BufferStats() (count, dropped, hwm int64)
}

// SelfMetricsCollector emits a snapshot of agent runtime metrics on every
// Collect cycle (Phase 2.5-1). Sent through the standard pipeline as
// MetricData{Type: "SelfMetrics"}.
type SelfMetricsCollector struct {
	BaseCollector
	stats       RuntimeStatsProvider
	bufferStats BufferStatsProvider // may be nil (file sender etc.)
}

// NewSelfMetricsCollector constructs a SelfMetricsCollector.
// bufferStats may be nil when no buffered transport is configured.
func NewSelfMetricsCollector(stats RuntimeStatsProvider, bs BufferStatsProvider) *SelfMetricsCollector {
	base := NewBaseCollector("SelfMetrics")
	base.SetInterval(60 * time.Second)
	return &SelfMetricsCollector{
		BaseCollector: base,
		stats:         stats,
		bufferStats:   bs,
	}
}

// Configure applies the configuration to the collector.
func (c *SelfMetricsCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	return nil
}

// DefaultConfig returns the default config (60s interval, enabled).
func (c *SelfMetricsCollector) DefaultConfig() config.CollectorConfig {
	return config.CollectorConfig{
		Enabled:  true,
		Interval: 60 * time.Second,
	}
}

// Collect captures a runtime snapshot. Errors from individual probes are
// swallowed (the field defaults to 0) so that one failing probe does not
// suppress the others — operators can still see the rest of the picture.
func (c *SelfMetricsCollector) Collect(_ context.Context) (*MetricData, error) {
	rss, _ := c.stats.ProcessRSSBytes()
	handles, _ := c.stats.ProcessHandleCount()

	var bufCount, bufDropped int64
	if c.bufferStats != nil {
		bufCount, bufDropped, _ = c.bufferStats.BufferStats()
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data: SelfMetricsData{
			GoroutineCount:     c.stats.NumGoroutine(),
			RSSBytes:           rss,
			HeapAllocBytes:     c.stats.AllocBytes(),
			HeapSysBytes:       c.stats.SysBytes(),
			HandleCount:        handles,
			BufferCount:        bufCount,
			BufferDroppedTotal: bufDropped,
		},
	}, nil
}

// --- Production RuntimeStatsProvider ---
//
// OS-independent fields (NumGoroutine / AllocBytes / SysBytes / ProcessRSSBytes)
// live here. ProcessHandleCount is OS-specific and split into:
//   - selfmetrics_windows.go : GetProcessHandleCount syscall (kernel32.dll)
//   - selfmetrics_linux.go   : /proc/self/fd entry count
//   - selfmetrics_other.go   : stub returning 0 (macOS, BSD, dev only)

type defaultRuntimeStats struct {
	proc *process.Process
}

// NewDefaultRuntimeStats returns a RuntimeStatsProvider backed by the Go
// runtime and gopsutil. Process handle initialization errors are silently
// absorbed; ProcessRSSBytes returns 0 in that case.
func NewDefaultRuntimeStats() RuntimeStatsProvider {
	p, _ := process.NewProcess(int32(os.Getpid()))
	return &defaultRuntimeStats{proc: p}
}

func (d *defaultRuntimeStats) NumGoroutine() int { return runtime.NumGoroutine() }

func (d *defaultRuntimeStats) AllocBytes() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Alloc
}

func (d *defaultRuntimeStats) SysBytes() uint64 {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	return ms.Sys
}

func (d *defaultRuntimeStats) ProcessRSSBytes() (uint64, error) {
	if d.proc == nil {
		return 0, nil
	}
	mi, err := d.proc.MemoryInfo()
	if err != nil || mi == nil {
		return 0, err
	}
	return mi.RSS, nil
}
