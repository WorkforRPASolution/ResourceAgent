package collector

import (
	"fmt"
	"sync"

	"resourceagent/internal/config"
)

// Registry manages collector registration and lifecycle.
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]Collector
}

// NewRegistry creates a new collector registry.
func NewRegistry() *Registry {
	return &Registry{
		collectors: make(map[string]Collector),
	}
}

// Register adds a collector to the registry.
func (r *Registry) Register(c Collector) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := c.Name()
	if _, exists := r.collectors[name]; exists {
		return fmt.Errorf("collector %s already registered", name)
	}

	r.collectors[name] = c
	return nil
}

// Get retrieves a collector by name.
func (r *Registry) Get(name string) (Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	c, ok := r.collectors[name]
	return c, ok
}

// All returns all registered collectors.
func (r *Registry) All() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Collector, 0, len(r.collectors))
	for _, c := range r.collectors {
		result = append(result, c)
	}
	return result
}

// Configure applies configuration to all registered collectors.
func (r *Registry) Configure(configs map[string]config.CollectorConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for name, cfg := range configs {
		if c, ok := r.collectors[name]; ok {
			if err := c.Configure(cfg); err != nil {
				return fmt.Errorf("failed to configure collector %s: %w", name, err)
			}
		}
	}
	return nil
}

// EnabledCollectors returns only the enabled collectors.
func (r *Registry) EnabledCollectors() []Collector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Collector
	for _, c := range r.collectors {
		// Check if collector has Enabled method via BaseCollector
		if bc, ok := c.(*CPUCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else if bc, ok := c.(*MemoryCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else if bc, ok := c.(*DiskCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else if bc, ok := c.(*NetworkCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else if bc, ok := c.(*TemperatureCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else if bc, ok := c.(*CPUProcessCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else if bc, ok := c.(*MemoryProcessCollector); ok && bc.Enabled() {
			result = append(result, c)
		} else {
			// Default: assume enabled if not determinable
			result = append(result, c)
		}
	}
	return result
}

// DefaultRegistry creates a registry with all default collectors pre-registered.
func DefaultRegistry() *Registry {
	r := NewRegistry()

	// Register all default collectors
	_ = r.Register(NewCPUCollector())
	_ = r.Register(NewMemoryCollector())
	_ = r.Register(NewDiskCollector())
	_ = r.Register(NewNetworkCollector())
	_ = r.Register(NewTemperatureCollector())
	_ = r.Register(NewCPUProcessCollector())
	_ = r.Register(NewMemoryProcessCollector())

	return r
}
