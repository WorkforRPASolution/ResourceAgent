// Package scheduler provides metric collection scheduling functionality.
package scheduler

import (
	"context"
	"sync"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/logger"
	"resourceagent/internal/sender"
)

// Scheduler manages the periodic collection of metrics.
type Scheduler struct {
	registry *collector.Registry
	sender   sender.Sender
	agentID  string
	hostname string
	tags     map[string]string

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	log     logger.Config
}

// New creates a new scheduler with the given components.
func New(registry *collector.Registry, s sender.Sender, agentID, hostname string, tags map[string]string) *Scheduler {
	return &Scheduler{
		registry: registry,
		sender:   s,
		agentID:  agentID,
		hostname: hostname,
		tags:     tags,
	}
}

// Start begins the metric collection schedule.
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = true

	ctx, s.cancel = context.WithCancel(ctx)
	s.mu.Unlock()

	log := logger.WithComponent("scheduler")
	log.Info().Msg("Starting scheduler")

	// Start a goroutine for each enabled collector
	collectors := s.registry.EnabledCollectors()
	log.Info().Int("enabled_count", len(collectors)).Msg("Enabled collectors count")
	for _, c := range collectors {
		log.Info().Str("collector", c.Name()).Msg("Collector is enabled")
		s.wg.Add(1)
		go s.runCollector(ctx, c)
	}

	return nil
}

// Stop stops the scheduler and waits for all collectors to finish.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.cancel()
	s.mu.Unlock()

	log := logger.WithComponent("scheduler")
	log.Info().Msg("Stopping scheduler, waiting for collectors to finish")

	s.wg.Wait()
	log.Info().Msg("Scheduler stopped")
}

// IsRunning returns whether the scheduler is currently running.
func (s *Scheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) runCollector(ctx context.Context, c collector.Collector) {
	defer s.wg.Done()

	log := logger.WithComponent("scheduler")
	name := c.Name()
	interval := c.Interval()

	log.Info().
		Str("collector", name).
		Dur("interval", interval).
		Msg("Starting collector")

	// Initial collection
	s.collect(ctx, c)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info().Str("collector", name).Msg("Collector stopped")
			return
		case <-ticker.C:
			s.collect(ctx, c)
		}
	}
}

func (s *Scheduler) collect(ctx context.Context, c collector.Collector) {
	log := logger.WithComponent("scheduler")
	name := c.Name()

	// Create a timeout context for collection
	collectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	startTime := time.Now()
	data, err := c.Collect(collectCtx)
	duration := time.Since(startTime)

	if err != nil {
		log.Error().
			Err(err).
			Str("collector", name).
			Dur("duration", duration).
			Msg("Collection failed")
		return
	}

	if data == nil {
		log.Warn().
			Str("collector", name).
			Msg("Collector returned nil data")
		return
	}

	// Enrich metric data with agent information
	data.AgentID = s.agentID
	data.Hostname = s.hostname
	data.Tags = s.tags

	// Send to Kafka
	sendCtx, sendCancel := context.WithTimeout(ctx, 10*time.Second)
	defer sendCancel()

	if err := s.sender.Send(sendCtx, data); err != nil {
		log.Error().
			Err(err).
			Str("collector", name).
			Msg("Failed to send metrics")
		return
	}

	log.Debug().
		Str("collector", name).
		Dur("duration", duration).
		Msg("Collection completed")
}

// UpdateCollectorInterval updates the interval for a specific collector.
func (s *Scheduler) UpdateCollectorInterval(name string, interval time.Duration) error {
	c, ok := s.registry.Get(name)
	if !ok {
		return nil
	}

	// This will take effect on the next collection cycle
	if base, ok := c.(*collector.CPUCollector); ok {
		base.SetInterval(interval)
	} else if base, ok := c.(*collector.MemoryCollector); ok {
		base.SetInterval(interval)
	} else if base, ok := c.(*collector.DiskCollector); ok {
		base.SetInterval(interval)
	} else if base, ok := c.(*collector.NetworkCollector); ok {
		base.SetInterval(interval)
	} else if base, ok := c.(*collector.TemperatureCollector); ok {
		base.SetInterval(interval)
	} else if base, ok := c.(*collector.CPUProcessCollector); ok {
		base.SetInterval(interval)
	} else if base, ok := c.(*collector.MemoryProcessCollector); ok {
		base.SetInterval(interval)
	}

	return nil
}
