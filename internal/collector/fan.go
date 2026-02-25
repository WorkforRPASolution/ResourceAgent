package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
)

// FanCollector collects system fan speed metrics.
type FanCollector struct {
	BaseCollector
	includeFans []string // Specific fans to include; empty means all
}

// NewFanCollector creates a new fan speed collector.
func NewFanCollector() *FanCollector {
	return &FanCollector{
		BaseCollector: NewBaseCollector("fan"),
	}
}

// Configure applies the configuration to the collector.
func (c *FanCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeFans = cfg.IncludeZones // Reuse IncludeZones for fan filtering
	return nil
}

// Collect gathers fan speed metrics.
// Platform-specific implementation is in fan_windows.go and fan_linux.go.
func (c *FanCollector) Collect(ctx context.Context) (*MetricData, error) {
	sensors, err := c.collectFanSpeeds(ctx)
	if err != nil {
		// Log error but return empty data rather than failing
		// Fan sensors may not be available on all systems
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      FanData{Sensors: []FanSensor{}},
		}, nil
	}

	if sensors == nil {
		sensors = []FanSensor{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      FanData{Sensors: sensors},
	}, nil
}

func (c *FanCollector) shouldInclude(fanName string) bool {
	for _, name := range c.includeFans {
		if name == fanName {
			return true
		}
	}
	return false
}
