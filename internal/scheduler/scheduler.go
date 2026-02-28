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

// CollectorSource provides access to enabled collectors.
type CollectorSource interface {
	EnabledCollectors() []collector.Collector
}

// Scheduler manages the periodic collection of metrics.
type Scheduler struct {
	registry CollectorSource
	sender   sender.Sender
	agentID  string
	hostname string

	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
	parentCtx     context.Context
	wg            sync.WaitGroup
	log           logger.Config
	reconfigureMu sync.Mutex // serializes concurrent Reconfigure calls
}

// New creates a new scheduler with the given components.
func New(registry CollectorSource, s sender.Sender, agentID, hostname string) *Scheduler {
	return &Scheduler{
		registry: registry,
		sender:   s,
		agentID:  agentID,
		hostname: hostname,
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
	s.parentCtx = ctx

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

// Reconfigure stops all collector goroutines and restarts them with current settings.
// Unlike Stop()+Start(), this keeps running=true throughout to prevent concurrent
// Start() calls from entering during the restart window.
// Concurrent calls are serialized by reconfigureMu.
func (s *Scheduler) Reconfigure() {
	s.reconfigureMu.Lock()
	defer s.reconfigureMu.Unlock()

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	s.cancel()
	parentCtx := s.parentCtx
	s.mu.Unlock()

	s.wg.Wait()

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return // Stop() was called while we were waiting
	}

	log := logger.WithComponent("scheduler")
	log.Info().Msg("Reconfiguring scheduler")

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancel = cancel

	collectors := s.registry.EnabledCollectors()
	log.Info().Int("enabled_count", len(collectors)).Msg("Enabled collectors count after reconfigure")
	for _, c := range collectors {
		log.Info().Str("collector", c.Name()).Msg("Collector is enabled")
		s.wg.Add(1)
		go s.runCollector(ctx, c)
	}
}

