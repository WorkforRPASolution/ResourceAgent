package collector

import (
	"context"
	"time"

	"github.com/shirou/gopsutil/v3/host"

	"resourceagent/internal/config"
)

// UptimeCollector collects system uptime and boot time metrics.
type UptimeCollector struct {
	BaseCollector
}

// NewUptimeCollector creates a new uptime collector.
func NewUptimeCollector() *UptimeCollector {
	return &UptimeCollector{
		BaseCollector: NewBaseCollector("uptime"),
	}
}

// Configure applies the configuration to the collector.
func (c *UptimeCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	return nil
}

// Collect gathers system uptime and boot time metrics.
func (c *UptimeCollector) Collect(ctx context.Context) (*MetricData, error) {
	bootTimestamp, err := host.BootTimeWithContext(ctx)
	if err != nil {
		return nil, err
	}

	bootTime := time.Unix(int64(bootTimestamp), 0)
	uptimeMinutes := time.Since(bootTime).Minutes()

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data: UptimeData{
			BootTimeUnix:  int64(bootTimestamp),
			BootTimeStr:   bootTime.Format("2006-01-02T15:04:05"),
			UptimeMinutes: uptimeMinutes,
		},
	}, nil
}
