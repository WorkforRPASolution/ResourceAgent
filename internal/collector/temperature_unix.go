//go:build linux || darwin

package collector

import (
	"context"

	"github.com/shirou/gopsutil/v3/host"
)

// collectTemperatures collects CPU temperatures using gopsutil.
// Linux-specific implementation that reads from /sys/class/thermal or lm-sensors.
func (c *TemperatureCollector) collectTemperatures(ctx context.Context) ([]TemperatureSensor, error) {
	temps, err := host.SensorsTemperaturesWithContext(ctx)
	if err != nil {
		// Temperature sensors may not be available on all systems
		return nil, nil
	}

	var sensors []TemperatureSensor

	for _, temp := range temps {
		// Skip if specific zones are configured and this one isn't in the list
		if len(c.includeZones) > 0 && !c.shouldInclude(temp.SensorKey) {
			continue
		}

		// Skip sensors with zero or invalid readings
		if temp.Temperature <= 0 || temp.Temperature > 200 {
			continue
		}

		sensors = append(sensors, TemperatureSensor{
			Name:        temp.SensorKey,
			Temperature: temp.Temperature,
			High:        temp.High,
			Critical:    temp.Critical,
		})
	}

	return sensors, nil
}
