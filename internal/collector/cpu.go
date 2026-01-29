package collector

import (
	"context"
	"runtime"
	"time"

	"github.com/shirou/gopsutil/v3/cpu"

	"resourceagent/internal/config"
)

// CPUCollector collects overall CPU usage metrics.
type CPUCollector struct {
	BaseCollector
}

// NewCPUCollector creates a new CPU collector.
func NewCPUCollector() *CPUCollector {
	return &CPUCollector{
		BaseCollector: NewBaseCollector("cpu"),
	}
}

// Configure applies the configuration to the collector.
func (c *CPUCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	return nil
}

// Collect gathers CPU metrics.
func (c *CPUCollector) Collect(ctx context.Context) (*MetricData, error) {
	// Get overall CPU usage (blocking call for 200ms to measure)
	percentages, err := cpu.PercentWithContext(ctx, 200*time.Millisecond, false)
	if err != nil {
		return nil, err
	}

	// Get per-core CPU usage
	perCore, err := cpu.PercentWithContext(ctx, 0, true)
	if err != nil {
		perCore = nil // Non-critical, continue without per-core data
	}

	// Get CPU times for detailed breakdown
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return nil, err
	}

	var cpuData CPUData
	cpuData.CoreCount = runtime.NumCPU()

	if len(percentages) > 0 {
		cpuData.UsagePercent = percentages[0]
	}

	if len(times) > 0 {
		t := times[0]
		total := t.User + t.System + t.Idle + t.Iowait + t.Irq + t.Softirq + t.Steal + t.Guest
		if total > 0 {
			cpuData.User = (t.User / total) * 100
			cpuData.System = (t.System / total) * 100
			cpuData.Idle = (t.Idle / total) * 100
			cpuData.IOWait = (t.Iowait / total) * 100
			cpuData.Irq = (t.Irq / total) * 100
			cpuData.SoftIrq = (t.Softirq / total) * 100
			cpuData.Steal = (t.Steal / total) * 100
			cpuData.Guest = (t.Guest / total) * 100
		}
	}

	if len(perCore) > 0 {
		cpuData.PerCore = perCore
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now().UTC(),
		Data:      cpuData,
	}, nil
}
