//go:build linux || darwin

package collector

import (
	"context"
)

// collectVoltageMetrics returns empty data on Linux/Darwin.
// Future: Could implement hwmon/sysfs parsing for voltage sensors on Linux.
func (c *VoltageCollector) collectVoltageMetrics(ctx context.Context) ([]VoltageSensor, error) {
	// Voltage metrics via LHM are only available on Windows
	// Return nil to indicate no data (not an error)
	return nil, nil
}
