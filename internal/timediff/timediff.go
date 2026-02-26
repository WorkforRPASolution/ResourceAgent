// Package timediff provides periodic server-PC time difference measurement via Redis TIME command.
package timediff

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// Syncer periodically measures the time difference between the local machine and a Redis server.
type Syncer struct {
	diff      atomic.Int64
	redisAddr string
	redisCfg  config.RedisConfig
	dialFunc  func(string, string) (net.Conn, error)
	eqpID     string
	interval  time.Duration
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewSyncer creates a new Syncer.
func NewSyncer(redisAddr string, redisCfg config.RedisConfig, dialFunc func(string, string) (net.Conn, error), eqpID string, intervalSec int) *Syncer {
	return &Syncer{
		redisAddr: redisAddr,
		redisCfg:  redisCfg,
		dialFunc:  dialFunc,
		eqpID:     eqpID,
		interval:  time.Duration(intervalSec) * time.Second,
	}
}

// Start performs the first sync synchronously (returns error on failure),
// then starts a background ticker for periodic syncs.
func (s *Syncer) Start(ctx context.Context) error {
	if err := s.syncOnce(ctx); err != nil {
		return fmt.Errorf("initial time diff sync failed: %w", err)
	}

	tickCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	s.wg.Add(1)
	go s.loop(tickCtx)

	return nil
}

// Stop cancels the background ticker and waits for it to finish.
func (s *Syncer) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
}

// GetDiff returns the current time difference in milliseconds (local - server).
func (s *Syncer) GetDiff() int64 {
	return s.diff.Load()
}

func (s *Syncer) loop(ctx context.Context) {
	defer s.wg.Done()
	log := logger.WithComponent("timediff")

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.syncOnce(ctx); err != nil {
				log.Warn().Err(err).Msg("periodic time diff sync failed")
			}
		}
	}
}

func (s *Syncer) syncOnce(ctx context.Context) error {
	log := logger.WithComponent("timediff")

	client, err := createRedisClient(s.redisAddr, s.redisCfg, s.dialFunc)
	if err != nil {
		return fmt.Errorf("failed to create Redis client: %w", err)
	}
	defer client.Close()

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	serverTime, err := client.Time(queryCtx).Result()
	if err != nil {
		return fmt.Errorf("Redis TIME failed: %w", err)
	}

	serverTimeMs := serverTime.UnixMilli()
	myTimeMs := time.Now().UnixMilli()
	diff := myTimeMs - serverTimeMs

	// Store diff in Redis
	key := fmt.Sprintf("EQP_DIFF:%s", s.eqpID)
	if err := client.Set(queryCtx, key, diff, 0).Err(); err != nil {
		return fmt.Errorf("Redis SET %s failed: %w", key, err)
	}

	s.diff.Store(diff)

	log.Info().
		Int64("server_timestamp", serverTimeMs).
		Int64("my_timestamp", myTimeMs).
		Int64("diff", diff).
		Msg("time diff synced")

	return nil
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
