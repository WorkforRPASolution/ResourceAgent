//go:build windows

package collector

import (
	"context"
)

// collectFanSpeeds collects fan speeds using LibreHardwareMonitor helper.
// Windows-specific implementation that uses shared LhmProvider for efficiency.
func (c *FanCollector) collectFanSpeeds(ctx context.Context) ([]FanSensor, error) {
	provider := GetLhmProvider()
	data, err := provider.GetData(ctx)
	if err != nil {
		return nil, err
	}

	var sensors []FanSensor
	for _, f := range data.Fans {
		// Skip if specific fans are configured and this one isn't in the list
		if len(c.includeFans) > 0 && !c.shouldInclude(f.Name) {
			continue
		}

		sensors = append(sensors, FanSensor{
			Name: f.Name,
			RPM:  f.RPM,
		})
	}

	return sensors, nil
}
