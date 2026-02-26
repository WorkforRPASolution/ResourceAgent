package timediff

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"resourceagent/internal/config"
)

func TestSyncer_Start(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	syncer := NewSyncer(mr.Addr(), cfg, nil, "EQP001", 3600)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer syncer.Stop()

	// GetDiff should return a value (time difference between local and miniredis)
	diff := syncer.GetDiff()
	// miniredis runs locally so diff should be small (within 5 seconds)
	if diff < -5000 || diff > 5000 {
		t.Errorf("expected diff within ±5000ms, got %d", diff)
	}

	// Redis should have EQP_DIFF:EQP001 key
	mr.Select(10)
	val, err := mr.Get(fmt.Sprintf("EQP_DIFF:%s", "EQP001"))
	if err != nil {
		t.Fatalf("expected EQP_DIFF:EQP001 in Redis, got error: %v", err)
	}
	if val == "" {
		t.Error("expected non-empty EQP_DIFF:EQP001 value")
	}
}

func TestSyncer_PeriodicSync(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	syncer := NewSyncer(mr.Addr(), cfg, nil, "EQP002", 1) // 1 second interval

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer syncer.Stop()

	firstDiff := syncer.GetDiff()

	// Wait for at least one periodic sync
	time.Sleep(1500 * time.Millisecond)

	secondDiff := syncer.GetDiff()

	// Both should be reasonable values (not necessarily different, but both should be set)
	if firstDiff < -5000 || firstDiff > 5000 {
		t.Errorf("first diff out of range: %d", firstDiff)
	}
	if secondDiff < -5000 || secondDiff > 5000 {
		t.Errorf("second diff out of range: %d", secondDiff)
	}
}

func TestSyncer_GetDiff_ThreadSafe(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	syncer := NewSyncer(mr.Addr(), cfg, nil, "EQP003", 3600)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer syncer.Stop()

	// Multiple goroutines reading GetDiff concurrently
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = syncer.GetDiff()
		}()
	}
	wg.Wait()
	// No race condition = test passes
}

func TestSyncer_Stop(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	syncer := NewSyncer(mr.Addr(), cfg, nil, "EQP004", 3600)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Stop should return without hanging
	done := make(chan struct{})
	go func() {
		syncer.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Stop timed out")
	}
}
