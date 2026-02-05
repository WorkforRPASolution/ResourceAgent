//go:build linux || darwin

package collector

import (
	"context"
)

// collectStorageMetrics returns empty data on Linux/Darwin.
// Future: Could implement smartctl parsing for S.M.A.R.T data on Linux.
func (c *StorageSmartCollector) collectStorageMetrics(ctx context.Context) ([]StorageSmartSensor, error) {
	// S.M.A.R.T metrics via LHM are only available on Windows
	// Return nil to indicate no data (not an error)
	return nil, nil
}
