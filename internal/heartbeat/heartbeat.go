// Package heartbeat provides periodic Redis heartbeat for alive-check.
package heartbeat

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

const (
	DefaultInterval    = 10 * time.Second
	DefaultTTL         = 30 * time.Second
	HealthKeyPrefix    = "AgentHealth"
	AgentGroup         = "resource_agent"
	StalenessThreshold = 90 * time.Second
	HeartbeatDB        = 0 // AgentHealth always writes to DB 0
)

// Sender periodically sends a heartbeat to Redis via SETEX.
type Sender struct {
	redisAddr   string
	redisCfg    config.RedisConfig
	dialFunc    func(string, string) (net.Conn, error)
	key         string
	startTime   time.Time
	interval    time.Duration
	ttl         time.Duration
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	healthCheck func() (string, string) // (status, reason) — nil means always OK
	mu          sync.RWMutex            // protects healthCheck

	clientMu sync.Mutex
	client   *redis.Client // lazy-initialised on first send
}

// BuildKey returns the Redis key for the heartbeat: "AgentHealth:{agentGroup}:{process}-{eqpModel}-{eqpID}".
func BuildKey(agentGroup, process, eqpModel, eqpID string) string {
	return fmt.Sprintf("%s:%s:%s-%s-%s", HealthKeyPrefix, agentGroup, process, eqpModel, eqpID)
}

// NewSender creates a new heartbeat Sender.
func NewSender(redisAddr string, redisCfg config.RedisConfig, dialFunc func(string, string) (net.Conn, error),
	process, eqpModel, eqpID string) *Sender {
	return &Sender{
		redisAddr: redisAddr,
		redisCfg:  redisCfg,
		dialFunc:  dialFunc,
		key:       BuildKey(AgentGroup, process, eqpModel, eqpID),
		startTime: time.Now(),
		interval:  DefaultInterval,
		ttl:       DefaultTTL,
	}
}

// SetHealthCheck sets a function that returns (status, reason) for heartbeat values.
// If fn is nil, the heartbeat always reports "OK" with no reason.
func (s *Sender) SetHealthCheck(fn func() (string, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.healthCheck = fn
}

// Start sends the first heartbeat (logging on failure) and starts a background ticker.
func (s *Sender) Start(ctx context.Context) {
	log := logger.WithComponent("heartbeat")

	s.sendOnce(ctx)

	tickCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.wg.Add(1)
	go s.loop(tickCtx)

	log.Info().Str("key", s.key).Dur("interval", s.interval).Msg("heartbeat started")
}

// Stop cancels the background ticker, waits for it to finish, and records a SHUTDOWN heartbeat.
func (s *Sender) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()

	// SHUTDOWN status (best-effort)
	s.sendShutdown()

	s.clientMu.Lock()
	if s.client != nil {
		s.client.Close()
		s.client = nil
	}
	s.clientMu.Unlock()
}

// ensureClient returns the Redis client, lazily creating it on first use.
// Returns nil if creation fails — callers must treat heartbeat as best-effort.
func (s *Sender) ensureClient() *redis.Client {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	if s.client != nil {
		return s.client
	}
	c, err := createRedisClient(s.redisAddr, s.redisCfg, s.dialFunc)
	if err != nil {
		return nil
	}
	s.client = c
	return s.client
}

func (s *Sender) sendShutdown() {
	log := logger.WithComponent("heartbeat")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	client := s.ensureClient()
	if client == nil {
		log.Warn().Msg("Redis client unavailable for shutdown heartbeat")
		return
	}

	uptimeSeconds := int64(time.Since(s.startTime).Seconds())
	value := fmt.Sprintf("SHUTDOWN:%d", uptimeSeconds)

	if err := client.SetEx(ctx, s.key, value, s.ttl).Err(); err != nil {
		log.Debug().Err(err).Str("key", s.key).Msg("shutdown heartbeat SETEX failed")
		return
	}

	log.Info().Str("key", s.key).Str("value", value).Msg("shutdown heartbeat sent")
}

func (s *Sender) loop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendOnce(ctx)
		}
	}
}

func (s *Sender) sendOnce(ctx context.Context) {
	log := logger.WithComponent("heartbeat")

	client := s.ensureClient()
	if client == nil {
		log.Warn().Msg("Redis client unavailable for heartbeat (will retry on next tick)")
		return
	}

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	uptimeSeconds := int64(time.Since(s.startTime).Seconds())

	// Call healthCheck
	s.mu.RLock()
	hc := s.healthCheck
	s.mu.RUnlock()

	status := "OK"
	reason := ""
	if hc != nil {
		status, reason = hc()
	}

	// Value format: "OK:3600" or "WARN:3600:no_collection"
	var value string
	if reason != "" {
		value = fmt.Sprintf("%s:%d:%s", status, uptimeSeconds, reason)
	} else {
		value = fmt.Sprintf("%s:%d", status, uptimeSeconds)
	}

	if err := client.SetEx(queryCtx, s.key, value, s.ttl).Err(); err != nil {
		log.Debug().Err(err).Str("key", s.key).Msg("heartbeat SETEX failed")
		return
	}

	if status != "OK" {
		log.Warn().Str("key", s.key).Str("value", value).Msg("heartbeat sent with abnormal status")
	} else {
		log.Debug().Str("key", s.key).Str("value", value).Msg("heartbeat sent")
	}
}

// createRedisClient creates a Redis client with optional custom dialer.
func createRedisClient(redisAddress string, cfg config.RedisConfig, dialFunc func(network, addr string) (net.Conn, error)) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:     redisAddress,
		Password: cfg.ResolvePassword(),
		DB:       HeartbeatDB,
	}

	if dialFunc != nil {
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialFunc(network, addr)
		}
	}

	return redis.NewClient(opts), nil
}
