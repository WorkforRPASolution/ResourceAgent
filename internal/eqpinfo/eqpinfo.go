package eqpinfo

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"resourceagent/internal/config"
)

// EqpInfo contains equipment information retrieved from Redis.
type EqpInfo struct {
	Process  string
	EqpModel string
	EqpID    string
	Line     string
	LineDesc string
	Index    string
}

// ParseEqpInfoValue parses a colon-separated EQP_INFO value.
// Expected format: "process:eqpModel:eqpid:line:lineDesc:index"
func ParseEqpInfoValue(value string) (*EqpInfo, error) {
	parts := strings.Split(value, ":")
	if len(parts) < 6 {
		return nil, fmt.Errorf("invalid EQP_INFO value: expected 6 colon-separated segments, got %d in %q", len(parts), value)
	}

	return &EqpInfo{
		Process:  parts[0],
		EqpModel: parts[1],
		EqpID:    parts[2],
		Line:     parts[3],
		LineDesc: parts[4],
		Index:    parts[5],
	}, nil
}

// FetchEqpInfo retrieves equipment info from Redis.
// redisAddress is the resolved address (e.g., "virtualIP:port").
// dialFunc is optional - if non-nil, used as custom dialer (e.g., SOCKS proxy).
// Returns nil (not error) if key is not found - this is expected for new/unknown machines.
func FetchEqpInfo(ctx context.Context, redisAddress string, redisCfg config.RedisConfig,
	dialFunc func(network, addr string) (net.Conn, error),
	ipAddr, ipAddrLocal string) (*EqpInfo, error) {

	// Create Redis client
	client, err := createRedisClient(redisAddress, redisCfg, dialFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create Redis client: %w", err)
	}
	defer client.Close()

	// Build the hash key
	hashKey := fmt.Sprintf("%s:%s", ipAddr, ipAddrLocal)

	// Set timeout for Redis operation
	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// HGET EQP_INFO {ipAddr}:{ipAddrLocal}
	value, err := client.HGet(queryCtx, "EQP_INFO", hashKey).Result()
	if err == redis.Nil {
		// Key not found - this is normal for unknown machines
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Redis HGET EQP_INFO %s failed: %w", hashKey, err)
	}

	return ParseEqpInfoValue(value)
}

// createRedisClient creates a Redis client with optional custom dialer.
// redisAddress is the resolved address (e.g., "virtualIP:port").
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
