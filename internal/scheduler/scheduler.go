// Package scheduler provides metric collection scheduling functionality.
package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
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

	mu             sync.Mutex
	running        bool
	cancel         context.CancelFunc
	parentCtx      context.Context
	wg             sync.WaitGroup
	log            logger.Config
	reconfigureMu  sync.Mutex // serializes concurrent Reconfigure calls
	lastActivityMs atomic.Int64
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

// LastActivity returns the time of the last successful metric collection.
// Returns zero Time if no successful collection has occurred.
func (s *Scheduler) LastActivity() time.Time {
	ms := s.lastActivityMs.Load()
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
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

	// 30s collect timeout — 가장 느린 collector(LhmHelper daemon 응답, WMI Win32_DiskDrive 쿼리)
	// 기준. 일반 collector는 1초 미만에 끝남.
	//
	// 발동 시 동작: context.Done()으로 신호. collector가 ctx를 존중하지 않을 가능성 있어
	// 두 가지 안전망 적용됨:
	//   - LhmProvider (Phase 1-1): timeout 3회 연속 → Process.Kill + 응답 drain → leak 0
	//   - StorageHealth WMI (Phase 1-2): in-flight flag로 쿼리 중복 방지, stale-cache fallback
	//     → worst-case in-flight goroutine 1개로 bounded
	// 그 외 collector는 ctx 존중 가정. 새 collector 추가 시 동일 보호장치 검토 필요.
	//
	// 이 값을 줄일 때 주의: 위 두 보호장치의 trigger 조건도 함께 검토할 것.
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

	// 10s send timeout — sender 한 번의 전송 호출 상한.
	//
	// sender별 단절 시 동작:
	//   - kafkarest: BufferedHTTPTransport가 in-memory buffer로 enqueue (background flush).
	//     장기 단절 시 Batch.MaxBufferedRecords(기본 10,000) FIFO oldest-drop (Phase 2-1 / H2).
	//     drop 신호: BUFFER_DROP_OLDEST 로그 + agent.buffer_dropped_total 메트릭.
	//   - kafka (sarama): 자체 retry. Batch.MaxRetries 후 drop.
	//   - file: 로컬 파일 쓰기. lumberjack rotation, 디스크 full 외 실패 거의 없음.
	//
	// 이 값을 늘릴 때 주의: collect cycle(interval)보다 길면 다음 cycle 누락 가능.
	sendCtx, sendCancel := context.WithTimeout(ctx, 10*time.Second)
	defer sendCancel()

	if err := s.sender.Send(sendCtx, data); err != nil {
		log.Error().
			Err(err).
			Str("collector", name).
			Msg("Failed to send metrics")
		return
	}

	s.lastActivityMs.Store(time.Now().UnixMilli())

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
