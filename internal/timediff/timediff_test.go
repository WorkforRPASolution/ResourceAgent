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

// 핵심: Start에서 client를 1회 생성, 이후 syncOnce는 동일 client 재사용.
func TestSyncer_ReusesRedisClientAfterStart(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
	syncer := NewSyncer(mr.Addr(), cfg, nil, "EQP_REUSE", 3600)

	if syncer.client != nil {
		t.Fatal("expected client to be nil before Start")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := syncer.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer syncer.Stop()

	first := syncer.client
	if first == nil {
		t.Fatal("expected client to be set after Start")
	}

	// 추가 sync 호출 후에도 동일 client 사용
	if err := syncer.syncOnce(ctx); err != nil {
		t.Fatalf("syncOnce failed: %v", err)
	}
	if syncer.client != first {
		t.Error("expected client to be reused across syncOnce calls")
	}
}

// 핵심: Start에서 Redis 연결 실패하면 fatal (기존 의미 보존).
func TestSyncer_Start_FailsFastOnBadAddr(t *testing.T) {
	cfg := config.RedisConfig{Port: 6379}
	syncer := NewSyncer("127.0.0.1:1", cfg, nil, "EQP_FAIL", 3600)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := syncer.Start(ctx); err == nil {
		syncer.Stop()
		t.Fatal("expected Start to fail with bad Redis address")
	}
}

func TestSyncer_Start(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
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

	cfg := config.RedisConfig{Port: 6379}
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

	cfg := config.RedisConfig{Port: 6379}
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

	cfg := config.RedisConfig{Port: 6379}
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
