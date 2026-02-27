package collector

import (
	"context"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"resourceagent/internal/config"
)

// ProcessWatchCollector monitors required and forbidden processes.
// Required processes should always be running; forbidden processes should never run.
type ProcessWatchCollector struct {
	BaseCollector
	requiredProcesses  []string
	forbiddenProcesses []string
	requiredMatcher    *ProcessMatcher
	forbiddenMatcher   *ProcessMatcher
}

// NewProcessWatchCollector creates a new process watch collector.
func NewProcessWatchCollector() *ProcessWatchCollector {
	return &ProcessWatchCollector{
		BaseCollector: NewBaseCollector("ProcessWatch"),
	}
}

// Configure applies the configuration to the collector.
func (c *ProcessWatchCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.requiredProcesses = cfg.RequiredProcesses
	c.forbiddenProcesses = cfg.ForbiddenProcesses
	c.requiredMatcher = NewProcessMatcher(cfg.RequiredProcesses)
	c.forbiddenMatcher = NewProcessMatcher(cfg.ForbiddenProcesses)
	return nil
}

// Collect checks the running status of all watched processes.
func (c *ProcessWatchCollector) Collect(ctx context.Context) (*MetricData, error) {
	// Build a map of matched process name → PID from running processes
	pidMap := make(map[string]int32) // lowered/original name → PID

	if c.requiredMatcher.HasWatchList() || c.forbiddenMatcher.HasWatchList() {
		procs, err := process.ProcessesWithContext(ctx)
		if err != nil {
			return nil, err
		}

		for _, p := range procs {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			default:
			}

			name, err := p.NameWithContext(ctx)
			if err != nil || name == "" {
				continue
			}

			if c.requiredMatcher.IsWatched(name) || c.forbiddenMatcher.IsWatched(name) {
				pidMap[name] = p.Pid
			}
		}
	}

	statuses := make([]ProcessWatchStatus, 0, len(c.requiredProcesses)+len(c.forbiddenProcesses))

	// Check required processes
	for _, name := range c.requiredProcesses {
		pid, running := findPID(pidMap, name, c.requiredMatcher)
		statuses = append(statuses, ProcessWatchStatus{
			Name:    name,
			PID:     pid,
			Running: running,
			Type:    "required",
		})
	}

	// Check forbidden processes
	for _, name := range c.forbiddenProcesses {
		pid, running := findPID(pidMap, name, c.forbiddenMatcher)
		statuses = append(statuses, ProcessWatchStatus{
			Name:    name,
			PID:     pid,
			Running: running,
			Type:    "forbidden",
		})
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      ProcessWatchData{Statuses: statuses},
	}, nil
}

// findPID looks up a process name in the PID map using the matcher's case sensitivity.
func findPID(pidMap map[string]int32, configName string, matcher *ProcessMatcher) (int32, bool) {
	for runningName, pid := range pidMap {
		if matcher.IsWatched(runningName) && matchesConfigName(configName, runningName, matcher) {
			return pid, true
		}
	}
	return 0, false
}

// matchesConfigName checks if a running process name matches the config-specified name.
func matchesConfigName(configName, runningName string, matcher *ProcessMatcher) bool {
	if matcher.caseInsensitive {
		return strings.EqualFold(configName, runningName)
	}
	return configName == runningName
}
