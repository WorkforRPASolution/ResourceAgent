package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
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
// Platform-specific implementation is in temperature_windows.go and temperature_linux.go.
func (c *TemperatureCollector) Collect(ctx context.Context) (*MetricData, error) {
	sensors, err := c.collectTemperatures(ctx)
	if err != nil {
		log := logger.WithComponent("collector")
		log.Warn().Str("collector", c.Name()).Err(err).Msg("Failed to collect, returning empty data")
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      TemperatureData{Sensors: []TemperatureSensor{}},
		}, nil
	}

	if sensors == nil {
		sensors = []TemperatureSensor{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
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
