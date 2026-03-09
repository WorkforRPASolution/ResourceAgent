// Package metainfo writes agent version metadata to Redis DB 0.
// WebManager reads this to display ResourceAgent version in the UI.
// TODO: Migrate to HTTP → EARSInterfaceServer → MongoDB direct write.
package metainfo

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/redis/go-redis/v9"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

const (
	MetaInfoDB = 0
	KeyPrefix  = "ResourceAgentMetaInfo"
)

// BuildKey returns the Redis hash key: "ResourceAgentMetaInfo:{process}-{eqpModel}".
func BuildKey(process, eqpModel string) string {
	return fmt.Sprintf("%s:%s-%s", KeyPrefix, process, eqpModel)
}

// WriteVersion writes the agent version to Redis as HSET ResourceAgentMetaInfo:{process}-{model} {eqpID} {version}.
// This is a best-effort one-time write at startup; failures are logged but not fatal.
func WriteVersion(ctx context.Context, redisAddr string, redisCfg config.RedisConfig,
	dialFunc func(string, string) (net.Conn, error),
	process, eqpModel, eqpID, version string) {

	log := logger.WithComponent("metainfo")

	opts := &redis.Options{
		Addr:     redisAddr,
		Password: redisCfg.ResolvePassword(),
		DB:       MetaInfoDB,
	}
	if dialFunc != nil {
		opts.Dialer = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialFunc(network, addr)
		}
	}

	client := redis.NewClient(opts)
	defer client.Close()

	queryCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	key := BuildKey(process, eqpModel)
	if err := client.HSet(queryCtx, key, eqpID, version).Err(); err != nil {
		log.Warn().Err(err).Str("key", key).Str("eqpId", eqpID).Msg("failed to write version to Redis")
		return
	}

	log.Info().Str("key", key).Str("eqpId", eqpID).Str("version", version).Msg("version written to Redis")
}
