package collector

import (
	"container/heap"
	"context"
	"sort"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"resourceagent/internal/config"
)

// memQuickInfo holds minimal per-process data from the 1st pass.
type memQuickInfo struct {
	proc          *process.Process
	memoryPercent float32
	name          string
	rss           uint64
	vms           uint64
	swap          uint64
	watched       bool
}

// memMinHeap is a min-heap of memQuickInfo ordered by memoryPercent (lowest at top).
type memMinHeap []memQuickInfo

func (h memMinHeap) Len() int            { return len(h) }
func (h memMinHeap) Less(i, j int) bool   { return h[i].memoryPercent < h[j].memoryPercent }
func (h memMinHeap) Swap(i, j int)        { h[i], h[j] = h[j], h[i] }
func (h *memMinHeap) Push(x interface{})  { *h = append(*h, x.(memQuickInfo)) }
func (h *memMinHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

// MemoryProcessCollector collects per-process memory usage metrics.
type MemoryProcessCollector struct {
	BaseCollector
	topN           int             // Number of top processes to report
	watchProcesses []string        // List of process names to always include
	matcher        *ProcessMatcher // For efficient process name matching
}

// NewMemoryProcessCollector creates a new memory process collector.
func NewMemoryProcessCollector() *MemoryProcessCollector {
	return &MemoryProcessCollector{
		BaseCollector: NewBaseCollector("MemoryProcess"),
		topN:          10,
	}
}

// DefaultConfig returns the default CollectorConfig for the memory process collector.
func (c *MemoryProcessCollector) DefaultConfig() config.CollectorConfig {
	cfg := c.BaseCollector.DefaultConfig()
	cfg.Interval = 30 * time.Second
	cfg.TopN = 10
	return cfg
}

// Configure applies the configuration to the collector.
func (c *MemoryProcessCollector) Configure(cfg config.CollectorConfig) error {
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

// Collect gathers per-process memory metrics using a 2-pass approach for performance.
// 1st pass: collect only memory info and name for all processes (minimal syscalls)
// 2nd pass: collect detailed info (username, createTime) only for selected processes
//
// Selection algorithm:
// 1. All watched processes are collected (no limit)
// 2. Remaining slots (topN - watched count) filled from top memory consumers
// 3. Output: watched processes first, then top N (no duplicates)
func (c *MemoryProcessCollector) Collect(ctx context.Context) (*MetricData, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}

	// 1st Pass: Memory% + Name + MemInfo only (3 syscalls per process).
	// Watched processes go to watchedList; non-watched go directly into a min-heap
	// of size remainingSlots for O(n log k) TopN selection.
	var watchedList []memQuickInfo
	h := &memMinHeap{}

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

		q := memQuickInfo{
			proc:          p,
			memoryPercent: memPercent,
			name:          name,
			rss:           memInfo.RSS,
			vms:           memInfo.VMS,
			swap:          memInfo.Swap,
			watched:       c.matcher.IsWatched(name),
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
			} else if q.memoryPercent > (*h)[0].memoryPercent {
				(*h)[0] = q
				heap.Fix(h, 0)
			}
		}
	}

	// Sort watched by memory descending (for consistent ordering)
	sort.Slice(watchedList, func(i, j int) bool {
		return watchedList[i].memoryPercent > watchedList[j].memoryPercent
	})

	// Extract heap items sorted descending
	heapItems := make([]memQuickInfo, h.Len())
	for i := h.Len() - 1; i >= 0; i-- {
		heapItems[i] = heap.Pop(h).(memQuickInfo)
	}

	// 2nd Pass: detailed info only for selected processes (2 syscalls per process)
	processList := make([]ProcessMemory, 0, len(watchedList)+len(heapItems))
	for _, q := range watchedList {
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
			Watched:       q.watched,
		})
	}
	for _, q := range heapItems {
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
			Watched:       q.watched,
		})
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      ProcessMemoryData{Processes: processList},
	}, nil
}
