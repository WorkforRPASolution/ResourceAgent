package collector

import (
	"container/heap"
	"context"
	"runtime"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"resourceagent/internal/config"
)

// cpuQuickInfo holds minimal per-process data from the 1st pass.
type cpuQuickInfo struct {
	proc       *process.Process
	cpuPercent float64
	name       string
	watched    bool
}

// cpuMinHeap is a min-heap of cpuQuickInfo ordered by cpuPercent (lowest at top).
type cpuMinHeap []cpuQuickInfo

func (h cpuMinHeap) Len() int            { return len(h) }
func (h cpuMinHeap) Less(i, j int) bool   { return h[i].cpuPercent < h[j].cpuPercent }
func (h cpuMinHeap) Swap(i, j int)        { h[i], h[j] = h[j], h[i] }
func (h *cpuMinHeap) Push(x interface{})  { *h = append(*h, x.(cpuQuickInfo)) }
func (h *cpuMinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// CPUProcessCollector collects per-process CPU usage metrics.
type CPUProcessCollector struct {
	BaseCollector
	topN           int             // Number of top processes to report
	watchProcesses []string        // List of process names to always include
	matcher        *ProcessMatcher // For efficient process name matching
	numCPU         float64         // Number of logical CPUs for normalization
	warmedUp       bool            // Whether baseline CPUPercent has been populated
}

// NewCPUProcessCollector creates a new CPU process collector.
func NewCPUProcessCollector() *CPUProcessCollector {
	return &CPUProcessCollector{
		BaseCollector: NewBaseCollector("CPUProcess"),
		topN:          10,
		numCPU:        float64(runtime.NumCPU()),
	}
}

// DefaultConfig returns the default CollectorConfig for the CPU process collector.
func (c *CPUProcessCollector) DefaultConfig() config.CollectorConfig {
	cfg := c.BaseCollector.DefaultConfig()
	cfg.Interval = 30 * time.Second
	cfg.TopN = 10
	return cfg
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

	// Warmup: first call populates gopsutil internal baselines, returns nil
	if !c.warmedUp {
		for _, p := range procs {
			p.CPUPercentWithContext(ctx)
		}
		c.warmedUp = true
		return nil, nil
	}

	// 1st Pass: CPU% + Name only (2 syscalls per process).
	// Watched processes go to watchedList; non-watched go directly into a min-heap
	// of size remainingSlots for O(n log k) TopN selection.
	var watchedList []cpuQuickInfo
	h := &cpuMinHeap{}

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
		cpuPercent = cpuPercent / c.numCPU // Normalize to 0-100% range

		name, _ := p.NameWithContext(ctx)
		if name == "" {
			continue
		}

		q := cpuQuickInfo{
			proc:       p,
			cpuPercent: cpuPercent,
			name:       name,
			watched:    c.matcher.IsWatched(name),
		}

		if q.watched {
			watchedList = append(watchedList, q)
		} else {
			// Min-heap: keep top remainingSlots items
			remainingSlots := c.topN - len(watchedList)
			if remainingSlots <= 0 {
				continue
			}
			if h.Len() < remainingSlots {
				heap.Push(h, q)
			} else if q.cpuPercent > (*h)[0].cpuPercent {
				(*h)[0] = q
				heap.Fix(h, 0)
			}
		}
	}

	// Sort watched by CPU descending (for consistent ordering)
	sort.Slice(watchedList, func(i, j int) bool {
		return watchedList[i].cpuPercent > watchedList[j].cpuPercent
	})

	// Extract heap items sorted descending
	heapItems := make([]cpuQuickInfo, h.Len())
	for i := h.Len() - 1; i >= 0; i-- {
		heapItems[i] = heap.Pop(h).(cpuQuickInfo)
	}

	// 2nd Pass: detailed info only for selected processes (2 syscalls per process)
	processList := make([]ProcessCPU, 0, len(watchedList)+len(heapItems))
	for _, q := range watchedList {
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
	for _, q := range heapItems {
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
