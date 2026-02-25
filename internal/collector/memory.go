package collector

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/mem"

	"resourceagent/internal/config"
)

// MemoryCollector collects system memory usage metrics.
type MemoryCollector struct {
	BaseCollector
}

// NewMemoryCollector creates a new memory collector.
func NewMemoryCollector() *MemoryCollector {
	return &MemoryCollector{
		BaseCollector: NewBaseCollector("memory"),
	}
}

// Configure applies the configuration to the collector.
func (c *MemoryCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	return nil
}

// Collect gathers memory metrics.
func (c *MemoryCollector) Collect(ctx context.Context) (*MetricData, error) {
	vm, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return nil, err
	}

	swap, err := mem.SwapMemoryWithContext(ctx)
	if err != nil {
		// Swap may not be available on all systems, continue with zero values
		swap = &mem.SwapMemoryStat{}
	}

	memData := MemoryData{
		TotalBytes:     vm.Total,
		UsedBytes:      vm.Used,
		AvailableBytes: vm.Available,
		UsagePercent:   vm.UsedPercent,
		SwapTotalBytes: swap.Total,
		SwapUsedBytes:  swap.Used,
		SwapFreeBytes:  swap.Free,
		SwapPercent:    swap.UsedPercent,
		Cached:         vm.Cached,
		Buffers:        vm.Buffers,
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      memData,
	}, nil
}
