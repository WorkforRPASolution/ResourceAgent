package heartbeat

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"resourceagent/internal/config"
)

func TestBuildKey(t *testing.T) {
	got := BuildKey("ARS", "M1", "EQP01")
	want := "AgentRunning:ARS-M1-EQP01"
	if got != want {
		t.Errorf("BuildKey() = %q, want %q", got, want)
	}
}

func TestSender_SendOnce(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP001")

	ctx := context.Background()
	s.sendOnce(ctx)

	mr.Select(10)
	key := BuildKey("ARS", "MODEL1", "EQP001")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s in Redis, got error: %v", key, err)
	}

	// Value should be a number (uptime seconds)
	uptime, err := strconv.Atoi(val)
	if err != nil {
		t.Fatalf("expected numeric value, got %q: %v", val, err)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %d", uptime)
	}

	// TTL should be 30s
	ttl := mr.TTL(key)
	if ttl != DefaultTTL {
		t.Errorf("expected TTL %v, got %v", DefaultTTL, ttl)
	}
}

func TestSender_UptimeIncreases(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP002")
	s.interval = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	// Wait for at least one tick
	time.Sleep(2 * time.Second)

	mr.Select(10)
	key := BuildKey("ARS", "MODEL1", "EQP002")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s, got error: %v", key, err)
	}

	uptime, err := strconv.Atoi(val)
	if err != nil {
		t.Fatalf("expected numeric value, got %q", val)
	}
	if uptime < 1 {
		t.Errorf("expected uptime >= 1, got %d", uptime)
	}
}

func TestSender_Stop(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP003")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Stop timed out")
	}
}

// TestE2E_RealRedis verifies heartbeat against a real Redis instance.
// Run with: REDIS_E2E=localhost:6379 go test ./internal/heartbeat/... -v -run TestE2E
func TestE2E_RealRedis(t *testing.T) {
	addr := os.Getenv("REDIS_E2E")
	if addr == "" {
		t.Skip("REDIS_E2E not set, skipping E2E test")
	}

	cfg := config.RedisConfig{Port: 6379, DB: 10}
	key := BuildKey("E2E_TEST", "HEARTBEAT", "E2E_001")

	// Clean up before and after
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: cfg.ResolvePassword(), DB: cfg.DB})
	defer rdb.Close()
	ctx := context.Background()
	rdb.Del(ctx, key)
	defer rdb.Del(ctx, key)

	s := NewSender(addr, cfg, nil, "E2E_TEST", "HEARTBEAT", "E2E_001")
	s.interval = 1 * time.Second
	s.Start(ctx)

	// Wait for at least one send
	time.Sleep(2 * time.Second)
	s.Stop()

	// Verify key exists with valid uptime value
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("key %s not found in real Redis: %v", key, err)
	}

	uptime, err := strconv.Atoi(val)
	if err != nil {
		t.Fatalf("expected numeric value, got %q: %v", val, err)
	}
	if uptime < 1 {
		t.Errorf("expected uptime >= 1, got %d", uptime)
	}

	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL check failed: %v", err)
	}
	if ttl <= 0 || ttl > DefaultTTL {
		t.Errorf("expected TTL in (0, 30s], got %v", ttl)
	}

	t.Logf("E2E PASS: key=%s, uptime=%d, ttl=%v", key, uptime, ttl)
}

func TestSender_RedisDown(t *testing.T) {
	cfg := config.RedisConfig{Port: 6379, DB: 10}
	s := NewSender("localhost:1", cfg, nil, "ARS", "MODEL1", "EQP004")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start should not panic even with bad address
	s.Start(ctx)

	done := make(chan struct{})
	go func() {
		s.Stop()
		close(done)
	}()

	select {
	case <-done:
		// OK
	case <-time.After(3 * time.Second):
		t.Fatal("Stop timed out with bad Redis address")
	}
}
