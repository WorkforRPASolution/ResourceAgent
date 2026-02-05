//go:build windows

package collector

import (
	"context"
)

// collectTemperatures collects CPU temperatures using LibreHardwareMonitor helper.
// Windows-specific implementation that uses shared LhmProvider for efficiency.
func (c *TemperatureCollector) collectTemperatures(ctx context.Context) ([]TemperatureSensor, error) {
	provider := GetLhmProvider()
	data, err := provider.GetData(ctx)
	if err != nil {
		return nil, err
	}

	var sensors []TemperatureSensor
	for _, s := range data.Sensors {
		// Skip if specific zones are configured and this one isn't in the list
		if len(c.includeZones) > 0 && !c.shouldInclude(s.Name) {
			continue
		}

		sensors = append(sensors, TemperatureSensor{
			Name:        s.Name,
			Temperature: s.Temperature,
			High:        s.High,
			Critical:    s.Critical,
		})
	}

	return sensors, nil
}
