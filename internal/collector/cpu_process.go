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

// Collect gathers per-process CPU metrics using a 2-pass approach for performance.
// 1st pass: collect only CPU% and name for all processes (minimal syscalls)
// 2nd pass: collect detailed info (username, createTime) only for top N processes
func (c *CPUProcessCollector) Collect(ctx context.Context) (*MetricData, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	// 1st Pass: CPU% + Name only (2 syscalls per process)
	type quickInfo struct {
		proc       *process.Process
		cpuPercent float64
		name       string
	}
	quickList := make([]quickInfo, 0, len(procs))

	for _, p := range procs {
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

		quickList = append(quickList, quickInfo{
			proc:       p,
			cpuPercent: cpuPercent,
			name:       name,
		})
	}

	// Sort by CPU usage descending
	sort.Slice(quickList, func(i, j int) bool {
		return quickList[i].cpuPercent > quickList[j].cpuPercent
	})

	// Keep only top N
	if len(quickList) > c.topN {
		quickList = quickList[:c.topN]
	}

	// 2nd Pass: detailed info only for top N (2 syscalls per process)
	processList := make([]ProcessCPU, 0, len(quickList))
	for _, q := range quickList {
		username, _ := q.proc.UsernameWithContext(ctx)
		createTime, _ := q.proc.CreateTimeWithContext(ctx)

		processList = append(processList, ProcessCPU{
			PID:        q.proc.Pid,
			Name:       q.name,
			CPUPercent: q.cpuPercent,
			Username:   username,
			CreateTime: createTime,
		})
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now().UTC(),
		Data:      ProcessCPUData{Processes: processList},
	}, nil
}
