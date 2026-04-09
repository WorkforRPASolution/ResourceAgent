package collector

import (
	"context"
	"strings"
	"time"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// StorageHealthCollector collects simple OK/DEGRADED/PRED_FAIL/FAIL/UNKNOWN health status
// for storage devices. Unlike StorageSmartCollector, this does not depend on LhmHelper.
//   - Windows: WMI Win32_DiskDrive.Status
//   - Linux: smartctl -H
//   - Darwin: returns empty data (development/test only)
type StorageHealthCollector struct {
	BaseCollector
	includeDrives []string
}

// NewStorageHealthCollector creates a new storage health collector.
func NewStorageHealthCollector() *StorageHealthCollector {
	return &StorageHealthCollector{
		BaseCollector: NewBaseCollector("StorageHealth"),
	}
}

// DefaultConfig returns the default CollectorConfig for the storage health collector.
func (c *StorageHealthCollector) DefaultConfig() config.CollectorConfig {
	cfg := c.BaseCollector.DefaultConfig()
	cfg.Interval = 300 * time.Second
	return cfg
}

// Configure applies the configuration to the collector.
func (c *StorageHealthCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeDrives = cfg.Disks
	c.platformConfigure()
	return nil
}

// Collect gathers storage health status.
// Platform-specific implementation is in storage_health_windows.go and storage_health_unix.go.
func (c *StorageHealthCollector) Collect(ctx context.Context) (*MetricData, error) {
	disks, err := c.collectHealthStatus(ctx)
	if err != nil {
		log := logger.WithComponent("collector")
		log.Warn().Str("collector", c.Name()).Err(err).Msg("Failed to collect, returning empty data")
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      StorageHealthData{Disks: []StorageHealthDisk{}},
		}, nil
	}

	if disks == nil {
		disks = []StorageHealthDisk{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      StorageHealthData{Disks: disks},
	}, nil
}

func (c *StorageHealthCollector) shouldInclude(driveName string) bool {
	for _, name := range c.includeDrives {
		if name == driveName {
			return true
		}
	}
	return false
}

// normalizeHealthStatus converts platform-specific status strings to a 5-level enum.
//
//	OK        (0) — normal
//	DEGRADED  (1) — working but degraded performance
//	PRED_FAIL (2) — S.M.A.R.T predicted failure, replace soon
//	FAIL      (3) — error/failure, immediate action needed
//	UNKNOWN  (-1) — cannot determine
func normalizeHealthStatus(raw string) string {
	upper := strings.ToUpper(strings.TrimSpace(raw))
	switch upper {
	case "OK", "PASSED":
		return "OK"
	case "DEGRADED", "STRESSED":
		return "DEGRADED"
	case "PRED FAIL":
		return "PRED_FAIL"
	case "":
		return "UNKNOWN"
	default:
		// Check for FAIL-class statuses (may contain extra chars like "FAILED!")
		for _, failToken := range []string{"FAIL", "ERROR", "NONRECOVER", "LOST COMM", "NO CONTACT"} {
			if strings.Contains(upper, failToken) {
				return "FAIL"
			}
		}
		if upper == "UNKNOWN" {
			return "UNKNOWN"
		}
		return "UNKNOWN"
	}
}

// HealthStatusValue returns the numeric value for EARS output.
func HealthStatusValue(status string) float64 {
	switch status {
	case "OK":
		return 0
	case "DEGRADED":
		return 1
	case "PRED_FAIL":
		return 2
	case "FAIL":
		return 3
	default: // UNKNOWN
		return -1
	}
}
