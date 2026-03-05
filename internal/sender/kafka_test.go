package sender

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resourceagent/internal/config"
)

// spyTransport records Deliver calls and can be identified by name.
type spyTransport struct {
	mu           sync.Mutex
	name         string
	deliverCalls int
	closed       int32 // atomic
}

func (s *spyTransport) Deliver(ctx context.Context, topic string, records []KafkaRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliverCalls++
	return nil
}

func (s *spyTransport) Close() error {
	atomic.AddInt32(&s.closed, 1)
	return nil
}

func (s *spyTransport) getDeliverCalls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliverCalls
}

func newTestKafkaSender(transport KafkaTransport) *KafkaSender {
	eqpInfo := &config.EqpInfoConfig{
		Process: "PROC", EqpModel: "MODEL", EqpID: "EQP",
		Line: "LINE", LineDesc: "Desc", Index: "1",
	}
	return NewKafkaSender(transport, "topic", eqpInfo, func() int64 { return 0 }, GrokRawFormatter{})
}

func TestSwapTransport_Success(t *testing.T) {
	oldT := &spyTransport{name: "old"}
	newT := &spyTransport{name: "new"}
	s := newTestKafkaSender(oldT)

	returned, err := s.SwapTransport(newT)
	if err != nil {
		t.Fatalf("SwapTransport failed: %v", err)
	}
	if returned != oldT {
		t.Error("expected old transport to be returned")
	}
}

func TestSwapTransport_ReturnsOld(t *testing.T) {
	t1 := &spyTransport{name: "t1"}
	t2 := &spyTransport{name: "t2"}
	t3 := &spyTransport{name: "t3"}
	s := newTestKafkaSender(t1)

	got1, _ := s.SwapTransport(t2)
	got2, _ := s.SwapTransport(t3)

	if got1 != t1 {
		t.Error("first swap should return t1")
	}
	if got2 != t2 {
		t.Error("second swap should return t2")
	}
}

func TestSwapTransport_ClosedSender(t *testing.T) {
	oldT := &spyTransport{name: "old"}
	s := newTestKafkaSender(oldT)
	s.Close()

	_, err := s.SwapTransport(&spyTransport{name: "new"})
	if err == nil {
		t.Fatal("expected error on closed sender")
	}
}

func TestSwapTransport_SendUsesNewTransport(t *testing.T) {
	oldT := &spyTransport{name: "old"}
	newT := &spyTransport{name: "new"}
	s := newTestKafkaSender(oldT)

	// Send before swap → old transport
	s.Send(context.Background(), newTestMetricData())
	if oldT.getDeliverCalls() != 1 {
		t.Errorf("expected 1 call to old transport, got %d", oldT.getDeliverCalls())
	}

	// Swap
	s.SwapTransport(newT)

	// Send after swap → new transport
	s.Send(context.Background(), newTestMetricData())
	if newT.getDeliverCalls() != 1 {
		t.Errorf("expected 1 call to new transport, got %d", newT.getDeliverCalls())
	}
	// Old transport should still have only 1 call
	if oldT.getDeliverCalls() != 1 {
		t.Errorf("old transport should still have 1 call, got %d", oldT.getDeliverCalls())
	}
}

func TestSwapTransport_ConcurrentSendAndSwap(t *testing.T) {
	oldT := &spyTransport{name: "old"}
	s := newTestKafkaSender(oldT)

	ctx := context.Background()
	var wg sync.WaitGroup
	var sendErrors int32

	// 10 goroutines sending continuously
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if err := s.Send(ctx, newTestMetricData()); err != nil {
					atomic.AddInt32(&sendErrors, 1)
				}
			}
		}()
	}

	// Swap transport multiple times during sends
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Millisecond)
		newT := &spyTransport{name: "swap"}
		s.SwapTransport(newT)
	}

	wg.Wait()

	if sendErrors != 0 {
		t.Errorf("expected 0 send errors, got %d", sendErrors)
	}
}
