//go:build linux || darwin

package collector

import (
	"context"
)

// collectGpuMetrics returns empty data on Linux/Darwin.
// Future: Could implement nvidia-smi parsing for NVIDIA GPUs on Linux.
func (c *GpuCollector) collectGpuMetrics(ctx context.Context) ([]GpuSensor, error) {
	// GPU metrics via LHM are only available on Windows
	// Return nil to indicate no data (not an error)
	return nil, nil
}
