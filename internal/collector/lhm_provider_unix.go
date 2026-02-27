//go:build linux || darwin

package collector

import (
	"context"
	"time"
)

// Start is a no-op on Unix systems. LhmHelper is Windows-only.
func (p *LhmProvider) Start(ctx context.Context) error {
	return nil
}

// Stop is a no-op on Unix systems.
func (p *LhmProvider) Stop() {}

// LhmData represents the complete JSON output from LhmHelper.exe.
// On Unix systems, this returns empty data as LhmHelper is Windows-only.
type LhmData struct {
	Sensors          []LhmSensor          `json:"Sensors"`
	Fans             []LhmFan             `json:"Fans"`
	Gpus             []LhmGpu             `json:"Gpus"`
	Storages         []LhmStorage         `json:"Storages"`
	Voltages         []LhmVoltage         `json:"Voltages"`
	MotherboardTemps []LhmMotherboardTemp `json:"MotherboardTemps"`
	Error            string               `json:"error,omitempty"`
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
// On Unix systems, this is a stub that returns empty data.
type LhmProvider struct {
	cacheTTL time.Duration
}

var lhmProviderInstance = &LhmProvider{
	cacheTTL: 5 * time.Second,
}

// GetLhmProvider returns the singleton LhmProvider instance.
func GetLhmProvider() *LhmProvider {
	return lhmProviderInstance
}

// SetCacheTTL sets the cache time-to-live duration.
func (p *LhmProvider) SetCacheTTL(ttl time.Duration) {
	p.cacheTTL = ttl
}

// GetData returns empty LhmData on Unix systems.
// LhmHelper is Windows-only; Unix collectors use native methods.
func (p *LhmProvider) GetData(ctx context.Context) (*LhmData, error) {
	return &LhmData{}, nil
}

// Invalidate is a no-op on Unix systems.
func (p *LhmProvider) Invalidate() {}
