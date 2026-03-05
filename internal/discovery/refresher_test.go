package discovery

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// mockTransport implements a minimal transport for testing.
type mockTransport struct {
	name   string
	closed int32 // atomic
}

func (m *mockTransport) Close() error {
	atomic.AddInt32(&m.closed, 1)
	return nil
}

func (m *mockTransport) isClosed() bool {
	return atomic.LoadInt32(&m.closed) > 0
}

func TestRefresher_AddressChanged(t *testing.T) {
	var swapCount int32
	var lastOldClosed int32

	r := NewRefresher(RefresherConfig{
		Interval: 50 * time.Millisecond,
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		return "http://new-server:8082", nil
	}
	r.transportFactory = func(addr string) (Closeable, error) {
		return &mockTransport{name: addr}, nil
	}
	r.swapTransport = func(newT Closeable) (Closeable, error) {
		atomic.AddInt32(&swapCount, 1)
		old := &mockTransport{name: "old"}
		return old, nil
	}
	r.closeOld = func(old Closeable) {
		old.Close()
		atomic.AddInt32(&lastOldClosed, 1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://old-server:8082")

	time.Sleep(200 * time.Millisecond)
	r.Stop()

	if atomic.LoadInt32(&swapCount) == 0 {
		t.Error("expected at least one swap")
	}
	if atomic.LoadInt32(&lastOldClosed) == 0 {
		t.Error("expected old transport to be closed")
	}
}

func TestRefresher_AddressUnchanged(t *testing.T) {
	var swapCount int32

	r := NewRefresher(RefresherConfig{
		Interval: 50 * time.Millisecond,
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		return "http://same:8082", nil
	}
	r.swapTransport = func(newT Closeable) (Closeable, error) {
		atomic.AddInt32(&swapCount, 1)
		return &mockTransport{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://same:8082")

	time.Sleep(200 * time.Millisecond)
	r.Stop()

	if atomic.LoadInt32(&swapCount) != 0 {
		t.Errorf("expected no swaps, got %d", atomic.LoadInt32(&swapCount))
	}
}

func TestRefresher_ServiceDiscoveryFails(t *testing.T) {
	var swapCount int32

	r := NewRefresher(RefresherConfig{
		Interval: 50 * time.Millisecond,
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		return "", fmt.Errorf("connection refused")
	}
	r.swapTransport = func(newT Closeable) (Closeable, error) {
		atomic.AddInt32(&swapCount, 1)
		return &mockTransport{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://original:8082")

	time.Sleep(200 * time.Millisecond)
	r.Stop()

	if atomic.LoadInt32(&swapCount) != 0 {
		t.Errorf("expected no swaps on SD failure, got %d", atomic.LoadInt32(&swapCount))
	}
}

func TestRefresher_TransportFactoryFails(t *testing.T) {
	var swapCount int32

	r := NewRefresher(RefresherConfig{
		Interval: 50 * time.Millisecond,
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		return "http://new:8082", nil
	}
	r.transportFactory = func(addr string) (Closeable, error) {
		return nil, fmt.Errorf("factory error")
	}
	r.swapTransport = func(newT Closeable) (Closeable, error) {
		atomic.AddInt32(&swapCount, 1)
		return &mockTransport{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://old:8082")

	time.Sleep(200 * time.Millisecond)
	r.Stop()

	if atomic.LoadInt32(&swapCount) != 0 {
		t.Errorf("expected no swaps on factory failure, got %d", atomic.LoadInt32(&swapCount))
	}
}

func TestRefresher_Stop(t *testing.T) {
	r := NewRefresher(RefresherConfig{
		Interval: 1 * time.Hour, // long interval, never fires
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		return "http://addr:8082", nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://addr:8082")

	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s")
	}
}

func TestRefresher_ConsecutiveAddressChanges(t *testing.T) {
	var mu sync.Mutex
	callNum := 0
	addresses := []string{"http://A:8082", "http://B:8082", "http://C:8082"}

	var closedNames []string
	var closedMu sync.Mutex

	r := NewRefresher(RefresherConfig{
		Interval: 50 * time.Millisecond,
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		mu.Lock()
		defer mu.Unlock()
		idx := callNum
		if idx >= len(addresses) {
			idx = len(addresses) - 1
		}
		callNum++
		return addresses[idx], nil
	}
	r.transportFactory = func(addr string) (Closeable, error) {
		return &mockTransport{name: addr}, nil
	}

	currentTransport := &mockTransport{name: "initial"}
	r.swapTransport = func(newT Closeable) (Closeable, error) {
		mu.Lock()
		defer mu.Unlock()
		old := currentTransport
		currentTransport = newT.(*mockTransport)
		return old, nil
	}
	r.closeOld = func(old Closeable) {
		old.Close()
		closedMu.Lock()
		closedNames = append(closedNames, old.(*mockTransport).name)
		closedMu.Unlock()
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://A:8082")

	time.Sleep(400 * time.Millisecond)
	r.Stop()

	closedMu.Lock()
	defer closedMu.Unlock()
	if len(closedNames) < 1 {
		t.Error("expected at least one old transport to be closed")
	}
}

func TestRefresher_IntervalZero_Disabled(t *testing.T) {
	r := NewRefresher(RefresherConfig{
		Interval: 0,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://addr:8082")

	// Stop should return immediately since no goroutine was started
	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()

	select {
	case <-done:
		// ok
	case <-time.After(1 * time.Second):
		t.Fatal("Stop did not return within 1s for disabled refresher")
	}
}

func TestRefresher_NegativeInterval_Disabled(t *testing.T) {
	r := NewRefresher(RefresherConfig{
		Interval: -1 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://addr:8082")

	done := make(chan struct{})
	go func() {
		r.Stop()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("Stop did not return within 1s for disabled refresher")
	}
}

func TestRefresher_SwapTransportFails(t *testing.T) {
	var factoryCount int32
	newTransports := make([]*mockTransport, 0)
	var mu sync.Mutex

	r := NewRefresher(RefresherConfig{
		Interval: 50 * time.Millisecond,
	})
	r.fetchAddr = func(ctx context.Context) (string, error) {
		return "http://new:8082", nil
	}
	r.transportFactory = func(addr string) (Closeable, error) {
		atomic.AddInt32(&factoryCount, 1)
		mt := &mockTransport{name: addr}
		mu.Lock()
		newTransports = append(newTransports, mt)
		mu.Unlock()
		return mt, nil
	}
	r.swapTransport = func(newT Closeable) (Closeable, error) {
		return nil, fmt.Errorf("sender is closed")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://old:8082")

	time.Sleep(200 * time.Millisecond)
	r.Stop()

	// New transport should be closed since swap failed
	mu.Lock()
	defer mu.Unlock()
	for _, mt := range newTransports {
		if !mt.isClosed() {
			t.Errorf("new transport %q should be closed after swap failure", mt.name)
		}
	}
}
