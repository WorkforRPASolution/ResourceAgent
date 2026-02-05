//go:build windows

package collector

import (
	"context"
)

// collectMotherboardTemps collects motherboard temperature metrics using LibreHardwareMonitor helper.
// Windows-specific implementation that uses shared LhmProvider for efficiency.
func (c *MotherboardTempCollector) collectMotherboardTemps(ctx context.Context) ([]MotherboardTempSensor, error) {
	provider := GetLhmProvider()
	data, err := provider.GetData(ctx)
	if err != nil {
		return nil, err
	}

	var sensors []MotherboardTempSensor
	for _, t := range data.MotherboardTemps {
		// Skip if specific sensors are configured and this one isn't in the list
		if len(c.includeSensors) > 0 && !c.shouldInclude(t.Name) {
			continue
		}

		sensors = append(sensors, MotherboardTempSensor{
			Name:        t.Name,
			Temperature: t.Temperature,
		})
	}

	return sensors, nil
}
