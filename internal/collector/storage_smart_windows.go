//go:build windows

package collector

import (
	"context"
)

// collectStorageMetrics collects S.M.A.R.T metrics using LibreHardwareMonitor helper.
// Windows-specific implementation that uses shared LhmProvider for efficiency.
func (c *StorageSmartCollector) collectStorageMetrics(ctx context.Context) ([]StorageSmartSensor, error) {
	provider := GetLhmProvider()
	data, err := provider.GetData(ctx)
	if err != nil {
		return nil, err
	}

	var storages []StorageSmartSensor
	for _, s := range data.Storages {
		// Skip if specific drives are configured and this one isn't in the list
		if len(c.includeDrives) > 0 && !c.shouldInclude(s.Name) {
			continue
		}

		storages = append(storages, StorageSmartSensor{
			Name:              s.Name,
			Type:              s.Type,
			Temperature:       s.Temperature,
			RemainingLife:     s.RemainingLife,
			MediaErrors:       s.MediaErrors,
			PowerCycles:       s.PowerCycles,
			UnsafeShutdowns:   s.UnsafeShutdowns,
			PowerOnHours:      s.PowerOnHours,
			TotalBytesWritten: s.TotalBytesWritten,
		})
	}

	return storages, nil
}
