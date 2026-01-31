//go:build windows

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"
)

// lhmHelperResult represents the JSON output from LhmHelper.exe
type lhmHelperResult struct {
	Sensors []lhmSensor `json:"Sensors"`
	Error   string      `json:"error,omitempty"`
}

type lhmSensor struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
	High        float64 `json:"High"`
	Critical    float64 `json:"Critical"`
}

// collectTemperatures collects CPU temperatures using LibreHardwareMonitor helper.
// Windows-specific implementation that calls LhmHelper.exe.
func (c *TemperatureCollector) collectTemperatures(ctx context.Context) ([]TemperatureSensor, error) {
	// Find LhmHelper.exe in the same directory as the agent or in PATH
	helperPath, err := findLhmHelper()
	if err != nil {
		return nil, fmt.Errorf("LhmHelper not found: %w", err)
	}

	// Execute with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, helperPath)
	output, err := cmd.Output()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("LhmHelper timed out")
		}
		return nil, fmt.Errorf("LhmHelper execution failed: %w", err)
	}

	var result lhmHelperResult
	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse LhmHelper output: %w", err)
	}

	if result.Error != "" {
		return nil, fmt.Errorf("LhmHelper error: %s", result.Error)
	}

	var sensors []TemperatureSensor
	for _, s := range result.Sensors {
		// Skip if specific zones are configured and this one isn't in the list
		if len(c.includeZones) > 0 && !c.shouldInclude(s.Name) {
			continue
		}

		sensors = append(sensors, TemperatureSensor{
			Name:        s.Name,
			Temperature: s.Temperature,
			High:        s.High,
			Critical:    s.Critical,
		})
	}

	return sensors, nil
}

// findLhmHelper searches for LhmHelper.exe in common locations.
func findLhmHelper() (string, error) {
	// Check common locations
	candidates := []string{
		"LhmHelper.exe",                           // Current directory or PATH
		"./LhmHelper.exe",                         // Explicit current directory
		filepath.Join(".", "tools", "LhmHelper.exe"), // Relative tools directory
	}

	// Get the executable's directory
	if exePath, err := exec.LookPath("resourceagent.exe"); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "LhmHelper.exe"),
			filepath.Join(exeDir, "tools", "LhmHelper.exe"),
		)
	}

	// Add Program Files location
	candidates = append(candidates,
		`C:\Program Files\ResourceAgent\LhmHelper.exe`,
		`C:\Program Files\ResourceAgent\tools\LhmHelper.exe`,
	)

	for _, path := range candidates {
		if fullPath, err := exec.LookPath(path); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("LhmHelper.exe not found in any expected location")
}
