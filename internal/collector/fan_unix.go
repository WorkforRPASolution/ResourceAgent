//go:build linux || darwin

package collector

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// collectFanSpeeds collects fan speeds from sysfs on Linux.
// On Darwin (macOS), returns empty as sysfs is not available.
func (c *FanCollector) collectFanSpeeds(ctx context.Context) ([]FanSensor, error) {
	// Darwin (macOS) doesn't have sysfs for fan data
	if runtime.GOOS == "darwin" {
		return nil, nil
	}

	var sensors []FanSensor

	// Find all hwmon devices
	hwmonPath := "/sys/class/hwmon"
	entries, err := os.ReadDir(hwmonPath)
	if err != nil {
		return nil, nil // hwmon may not be available
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		devicePath := filepath.Join(hwmonPath, entry.Name())

		// Get device name for labeling
		deviceName := getHwmonDeviceName(devicePath)

		// Find all fan*_input files
		fanFiles, err := filepath.Glob(filepath.Join(devicePath, "fan*_input"))
		if err != nil {
			continue
		}

		for _, fanFile := range fanFiles {
			// Check context cancellation
			select {
			case <-ctx.Done():
				return sensors, ctx.Err()
			default:
			}

			// Read RPM value
			data, err := os.ReadFile(fanFile)
			if err != nil {
				continue
			}

			rpmStr := strings.TrimSpace(string(data))
			rpm, err := strconv.ParseFloat(rpmStr, 64)
			if err != nil {
				continue
			}

			// Skip invalid readings
			if rpm < 0 {
				continue
			}

			// Get fan label if available
			fanName := getFanLabel(fanFile, deviceName)

			// Skip if specific fans are configured and this one isn't in the list
			if len(c.includeFans) > 0 && !c.shouldInclude(fanName) {
				continue
			}

			sensors = append(sensors, FanSensor{
				Name: fanName,
				RPM:  rpm,
			})
		}
	}

	return sensors, nil
}

// getHwmonDeviceName reads the device name from hwmon directory.
func getHwmonDeviceName(devicePath string) string {
	// Try to read the name file
	nameFile := filepath.Join(devicePath, "name")
	data, err := os.ReadFile(nameFile)
	if err != nil {
		return filepath.Base(devicePath)
	}
	return strings.TrimSpace(string(data))
}

// getFanLabel returns a label for the fan sensor.
func getFanLabel(fanInputPath, deviceName string) string {
	// Extract fan number from path (e.g., fan1_input -> 1)
	base := filepath.Base(fanInputPath)
	fanNum := strings.TrimSuffix(strings.TrimPrefix(base, "fan"), "_input")

	// Try to read fan label file (e.g., fan1_label)
	labelFile := strings.Replace(fanInputPath, "_input", "_label", 1)
	data, err := os.ReadFile(labelFile)
	if err == nil {
		label := strings.TrimSpace(string(data))
		if label != "" {
			return label
		}
	}

	// Fall back to device name + fan number
	return deviceName + " Fan #" + fanNum
}
