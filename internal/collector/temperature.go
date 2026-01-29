package collector

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/host"

	"resourceagent/internal/config"
)

// TemperatureCollector collects system temperature metrics.
type TemperatureCollector struct {
	BaseCollector
	includeZones []string // Specific thermal zones to include; empty means all
}

// NewTemperatureCollector creates a new temperature collector.
func NewTemperatureCollector() *TemperatureCollector {
	return &TemperatureCollector{
		BaseCollector: NewBaseCollector("temperature"),
	}
}

// Configure applies the configuration to the collector.
func (c *TemperatureCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeZones = cfg.IncludeZones
	return nil
}

// Collect gathers temperature metrics.
func (c *TemperatureCollector) Collect(ctx context.Context) (*MetricData, error) {
	temps, err := host.SensorsTemperaturesWithContext(ctx)
	if err != nil {
		// Temperature sensors may not be available on all systems
		// Return empty data rather than error
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now().UTC(),
			Data:      TemperatureData{Sensors: []TemperatureSensor{}},
		}, nil
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

		sensor := TemperatureSensor{
			Name:        temp.SensorKey,
			Temperature: temp.Temperature,
			High:        temp.High,
			Critical:    temp.Critical,
		}

		sensors = append(sensors, sensor)
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now().UTC(),
		Data:      TemperatureData{Sensors: sensors},
	}, nil
}

func (c *TemperatureCollector) shouldInclude(sensorKey string) bool {
	for _, zone := range c.includeZones {
		if zone == sensorKey {
			return true
		}
	}
	return false
}
