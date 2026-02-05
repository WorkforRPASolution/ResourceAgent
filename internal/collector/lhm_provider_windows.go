//go:build windows

package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// LhmData represents the complete JSON output from LhmHelper.exe.
// All LHM-based collectors share this cached data.
type LhmData struct {
	Sensors         []LhmSensor         `json:"Sensors"`
	Fans            []LhmFan            `json:"Fans"`
	Gpus            []LhmGpu            `json:"Gpus"`
	Storages        []LhmStorage        `json:"Storages"`
	Voltages        []LhmVoltage        `json:"Voltages"`
	MotherboardTemps []LhmMotherboardTemp `json:"MotherboardTemps"`
	Error           string              `json:"error,omitempty"`
}

// LhmSensor represents CPU temperature sensor data.
type LhmSensor struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
	High        float64 `json:"High"`
	Critical    float64 `json:"Critical"`
}

// LhmFan represents fan sensor data.
type LhmFan struct {
	Name string  `json:"Name"`
	RPM  float64 `json:"RPM"`
}

// LhmGpu represents GPU sensor data.
type LhmGpu struct {
	Name        string   `json:"Name"`
	Temperature *float64 `json:"Temperature"`
	CoreLoad    *float64 `json:"CoreLoad"`
	MemoryLoad  *float64 `json:"MemoryLoad"`
	FanSpeed    *float64 `json:"FanSpeed"`
	Power       *float64 `json:"Power"`
	CoreClock   *float64 `json:"CoreClock"`
	MemoryClock *float64 `json:"MemoryClock"`
}

// LhmStorage represents S.M.A.R.T storage data.
type LhmStorage struct {
	Name              string   `json:"Name"`
	Type              string   `json:"Type"`
	Temperature       *float64 `json:"Temperature"`
	RemainingLife     *float64 `json:"RemainingLife"`
	MediaErrors       *int64   `json:"MediaErrors"`
	PowerCycles       *int64   `json:"PowerCycles"`
	UnsafeShutdowns   *int64   `json:"UnsafeShutdowns"`
	PowerOnHours      *int64   `json:"PowerOnHours"`
	TotalBytesWritten *int64   `json:"TotalBytesWritten"`
}

// LhmVoltage represents voltage sensor data.
type LhmVoltage struct {
	Name    string  `json:"Name"`
	Voltage float64 `json:"Voltage"`
}

// LhmMotherboardTemp represents motherboard temperature sensor data.
type LhmMotherboardTemp struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
}

// LhmProvider provides cached access to LhmHelper.exe output.
// Thread-safe singleton that all LHM-based collectors share.
type LhmProvider struct {
	mu          sync.RWMutex
	data        *LhmData
	lastUpdate  time.Time
	cacheTTL    time.Duration
	helperPath  string
	helperFound bool
}

var (
	lhmProviderInstance *LhmProvider
	lhmProviderOnce     sync.Once
)

// GetLhmProvider returns the singleton LhmProvider instance.
func GetLhmProvider() *LhmProvider {
	lhmProviderOnce.Do(func() {
		lhmProviderInstance = &LhmProvider{
			cacheTTL: 5 * time.Second, // Default cache TTL
		}
	})
	return lhmProviderInstance
}

// SetCacheTTL sets the cache time-to-live duration.
func (p *LhmProvider) SetCacheTTL(ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cacheTTL = ttl
}

// GetData returns cached LhmHelper data, refreshing if stale.
func (p *LhmProvider) GetData(ctx context.Context) (*LhmData, error) {
	p.mu.RLock()
	if p.data != nil && time.Since(p.lastUpdate) < p.cacheTTL {
		data := p.data
		p.mu.RUnlock()
		return data, nil
	}
	p.mu.RUnlock()

	// Need to refresh - acquire write lock
	p.mu.Lock()
	defer p.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine may have refreshed)
	if p.data != nil && time.Since(p.lastUpdate) < p.cacheTTL {
		return p.data, nil
	}

	// Find helper if not already found
	if !p.helperFound {
		path, err := p.findLhmHelper()
		if err != nil {
			return nil, err
		}
		p.helperPath = path
		p.helperFound = true
	}

	// Execute LhmHelper
	data, err := p.executeLhmHelper(ctx)
	if err != nil {
		return nil, err
	}

	p.data = data
	p.lastUpdate = time.Now()
	return data, nil
}

// Invalidate clears the cache, forcing a refresh on next GetData call.
func (p *LhmProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = nil
	p.lastUpdate = time.Time{}
}

// executeLhmHelper runs LhmHelper.exe and parses its output.
func (p *LhmProvider) executeLhmHelper(ctx context.Context) (*LhmData, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, p.helperPath)
	output, err := cmd.Output()
	if err != nil {
		if cmdCtx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("LhmHelper timed out")
		}
		return nil, fmt.Errorf("LhmHelper execution failed: %w", err)
	}

	var data LhmData
	if err := json.Unmarshal(output, &data); err != nil {
		return nil, fmt.Errorf("failed to parse LhmHelper output: %w", err)
	}

	if data.Error != "" {
		return nil, fmt.Errorf("LhmHelper error: %s", data.Error)
	}

	return &data, nil
}

// findLhmHelper searches for LhmHelper.exe in common locations.
func (p *LhmProvider) findLhmHelper() (string, error) {
	candidates := []string{
		"LhmHelper.exe",
		"./LhmHelper.exe",
		filepath.Join(".", "tools", "LhmHelper.exe"),
	}

	if exePath, err := exec.LookPath("resourceagent.exe"); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "LhmHelper.exe"),
			filepath.Join(exeDir, "tools", "LhmHelper.exe"),
		)
	}

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
