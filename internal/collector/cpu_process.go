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
	topN           int             // Number of top processes to report
	watchProcesses []string        // List of process names to always include
	matcher        *ProcessMatcher // For efficient process name matching
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
	if cfg.TopN >= 0 {
		c.topN = cfg.TopN
	}
	c.watchProcesses = cfg.WatchProcesses
	c.matcher = NewProcessMatcher(cfg.WatchProcesses)
	return nil
}

// Collect gathers per-process CPU metrics using a 2-pass approach for performance.
// 1st pass: collect only CPU% and name for all processes (minimal syscalls)
// 2nd pass: collect detailed info (username, createTime) only for selected processes
//
// Selection algorithm:
// 1. All watched processes are collected (no limit)
// 2. Remaining slots (topN - watched count) filled from top CPU consumers
// 3. Output: watched processes first, then top N (no duplicates)
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
		watched    bool
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

		watched := c.matcher.IsWatched(name)

		quickList = append(quickList, quickInfo{
			proc:       p,
			cpuPercent: cpuPercent,
			name:       name,
			watched:    watched,
		})
	}

	// Separate watched and non-watched processes
	var watchedList []quickInfo
	var nonWatchedList []quickInfo

	for _, q := range quickList {
		if q.watched {
			watchedList = append(watchedList, q)
		} else {
			nonWatchedList = append(nonWatchedList, q)
		}
	}

	// Sort watched by CPU descending (for consistent ordering)
	sort.Slice(watchedList, func(i, j int) bool {
		return watchedList[i].cpuPercent > watchedList[j].cpuPercent
	})

	// Sort non-watched by CPU descending
	sort.Slice(nonWatchedList, func(i, j int) bool {
		return nonWatchedList[i].cpuPercent > nonWatchedList[j].cpuPercent
	})

	// Calculate remaining slots for non-watched processes
	remainingSlots := c.topN - len(watchedList)
	if remainingSlots < 0 {
		remainingSlots = 0
	}

	// Take top N from non-watched
	if len(nonWatchedList) > remainingSlots {
		nonWatchedList = nonWatchedList[:remainingSlots]
	}

	// Combine: watched first, then top N non-watched
	selectedList := make([]quickInfo, 0, len(watchedList)+len(nonWatchedList))
	selectedList = append(selectedList, watchedList...)
	selectedList = append(selectedList, nonWatchedList...)

	// 2nd Pass: detailed info only for selected processes (2 syscalls per process)
	processList := make([]ProcessCPU, 0, len(selectedList))
	for _, q := range selectedList {
		username, _ := q.proc.UsernameWithContext(ctx)
		createTime, _ := q.proc.CreateTimeWithContext(ctx)

		processList = append(processList, ProcessCPU{
			PID:        q.proc.Pid,
			Name:       q.name,
			CPUPercent: q.cpuPercent,
			Username:   username,
			CreateTime: createTime,
			Watched:    q.watched,
		})
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      ProcessCPUData{Processes: processList},
	}, nil
}
