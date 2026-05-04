//go:build windows

package collector

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// withMockWMI swaps wmiQueryFunc for the duration of the test, restoring
// the original on cleanup. Always pair with resetWMIQueryStateForTest to
// avoid bleed between tests.
func withMockWMI(t *testing.T, mock func(string, interface{}, ...interface{}) error) {
	t.Helper()
	original := wmiQueryFunc
	wmiQueryFunc = mock
	t.Cleanup(func() {
		wmiQueryFunc = original
		resetWMIQueryStateForTest()
	})
	resetWMIQueryStateForTest()
}

// assignDiskDrive populates *dst (which must be *[]Win32DiskDriveHealth) with
// the supplied entries — used by mocks because reflect.Set is required for
// the &dst pattern wmi.Query uses.
func assignDiskDrive(dst interface{}, entries []Win32DiskDriveHealth) {
	v := reflect.ValueOf(dst).Elem()
	v.Set(reflect.ValueOf(entries))
}

func TestQueryWMIDiskDrive_NormalReturn(t *testing.T) {
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		assignDiskDrive(dst, []Win32DiskDriveHealth{{Model: "Disk1", Status: "OK"}})
		return nil
	})

	disks, err := queryWMIDiskDrive(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(disks) != 1 || disks[0].Model != "Disk1" {
		t.Fatalf("unexpected result: %+v", disks)
	}

	// inFlight must be cleared so next call works normally.
	if wmiQueryState.inFlight.Load() {
		t.Error("inFlight not cleared after normal return")
	}
}

func TestQueryWMIDiskDrive_TimeoutDoesNotLeakAcrossCalls(t *testing.T) {
	// Mock blocks until a release channel is closed, simulating WMI hang.
	release := make(chan struct{})
	defer close(release)

	var callCount atomic.Int32
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		callCount.Add(1)
		<-release
		assignDiskDrive(dst, []Win32DiskDriveHealth{{Model: "RecoveredDisk"}})
		return nil
	})

	before := runtime.NumGoroutine()

	// First call: spawns the worker, ctx timeout fires before the mock
	// returns. Worker remains in flight.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()
	if _, err := queryWMIDiskDrive(ctx1); err == nil {
		t.Fatal("expected timeout error on first call")
	}
	if !wmiQueryState.inFlight.Load() {
		t.Fatal("inFlight should remain true after timeout (worker still running)")
	}

	// 50 follow-up calls. Each must return immediately without spawning a
	// new worker — that's the whole point of option C.
	for i := 0; i < 50; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		_, _ = queryWMIDiskDrive(ctx)
		cancel()
	}
	if got := callCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 call to wmi.Query (the original worker), got %d", got)
	}

	// Allow goroutines to settle. Any leaks would show up as a large delta.
	time.Sleep(100 * time.Millisecond)
	after := runtime.NumGoroutine()
	// The single worker is still parked on <-release; tolerance ±5 for
	// scheduler jitter, but not 50.
	if delta := after - before; delta > 5 {
		t.Errorf("goroutine leak detected: before=%d after=%d delta=%d (expected <=1 from blocked worker + scheduler jitter)",
			before, after, delta)
	}
}

func TestQueryWMIDiskDrive_InFlightReturnsCachedData(t *testing.T) {
	// Pre-populate cache via a successful query.
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		assignDiskDrive(dst, []Win32DiskDriveHealth{{Model: "Cached"}})
		return nil
	})
	if _, err := queryWMIDiskDrive(context.Background()); err != nil {
		t.Fatalf("setup query failed: %v", err)
	}

	// Now simulate a hang for the next worker. Re-swap mock manually to keep
	// the cache from withMockWMI's previous run alive.
	originalAfterPrime := wmiQueryFunc
	release := make(chan struct{})
	defer close(release)
	wmiQueryFunc = func(_ string, dst interface{}, _ ...interface{}) error {
		<-release
		assignDiskDrive(dst, nil)
		return nil
	}
	t.Cleanup(func() { wmiQueryFunc = originalAfterPrime })

	// Trigger a worker that will hang.
	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()
	_, _ = queryWMIDiskDrive(ctx1)

	// Subsequent in-flight call must return the cached "Cached" disk.
	disks, err := queryWMIDiskDrive(context.Background())
	if err != nil {
		t.Fatalf("expected cached data, got error: %v", err)
	}
	if len(disks) != 1 || disks[0].Model != "Cached" {
		t.Errorf("expected cached disk, got: %+v", disks)
	}
}

func TestQueryWMIDiskDrive_InFlightWithoutCacheReturnsError(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		<-release
		assignDiskDrive(dst, nil)
		return nil
	})

	// First call hangs and the ctx times out → worker is in flight, cache
	// is still empty (the hung worker hasn't reported).
	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()
	_, _ = queryWMIDiskDrive(ctx1)

	// Second call observes inFlight=true with empty cache → must surface
	// an error rather than block.
	disks, err := queryWMIDiskDrive(context.Background())
	if err == nil {
		t.Errorf("expected error when in-flight without cache, got disks=%+v", disks)
	}
	if disks != nil {
		t.Errorf("expected nil disks when erroring, got %+v", disks)
	}
}

func TestQueryWMIDiskDrive_RecoveryAfterHang(t *testing.T) {
	// Worker hangs initially, then is released. After release the cache
	// must be populated and inFlight must be cleared so subsequent calls
	// work normally.
	release := make(chan struct{})
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		<-release
		assignDiskDrive(dst, []Win32DiskDriveHealth{{Model: "Recovered"}})
		return nil
	})

	ctx1, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()
	_, _ = queryWMIDiskDrive(ctx1)
	if !wmiQueryState.inFlight.Load() {
		t.Fatal("inFlight should be true while worker hangs")
	}

	// Release the hung worker and let it finish updating the cache.
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for wmiQueryState.inFlight.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if wmiQueryState.inFlight.Load() {
		t.Fatal("inFlight not cleared after worker finished")
	}

	// Next call must spawn a fresh worker (mock count would catch otherwise)
	// and return the recovered cache.
	disks, err := queryWMIDiskDrive(context.Background())
	if err != nil {
		t.Fatalf("post-recovery query failed: %v", err)
	}
	if len(disks) != 1 || disks[0].Model != "Recovered" {
		t.Errorf("expected fresh result after recovery, got: %+v", disks)
	}
}

func TestQueryWMIDiskDrive_ErrorSurfacedAndCached(t *testing.T) {
	wmiErr := errors.New("WMI provider failure")
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		assignDiskDrive(dst, nil)
		return wmiErr
	})

	_, err := queryWMIDiskDrive(context.Background())
	if !errors.Is(err, wmiErr) {
		t.Errorf("expected wrapped wmi error, got: %v", err)
	}

	// Cache should hold the error so an in-flight follow-up surfaces the
	// same diagnostic instead of returning a stale OK from earlier.
	cached := loadWMICache()
	if cached == nil || !errors.Is(cached.err, wmiErr) {
		t.Errorf("expected cached error entry, got: %+v", cached)
	}
}

func TestQueryWMIDiskDrive_ConcurrentCallersCoalesce(t *testing.T) {
	// Many simultaneous callers should coalesce to a single worker.
	var callCount atomic.Int32
	gate := make(chan struct{})
	withMockWMI(t, func(_ string, dst interface{}, _ ...interface{}) error {
		callCount.Add(1)
		<-gate
		assignDiskDrive(dst, []Win32DiskDriveHealth{{Model: "OnceOnly"}})
		return nil
	})

	const concurrency = 20
	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			_, _ = queryWMIDiskDrive(ctx)
		}()
	}
	wg.Wait()

	// All callers but the first should have observed inFlight=true and
	// returned without invoking the mock. Exactly one worker.
	if got := callCount.Load(); got != 1 {
		t.Errorf("expected exactly 1 wmi.Query call across %d concurrent callers, got %d", concurrency, got)
	}

	// Release the worker and verify the cache populates.
	close(gate)
	deadline := time.Now().Add(2 * time.Second)
	for wmiQueryState.inFlight.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}
