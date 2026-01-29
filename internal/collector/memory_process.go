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

// Collect gathers per-process memory metrics using a 2-pass approach for performance.
// 1st pass: collect only memory info and name for all processes (minimal syscalls)
// 2nd pass: collect detailed info (username, createTime) only for top N processes
func (c *MemoryProcessCollector) Collect(ctx context.Context) (*MetricData, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	// 1st Pass: Memory% + Name + MemInfo only (3 syscalls per process)
	type quickInfo struct {
		proc          *process.Process
		memoryPercent float32
		name          string
		rss           uint64
		vms           uint64
		swap          uint64
	}
	quickList := make([]quickInfo, 0, len(procs))

	for _, p := range procs {
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

		quickList = append(quickList, quickInfo{
			proc:          p,
			memoryPercent: memPercent,
			name:          name,
			rss:           memInfo.RSS,
			vms:           memInfo.VMS,
			swap:          memInfo.Swap,
		})
	}

	// Sort by memory usage descending
	sort.Slice(quickList, func(i, j int) bool {
		return quickList[i].memoryPercent > quickList[j].memoryPercent
	})

	// Keep only top N
	if len(quickList) > c.topN {
		quickList = quickList[:c.topN]
	}

	// 2nd Pass: detailed info only for top N (2 syscalls per process)
	processList := make([]ProcessMemory, 0, len(quickList))
	for _, q := range quickList {
		username, _ := q.proc.UsernameWithContext(ctx)
		createTime, _ := q.proc.CreateTimeWithContext(ctx)

		processList = append(processList, ProcessMemory{
			PID:           q.proc.Pid,
			Name:          q.name,
			MemoryPercent: float64(q.memoryPercent),
			RSS:           q.rss,
			VMS:           q.vms,
			Swap:          q.swap,
			Username:      username,
			CreateTime:    createTime,
		})
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now().UTC(),
		Data:      ProcessMemoryData{Processes: processList},
	}, nil
}
