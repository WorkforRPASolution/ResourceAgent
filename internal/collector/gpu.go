package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// GpuCollector collects GPU metrics.
type GpuCollector struct {
	BaseCollector
	includeGpus []string // Specific GPUs to include; empty means all
}

// NewGpuCollector creates a new GPU collector.
func NewGpuCollector() *GpuCollector {
	return &GpuCollector{
		BaseCollector: NewBaseCollector("GPU"),
	}
}

// DefaultConfig returns the default CollectorConfig for the GPU collector.
func (c *GpuCollector) DefaultConfig() config.CollectorConfig {
	cfg := c.BaseCollector.DefaultConfig()
	cfg.Interval = 30 * time.Second
	return cfg
}

// Configure applies the configuration to the collector.
func (c *GpuCollector) Configure(cfg config.CollectorConfig) error {
	c.SetEnabled(cfg.Enabled)
	if cfg.Interval > 0 {
		c.SetInterval(cfg.Interval)
	}
	c.includeGpus = cfg.IncludeZones // Reuse IncludeZones for GPU filtering
	return nil
}

// Collect gathers GPU metrics.
// Platform-specific implementation is in gpu_windows.go and gpu_unix.go.
func (c *GpuCollector) Collect(ctx context.Context) (*MetricData, error) {
	gpus, err := c.collectGpuMetrics(ctx)
	if err != nil {
		log := logger.WithComponent("collector")
		log.Warn().Str("collector", c.Name()).Err(err).Msg("Failed to collect, returning empty data")
		return &MetricData{
			Type:      c.Name(),
			Timestamp: time.Now(),
			Data:      GpuData{Gpus: []GpuSensor{}},
		}, nil
	}

	if gpus == nil {
		gpus = []GpuSensor{}
	}

	return &MetricData{
		Type:      c.Name(),
		Timestamp: time.Now(),
		Data:      GpuData{Gpus: gpus},
	}, nil
}

func (c *GpuCollector) shouldInclude(gpuName string) bool {
	for _, name := range c.includeGpus {
		if name == gpuName {
			return true
		}
	}
	return false
}
