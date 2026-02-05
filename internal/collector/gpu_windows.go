//go:build windows

package collector

import (
	"context"
)

// collectGpuMetrics collects GPU metrics using LibreHardwareMonitor helper.
// Windows-specific implementation that uses shared LhmProvider for efficiency.
func (c *GpuCollector) collectGpuMetrics(ctx context.Context) ([]GpuSensor, error) {
	provider := GetLhmProvider()
	data, err := provider.GetData(ctx)
	if err != nil {
		return nil, err
	}

	var gpus []GpuSensor
	for _, g := range data.Gpus {
		// Skip if specific GPUs are configured and this one isn't in the list
		if len(c.includeGpus) > 0 && !c.shouldInclude(g.Name) {
			continue
		}

		gpus = append(gpus, GpuSensor{
			Name:        g.Name,
			Temperature: g.Temperature,
			CoreLoad:    g.CoreLoad,
			MemoryLoad:  g.MemoryLoad,
			FanSpeed:    g.FanSpeed,
			Power:       g.Power,
			CoreClock:   g.CoreClock,
			MemoryClock: g.MemoryClock,
		})
	}

	return gpus, nil
}
