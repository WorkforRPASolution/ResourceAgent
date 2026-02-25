package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
)

// StorageSmartCollector collects S.M.A.R.T metrics from storage devices.
type StorageSmartCollector struct {
	BaseCollector
	includeDrives []string // Specific drives to include; empty means all
}

// NewStorageSmartCollector creates a new S.M.A.R.T collector.
func NewStorageSmartCollector() *StorageSmartCollector {
	return &StorageSmartCollector{
		BaseCollector: NewBaseCollector("storage_smart"),
	}
}

// Configure applies the configuration to the collector.
func (c *StorageSmartCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeDrives = cfg.Disks // Reuse Disks for drive filtering
	return nil
}

// Collect gathers S.M.A.R.T metrics.
// Platform-specific implementation is in storage_smart_windows.go and storage_smart_unix.go.
func (c *StorageSmartCollector) Collect(ctx context.Context) (*MetricData, error) {
	storages, err := c.collectStorageMetrics(ctx)
	if err != nil {
		// Log error but return empty data rather than failing
		// S.M.A.R.T data may not be available on all systems
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      StorageSmartData{Storages: []StorageSmartSensor{}},
		}, nil
	}

	if storages == nil {
		storages = []StorageSmartSensor{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      StorageSmartData{Storages: storages},
	}, nil
}

func (c *StorageSmartCollector) shouldInclude(driveName string) bool {
	for _, name := range c.includeDrives {
		if name == driveName {
			return true
		}
	}
	return false
}
