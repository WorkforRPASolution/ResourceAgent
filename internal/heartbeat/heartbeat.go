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
	DefaultInterval = 10 * time.Second
	DefaultTTL      = 30 * time.Second
	KeyPrefix       = "AgentRunning"
)

// Sender periodically sends a heartbeat to Redis via SETEX.
type Sender struct {
	redisAddr string
	redisCfg  config.RedisConfig
	dialFunc  func(string, string) (net.Conn, error)
	key       string
	startTime time.Time
	interval  time.Duration
	ttl       time.Duration
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// BuildKey returns the Redis key for the heartbeat: "AgentRunning:{process}-{eqpModel}-{eqpID}".
func BuildKey(process, eqpModel, eqpID string) string {
	return fmt.Sprintf("%s:%s-%s-%s", KeyPrefix, process, eqpModel, eqpID)
}

// NewSender creates a new heartbeat Sender.
func NewSender(redisAddr string, redisCfg config.RedisConfig, dialFunc func(string, string) (net.Conn, error),
	process, eqpModel, eqpID string) *Sender {
	return &Sender{
		redisAddr: redisAddr,
		redisCfg:  redisCfg,
		dialFunc:  dialFunc,
		key:       BuildKey(process, eqpModel, eqpID),
		startTime: time.Now(),
		interval:  DefaultInterval,
		ttl:       DefaultTTL,
	}
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

// Stop cancels the background ticker and waits for it to finish.
func (s *Sender) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
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

	client, err := createRedisClient(s.redisAddr, s.redisCfg, s.dialFunc)
	if err != nil {
		log.Warn().Err(err).Msg("failed to create Redis client for heartbeat")
		return
	}
	defer client.Close()

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	uptimeSeconds := int64(time.Since(s.startTime).Seconds())

	if err := client.SetEx(queryCtx, s.key, uptimeSeconds, s.ttl).Err(); err != nil {
		log.Debug().Err(err).Str("key", s.key).Msg("heartbeat SETEX failed")
		return
	}

	log.Debug().Str("key", s.key).Int64("uptime_s", uptimeSeconds).Msg("heartbeat sent")
}

// createRedisClient creates a Redis client with optional custom dialer.
func createRedisClient(redisAddress string, cfg config.RedisConfig, dialFunc func(network, addr string) (net.Conn, error)) (*redis.Client, error) {
	opts := &redis.Options{
		Addr:     redisAddress,
		Password: cfg.ResolvePassword(),
		DB:       cfg.DB,
	}

	if dialFunc != nil {
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialFunc(network, addr)
		}
	}

	return redis.NewClient(opts), nil
}
