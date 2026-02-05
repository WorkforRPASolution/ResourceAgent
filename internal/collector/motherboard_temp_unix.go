//go:build linux || darwin

package collector

import (
	"context"
)

// collectMotherboardTemps returns empty data on Linux/Darwin.
// Future: Could implement hwmon/sysfs parsing for motherboard temperature sensors on Linux.
func (c *MotherboardTempCollector) collectMotherboardTemps(ctx context.Context) ([]MotherboardTempSensor, error) {
	// Motherboard temperature metrics via LHM are only available on Windows
	// Return nil to indicate no data (not an error)
	return nil, nil
}
