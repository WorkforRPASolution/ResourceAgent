package collector

import (
	"context"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"resourceagent/internal/config"
)

// CPUProcessCollector collects per-process CPU usage metrics.
type CPUProcessCollector struct {
	BaseCollector
	topN int // Number of top processes to report
}

// NewCPUProcessCollector creates a new CPU process collector.
func NewCPUProcessCollector() *CPUProcessCollector {
	return &CPUProcessCollector{
		BaseCollector: NewBaseCollector("cpu_process"),
		topN:          10,
	}
}

// Configure applies the configuration to the collector.
func (c *CPUProcessCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	if cfg.TopN > 0 {
		c.topN = cfg.TopN
	}
	return nil
}

// Collect gathers per-process CPU metrics.
func (c *CPUProcessCollector) Collect(ctx context.Context) (*MetricData, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	var processList []ProcessCPU

	for _, p := range procs {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		cpuPercent, err := p.CPUPercentWithContext(ctx)
		if err != nil {
			continue
		}

		name, _ := p.NameWithContext(ctx)
		if name == "" {
			continue
		}

		numThreads, _ := p.NumThreadsWithContext(ctx)
		username, _ := p.UsernameWithContext(ctx)
		createTime, _ := p.CreateTimeWithContext(ctx)

		proc := ProcessCPU{
			PID:        p.Pid,
			Name:       name,
			CPUPercent: cpuPercent,
			NumThreads: numThreads,
			Username:   username,
			CreateTime: createTime,
		}

		processList = append(processList, proc)
	}

	// Sort by CPU usage descending
	sort.Slice(processList, func(i, j int) bool {
		return processList[i].CPUPercent > processList[j].CPUPercent
	})

	// Keep only top N
	if len(processList) > c.topN {
		processList = processList[:c.topN]
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now().UTC(),
		Data:      ProcessCPUData{Processes: processList},
	}, nil
}
