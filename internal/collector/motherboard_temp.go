package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
)

// MotherboardTempCollector collects motherboard temperature metrics.
type MotherboardTempCollector struct {
	BaseCollector
	includeSensors []string // Specific sensors to include; empty means all
}

// NewMotherboardTempCollector creates a new motherboard temperature collector.
func NewMotherboardTempCollector() *MotherboardTempCollector {
	return &MotherboardTempCollector{
		BaseCollector: NewBaseCollector("motherboard_temp"),
	}
}

// Configure applies the configuration to the collector.
func (c *MotherboardTempCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeSensors = cfg.IncludeZones // Reuse IncludeZones for sensor filtering
	return nil
}

// Collect gathers motherboard temperature metrics.
// Platform-specific implementation is in motherboard_temp_windows.go and motherboard_temp_unix.go.
func (c *MotherboardTempCollector) Collect(ctx context.Context) (*MetricData, error) {
	sensors, err := c.collectMotherboardTemps(ctx)
	if err != nil {
		// Log error but return empty data rather than failing
		// Motherboard temperature sensors may not be available on all systems
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      MotherboardTempData{Sensors: []MotherboardTempSensor{}},
		}, nil
	}

	if sensors == nil {
		sensors = []MotherboardTempSensor{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      MotherboardTempData{Sensors: sensors},
	}, nil
}

func (c *MotherboardTempCollector) shouldInclude(sensorName string) bool {
	for _, name := range c.includeSensors {
		if name == sensorName {
			return true
		}
	}
	return false
}
