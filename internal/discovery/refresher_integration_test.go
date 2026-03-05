package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// fakeTransport tracks Deliver calls for integration testing.
type fakeTransport struct {
	mu           sync.Mutex
	name         string
	deliverCalls int
	closed       int32 // atomic
}

func (f *fakeTransport) Close() error {
	atomic.AddInt32(&f.closed, 1)
	return nil
}

func (f *fakeTransport) getDeliverCalls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.deliverCalls
}

func TestRefresher_Integration(t *testing.T) {
	// 1. Mock ServiceDiscovery server that returns configurable KafkaRest address
	var mu sync.Mutex
	currentAddr := "http://server-A:8082"

	sdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		addr := currentAddr
		mu.Unlock()
		services := map[string]string{
			"KafkaRest": addr,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(services)
	}))
	defer sdServer.Close()

	// 2. Track transport swaps
	var swapCount int32
	var currentTransport atomic.Value
	initialTransport := &fakeTransport{name: "server-A"}
	currentTransport.Store(initialTransport)

	var closedTransports []*fakeTransport
	var closedMu sync.Mutex

	// 3. Create Refresher with short interval
	r := NewRefresher(RefresherConfig{
		Interval: 100 * time.Millisecond,
	})

	r.fetchAddr = func(ctx context.Context) (string, error) {
		// Call the mock SD server
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, sdServer.URL, nil)
		if err != nil {
			return "", err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()

		var services map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&services); err != nil {
			return "", err
		}
		addr, ok := services["KafkaRest"]
		if !ok {
			return "", fmt.Errorf("no KafkaRest in response")
		}
		return addr, nil
	}

	r.transportFactory = func(addr string) (Closeable, error) {
		return &fakeTransport{name: addr}, nil
	}

	r.swapTransport = func(newT Closeable) (Closeable, error) {
		atomic.AddInt32(&swapCount, 1)
		old := currentTransport.Load().(*fakeTransport)
		currentTransport.Store(newT.(*fakeTransport))
		return old, nil
	}

	r.closeOld = func(old Closeable) {
		old.Close()
		closedMu.Lock()
		closedTransports = append(closedTransports, old.(*fakeTransport))
		closedMu.Unlock()
	}

	// 4. Start refresher
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://server-A:8082")

	// 5. Change SD server response
	time.Sleep(50 * time.Millisecond) // let first tick schedule
	mu.Lock()
	currentAddr = "http://server-B:8082"
	mu.Unlock()

	// Wait for refresher to detect the change
	deadline := time.After(2 * time.Second)
	for {
		if atomic.LoadInt32(&swapCount) >= 1 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for first address change swap")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	// 6. Verify new transport is active
	active := currentTransport.Load().(*fakeTransport)
	if active.name != "http://server-B:8082" {
		t.Errorf("expected active transport to be server-B, got %s", active.name)
	}

	// 7. Change again: B → C
	mu.Lock()
	currentAddr = "http://server-C:8082"
	mu.Unlock()

	deadline = time.After(2 * time.Second)
	for {
		if atomic.LoadInt32(&swapCount) >= 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timeout waiting for second address change swap")
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}

	active = currentTransport.Load().(*fakeTransport)
	if active.name != "http://server-C:8082" {
		t.Errorf("expected active transport to be server-C, got %s", active.name)
	}

	// 8. Stop refresher
	r.Stop()

	// 9. Verify old transports were closed
	closedMu.Lock()
	defer closedMu.Unlock()
	if len(closedTransports) < 2 {
		t.Errorf("expected at least 2 old transports closed, got %d", len(closedTransports))
	}

	// Verify initial transport (server-A) was closed
	foundA := false
	for _, ct := range closedTransports {
		if ct.name == "server-A" {
			foundA = true
			break
		}
	}
	if !foundA {
		t.Error("initial transport (server-A) was not closed")
	}
}

func TestRefresher_Integration_SDFailureKeepsOldTransport(t *testing.T) {
	callCount := int32(0)

	sdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&callCount, 1)
		if n >= 3 {
			// Start failing after 2 successful calls
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		services := map[string]string{"KafkaRest": "http://server-A:8082"}
		json.NewEncoder(w).Encode(services)
	}))
	defer sdServer.Close()

	var swapCount int32
	r := NewRefresher(RefresherConfig{
		Interval: 80 * time.Millisecond,
	})

	r.fetchAddr = func(ctx context.Context) (string, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, sdServer.URL, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return "", fmt.Errorf("SD returned %d", resp.StatusCode)
		}
		var services map[string]string
		json.NewDecoder(resp.Body).Decode(&services)
		return services["KafkaRest"], nil
	}

	r.swapTransport = func(newT Closeable) (Closeable, error) {
		atomic.AddInt32(&swapCount, 1)
		return &fakeTransport{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r.Start(ctx, "http://server-A:8082") // same as SD returns

	time.Sleep(600 * time.Millisecond)
	r.Stop()

	// No swaps should have occurred (address unchanged, then SD fails)
	if atomic.LoadInt32(&swapCount) != 0 {
		t.Errorf("expected 0 swaps, got %d", atomic.LoadInt32(&swapCount))
	}
}
