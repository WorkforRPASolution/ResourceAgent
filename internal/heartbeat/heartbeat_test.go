package heartbeat

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"resourceagent/internal/config"
)

// parseHeartbeatValue parses "STATUS:uptime" or "STATUS:uptime:reason"
func parseHeartbeatValue(val string) (status string, uptime int64, reason string) {
	parts := strings.SplitN(val, ":", 3)
	status = parts[0]
	if len(parts) >= 2 {
		uptime, _ = strconv.ParseInt(parts[1], 10, 64)
	}
	if len(parts) >= 3 {
		reason = parts[2]
	}
	return
}

func TestBuildKey(t *testing.T) {
	got := BuildKey("resource_agent", "ARS", "M1", "EQP01")
	want := "AgentHealth:resource_agent:ARS-M1-EQP01"
	if got != want {
		t.Errorf("BuildKey() = %q, want %q", got, want)
	}
}

func TestSender_SendOnce(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP001")

	ctx := context.Background()
	s.sendOnce(ctx)

	mr.Select(0)
	key := BuildKey(AgentGroup, "ARS", "MODEL1", "EQP001")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s in Redis, got error: %v", key, err)
	}

	// Value should be "OK:N" format
	status, uptime, reason := parseHeartbeatValue(val)
	if status != "OK" {
		t.Errorf("expected status OK, got %q", status)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %d", uptime)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}

	// TTL should be 30s
	ttl := mr.TTL(key)
	if ttl != DefaultTTL {
		t.Errorf("expected TTL %v, got %v", DefaultTTL, ttl)
	}
}

func TestSender_UptimeIncreases(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP002")
	s.interval = 1 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	// Wait for at least one tick
	time.Sleep(2 * time.Second)

	mr.Select(0)
	key := BuildKey(AgentGroup, "ARS", "MODEL1", "EQP002")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s, got error: %v", key, err)
	}

	// Value should be "OK:N" format with N >= 1
	status, uptime, _ := parseHeartbeatValue(val)
	if status != "OK" {
		t.Errorf("expected status OK, got %q", status)
	}
	if uptime < 1 {
		t.Errorf("expected uptime >= 1, got %d", uptime)
	}
}

func TestSender_Stop(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
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
		// OK — Stop completed
	case <-time.After(3 * time.Second):
		t.Fatal("Stop timed out")
	}

	// After Stop, the value should be "SHUTDOWN:N"
	mr.Select(0)
	key := BuildKey(AgentGroup, "ARS", "MODEL1", "EQP003")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s after Stop, got error: %v", key, err)
	}

	status, uptime, _ := parseHeartbeatValue(val)
	if status != "SHUTDOWN" {
		t.Errorf("expected status SHUTDOWN after Stop, got %q (val=%q)", status, val)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %d", uptime)
	}
}

func TestSender_SetHealthCheck_WARN(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP005")

	// Set a healthCheck that returns WARN
	s.SetHealthCheck(func() (string, string) {
		return "WARN", "no_collection"
	})

	ctx := context.Background()
	s.sendOnce(ctx)

	mr.Select(0)
	key := BuildKey(AgentGroup, "ARS", "MODEL1", "EQP005")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s, got error: %v", key, err)
	}

	// Value should be "WARN:N:no_collection"
	status, uptime, reason := parseHeartbeatValue(val)
	if status != "WARN" {
		t.Errorf("expected status WARN, got %q", status)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %d", uptime)
	}
	if reason != "no_collection" {
		t.Errorf("expected reason 'no_collection', got %q", reason)
	}
}

func TestSender_SetHealthCheck_OK(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP006")

	// Set a healthCheck that returns OK with empty reason
	s.SetHealthCheck(func() (string, string) {
		return "OK", ""
	})

	ctx := context.Background()
	s.sendOnce(ctx)

	mr.Select(0)
	key := BuildKey(AgentGroup, "ARS", "MODEL1", "EQP006")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s, got error: %v", key, err)
	}

	// Value should be "OK:N" (no reason appended)
	status, uptime, reason := parseHeartbeatValue(val)
	if status != "OK" {
		t.Errorf("expected status OK, got %q", status)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %d", uptime)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestSender_NilHealthCheck(t *testing.T) {
	mr := miniredis.RunT(t)

	cfg := config.RedisConfig{Port: 6379}
	s := NewSender(mr.Addr(), cfg, nil, "ARS", "MODEL1", "EQP007")

	// healthCheck is nil by default — should produce "OK:N"
	ctx := context.Background()
	s.sendOnce(ctx)

	mr.Select(0)
	key := BuildKey(AgentGroup, "ARS", "MODEL1", "EQP007")
	val, err := mr.Get(key)
	if err != nil {
		t.Fatalf("expected key %s, got error: %v", key, err)
	}

	status, uptime, reason := parseHeartbeatValue(val)
	if status != "OK" {
		t.Errorf("expected status OK, got %q", status)
	}
	if uptime < 0 {
		t.Errorf("expected non-negative uptime, got %d", uptime)
	}
	if reason != "" {
		t.Errorf("expected empty reason, got %q", reason)
	}
}

func TestSender_RedisDown(t *testing.T) {
	cfg := config.RedisConfig{Port: 6379}
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

// TestE2E_RealRedis verifies heartbeat against a real Redis instance.
// Run with: REDIS_E2E=localhost:6379 go test ./internal/heartbeat/... -v -run TestE2E
func TestE2E_RealRedis(t *testing.T) {
	addr := os.Getenv("REDIS_E2E")
	if addr == "" {
		t.Skip("REDIS_E2E not set, skipping E2E test")
	}

	cfg := config.RedisConfig{Port: 6379}
	key := BuildKey(AgentGroup, "E2E_TEST", "HEARTBEAT", "E2E_001")

	// Verify key uses AgentHealth prefix
	if !strings.HasPrefix(key, "AgentHealth:") {
		t.Fatalf("expected AgentHealth prefix, got key=%s", key)
	}

	// Clean up before and after
	rdb := redis.NewClient(&redis.Options{Addr: addr, Password: cfg.ResolvePassword(), DB: HeartbeatDB})
	defer rdb.Close()
	ctx := context.Background()
	rdb.Del(ctx, key)
	defer rdb.Del(ctx, key)

	s := NewSender(addr, cfg, nil, "E2E_TEST", "HEARTBEAT", "E2E_001")
	s.interval = 1 * time.Second
	s.Start(ctx)

	// Wait for at least one send
	time.Sleep(2 * time.Second)

	// Check value before Stop — should be "OK:N"
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("key %s not found in real Redis: %v", key, err)
	}

	status, uptime, _ := parseHeartbeatValue(val)
	if status != "OK" {
		t.Errorf("expected status OK, got %q", status)
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

	// Stop and verify SHUTDOWN
	s.Stop()

	val, err = rdb.Get(ctx, key).Result()
	if err != nil {
		t.Fatalf("key %s not found after Stop: %v", key, err)
	}

	status, uptime, _ = parseHeartbeatValue(val)
	if status != "SHUTDOWN" {
		t.Errorf("expected status SHUTDOWN after Stop, got %q (val=%q)", status, val)
	}
	if uptime < 1 {
		t.Errorf("expected uptime >= 1 after Stop, got %d", uptime)
	}

	t.Logf("E2E PASS: key=%s, val=%s, ttl=%v", key, val, ttl)
}
