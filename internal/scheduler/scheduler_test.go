package scheduler

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// mockCollector implements collector.Collector for testing.
type mockCollector struct {
	name     string
	mu       sync.Mutex
	interval time.Duration
	enabled  bool
	calls    int32
}

func newMockCollector(name string, interval time.Duration, enabled bool) *mockCollector {
	return &mockCollector{
		name:     name,
		interval: interval,
		enabled:  enabled,
	}
}

func (m *mockCollector) Name() string { return m.name }

func (m *mockCollector) Collect(_ context.Context) (*collector.MetricData, error) {
	atomic.AddInt32(&m.calls, 1)
	return &collector.MetricData{
		Type:      m.name,
		Timestamp: time.Now(),
		Data:      map[string]float64{"value": 1.0},
	}, nil
}

func (m *mockCollector) Configure(cfg config.CollectorConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.enabled = cfg.Enabled
	m.interval = cfg.Interval
	return nil
}

func (m *mockCollector) Interval() time.Duration {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.interval
}

func (m *mockCollector) Enabled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.enabled
}

func (m *mockCollector) DefaultConfig() config.CollectorConfig {
	return config.CollectorConfig{
		Enabled:  true,
		Interval: m.interval,
	}
}

func (m *mockCollector) collectCount() int32 {
	return atomic.LoadInt32(&m.calls)
}

func (m *mockCollector) resetCount() {
	atomic.StoreInt32(&m.calls, 0)
}

// mockCollectorSource implements CollectorSource, returning only enabled mock collectors.
type mockCollectorSource struct {
	mu         sync.Mutex
	collectors []*mockCollector
}

func (s *mockCollectorSource) EnabledCollectors() []collector.Collector {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []collector.Collector
	for _, mc := range s.collectors {
		if mc.Enabled() {
			result = append(result, mc)
		}
	}
	return result
}

// mockSender implements sender.Sender for testing.
type mockSender struct {
	mu    sync.Mutex
	sends int
}

func (s *mockSender) Send(_ context.Context, _ *collector.MetricData) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sends++
	return nil
}

func (s *mockSender) SendBatch(_ context.Context, _ []*collector.MetricData) error {
	return nil
}

func (s *mockSender) Close() error { return nil }

func init() {
	_ = logger.Init(logger.Config{Level: "disabled"})
}

func TestReconfigure_IntervalChange(t *testing.T) {
	mc := newMockCollector("test_cpu", 50*time.Millisecond, true)
	snd := &mockSender{}
	source := &mockCollectorSource{collectors: []*mockCollector{mc}}

	sched := New(source, snd, "agent1", "host1", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = sched.Start(ctx)

	// Let collector run a few cycles at 50ms interval
	time.Sleep(180 * time.Millisecond)
	countBefore := mc.collectCount()

	// Change interval to 200ms and reconfigure
	mc.mu.Lock()
	mc.interval = 200 * time.Millisecond
	mc.mu.Unlock()

	mc.resetCount()
	sched.Reconfigure()

	// Wait 500ms - at 200ms interval, expect ~2-3 collections (including initial)
	time.Sleep(500 * time.Millisecond)
	countAfter := mc.collectCount()

	if countBefore < 2 {
		t.Errorf("before reconfigure: expected at least 2 collections at 50ms, got %d", countBefore)
	}

	// At 200ms interval over 500ms: initial + ~2 ticks = ~3
	if countAfter < 2 || countAfter > 5 {
		t.Errorf("after reconfigure: expected 2-5 collections at 200ms interval over 500ms, got %d", countAfter)
	}

	sched.Stop()
}

func TestReconfigure_DisableCollector(t *testing.T) {
	mc := newMockCollector("test_mem", 50*time.Millisecond, true)
	snd := &mockSender{}
	source := &mockCollectorSource{collectors: []*mockCollector{mc}}

	sched := New(source, snd, "agent1", "host1", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = sched.Start(ctx)
	time.Sleep(120 * time.Millisecond)

	if mc.collectCount() < 1 {
		t.Fatal("expected at least 1 collection before disable")
	}

	// Disable the collector and reconfigure
	mc.mu.Lock()
	mc.enabled = false
	mc.mu.Unlock()

	mc.resetCount()
	sched.Reconfigure()

	// Wait and verify no new collections
	time.Sleep(200 * time.Millisecond)

	if mc.collectCount() != 0 {
		t.Errorf("after disable: expected 0 collections, got %d", mc.collectCount())
	}

	sched.Stop()
}

func TestReconfigure_EnableCollector(t *testing.T) {
	mc := newMockCollector("test_disk", 50*time.Millisecond, false) // starts disabled
	snd := &mockSender{}
	source := &mockCollectorSource{collectors: []*mockCollector{mc}}

	sched := New(source, snd, "agent1", "host1", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = sched.Start(ctx)
	time.Sleep(120 * time.Millisecond)

	if mc.collectCount() != 0 {
		t.Errorf("expected 0 collections while disabled, got %d", mc.collectCount())
	}

	// Enable and reconfigure
	mc.mu.Lock()
	mc.enabled = true
	mc.mu.Unlock()

	sched.Reconfigure()

	time.Sleep(150 * time.Millisecond)

	if mc.collectCount() < 1 {
		t.Error("expected at least 1 collection after enable")
	}

	sched.Stop()
}

func TestReconfigure_WhileNotRunning(t *testing.T) {
	mc := newMockCollector("test_net", 50*time.Millisecond, true)
	snd := &mockSender{}
	source := &mockCollectorSource{collectors: []*mockCollector{mc}}

	sched := New(source, snd, "agent1", "host1", nil)

	// Reconfigure without Start - should not panic
	sched.Reconfigure()

	if sched.IsRunning() {
		t.Error("scheduler should not be running after Reconfigure on non-started scheduler")
	}
}

func TestReconfigure_ConcurrentSafety(t *testing.T) {
	mc := newMockCollector("test_concurrent", 50*time.Millisecond, true)
	snd := &mockSender{}
	source := &mockCollectorSource{collectors: []*mockCollector{mc}}

	sched := New(source, snd, "agent1", "host1", nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_ = sched.Start(ctx)
	time.Sleep(80 * time.Millisecond)

	// Launch 2 concurrent Reconfigure calls
	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			sched.Reconfigure()
		}()
	}
	wg.Wait()

	// Scheduler should still be running and collecting
	if !sched.IsRunning() {
		t.Fatal("scheduler should still be running after concurrent Reconfigure")
	}

	mc.resetCount()
	time.Sleep(150 * time.Millisecond)

	if mc.collectCount() < 1 {
		t.Error("expected at least 1 collection after concurrent Reconfigure")
	}

	sched.Stop()
}

func TestReconfigure_PreservesParentContext(t *testing.T) {
	mc := newMockCollector("test_temp", 50*time.Millisecond, true)
	snd := &mockSender{}
	source := &mockCollectorSource{collectors: []*mockCollector{mc}}

	sched := New(source, snd, "agent1", "host1", nil)

	ctx, cancel := context.WithCancel(context.Background())

	_ = sched.Start(ctx)
	sched.Reconfigure()

	time.Sleep(80 * time.Millisecond)
	if mc.collectCount() < 1 {
		t.Fatal("expected collections after Reconfigure")
	}

	// Cancel parent context - goroutines should stop
	cancel()

	// Stop should complete quickly (not deadlock) because parent context propagated
	done := make(chan struct{})
	go func() {
		sched.Stop()
		close(done)
	}()

	select {
	case <-done:
		// Good - Stop completed, goroutines exited via parent context
	case <-time.After(2 * time.Second):
		t.Fatal("Stop() timed out - parent context not propagated after Reconfigure")
	}
}
