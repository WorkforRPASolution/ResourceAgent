package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// VoltageCollector collects voltage sensor metrics.
type VoltageCollector struct {
	BaseCollector
	includeSensors []string // Specific sensors to include; empty means all
}

// NewVoltageCollector creates a new voltage collector.
func NewVoltageCollector() *VoltageCollector {
	return &VoltageCollector{
		BaseCollector: NewBaseCollector("voltage"),
	}
}

// Configure applies the configuration to the collector.
func (c *VoltageCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeSensors = cfg.IncludeZones // Reuse IncludeZones for sensor filtering
	return nil
}

// Collect gathers voltage metrics.
// Platform-specific implementation is in voltage_windows.go and voltage_unix.go.
func (c *VoltageCollector) Collect(ctx context.Context) (*MetricData, error) {
	sensors, err := c.collectVoltageMetrics(ctx)
	if err != nil {
		log := logger.WithComponent("collector")
		log.Warn().Str("collector", c.Name()).Err(err).Msg("Failed to collect, returning empty data")
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      VoltageData{Sensors: []VoltageSensor{}},
		}, nil
	}

	if sensors == nil {
		sensors = []VoltageSensor{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      VoltageData{Sensors: sensors},
	}, nil
}

func (c *VoltageCollector) shouldInclude(sensorName string) bool {
	for _, name := range c.includeSensors {
		if name == sensorName {
			return true
		}
	}
	return false
}
