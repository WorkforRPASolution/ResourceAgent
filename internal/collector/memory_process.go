package collector

import (
	"context"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"resourceagent/internal/config"
)

// MemoryProcessCollector collects per-process memory usage metrics.
type MemoryProcessCollector struct {
	BaseCollector
	topN int // Number of top processes to report
}

// NewMemoryProcessCollector creates a new memory process collector.
func NewMemoryProcessCollector() *MemoryProcessCollector {
	return &MemoryProcessCollector{
		BaseCollector: NewBaseCollector("memory_process"),
		topN:          10,
	}
}

// Configure applies the configuration to the collector.
func (c *MemoryProcessCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	if cfg.TopN > 0 {
		c.topN = cfg.TopN
	}
	return nil
}

// Collect gathers per-process memory metrics.
func (c *MemoryProcessCollector) Collect(ctx context.Context) (*MetricData, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var processList []ProcessMemory

	for _, p := range procs {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		memInfo, err := p.MemoryInfoWithContext(ctx)
		if err != nil || memInfo == nil {
			continue
		}

		memPercent, err := p.MemoryPercentWithContext(ctx)
		if err != nil {
			continue
		}

		name, _ := p.NameWithContext(ctx)
		if name == "" {
			continue
		}

		username, _ := p.UsernameWithContext(ctx)
		createTime, _ := p.CreateTimeWithContext(ctx)

		proc := ProcessMemory{
			PID:           p.Pid,
			Name:          name,
			MemoryPercent: float64(memPercent),
			RSS:           memInfo.RSS,
			VMS:           memInfo.VMS,
			Swap:          memInfo.Swap,
			Username:      username,
			CreateTime:    createTime,
		}

		processList = append(processList, proc)
	}

	// Sort by memory usage descending
	sort.Slice(processList, func(i, j int) bool {
		return processList[i].MemoryPercent > processList[j].MemoryPercent
	})

	// Keep only top N
	if len(processList) > c.topN {
		processList = processList[:c.topN]
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now().UTC(),
		Data:      ProcessMemoryData{Processes: processList},
	}, nil
}
