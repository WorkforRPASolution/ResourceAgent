// Package collector provides interfaces and implementations for system metrics collection.
package collector

import (
	"context"
	"time"

	"resourceagent/internal/config"
)

// Collector defines the interface for all metric collectors.
type Collector interface {
	// Name returns the unique identifier for this collector.
	Name() string

	// Collect gathers metrics and returns the collected data.
	Collect(ctx context.Context) (*MetricData, error)

	// Configure applies the given configuration to the collector.
	Configure(cfg config.CollectorConfig) error

	// Interval returns the collection interval for this collector.
	Interval() time.Duration

	// Enabled returns whether the collector is enabled.
	Enabled() bool

	// DefaultConfig returns the default CollectorConfig for this collector.
	DefaultConfig() config.CollectorConfig
}

// BaseCollector provides common functionality for all collectors.
type BaseCollector struct {
	name     string
	interval time.Duration
	enabled  bool
}

// Name returns the collector name.
func (b *BaseCollector) Name() string {
	return b.name
}

// Interval returns the collection interval.
func (b *BaseCollector) Interval() time.Duration {
	return b.interval
}

// Enabled returns whether the collector is enabled.
func (b *BaseCollector) Enabled() bool {
	return b.enabled
}

// SetInterval sets the collection interval.
func (b *BaseCollector) SetInterval(d time.Duration) {
	b.interval = d
}

// SetEnabled sets whether the collector is enabled.
func (b *BaseCollector) SetEnabled(enabled bool) {
	b.enabled = enabled
}

// DefaultConfig returns the default CollectorConfig for this collector.
func (b *BaseCollector) DefaultConfig() config.CollectorConfig {
	return config.CollectorConfig{
		Enabled:  true,
		Interval: b.interval,
	}
}

// NewBaseCollector creates a new BaseCollector with the given name.
func NewBaseCollector(name string) BaseCollector {
	return BaseCollector{
		name:     name,
		interval: 10 * time.Second,
		enabled:  true,
	}
}
