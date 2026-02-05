//go:build windows

package collector

import (
	"context"
)

// collectVoltageMetrics collects voltage metrics using LibreHardwareMonitor helper.
// Windows-specific implementation that uses shared LhmProvider for efficiency.
func (c *VoltageCollector) collectVoltageMetrics(ctx context.Context) ([]VoltageSensor, error) {
	provider := GetLhmProvider()
	data, err := provider.GetData(ctx)
	if err != nil {
		return nil, err
	}

	var sensors []VoltageSensor
	for _, v := range data.Voltages {
		// Skip if specific sensors are configured and this one isn't in the list
		if len(c.includeSensors) > 0 && !c.shouldInclude(v.Name) {
			continue
		}

		sensors = append(sensors, VoltageSensor{
			Name:    v.Name,
			Voltage: v.Voltage,
		})
	}

	return sensors, nil
}
