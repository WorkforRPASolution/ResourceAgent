// Package main is the entry point for the ResourceAgent application.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/eqpinfo"
	"resourceagent/internal/logger"
	"resourceagent/internal/network"
	"resourceagent/internal/scheduler"
	"resourceagent/internal/sender"
	"resourceagent/internal/service"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	var (
		configPath  = flag.String("config", "configs/config.json", "Path to configuration file")
		showVersion = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("ResourceAgent %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	if err := logger.Init(cfg.Logging); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	log := logger.WithComponent("main")
	log.Info().
		Str("version", version).
		Str("config", *configPath).
		Msg("Starting ResourceAgent")

	// Create and run service
	svc := service.NewService(func(ctx context.Context) error {
		return run(ctx, cfg, *configPath)
	})

	if err := svc.Run(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Service exited with error")
	}

	log.Info().Msg("ResourceAgent stopped")
}

func run(ctx context.Context, cfg *config.Config, configPath string) error {
	log := logger.WithComponent("main")

	// Get agent identification
	agentID := config.GetAgentID(cfg)
	hostname := config.GetHostname(cfg)

	log.Info().
		Str("agent_id", agentID).
		Str("hostname", hostname).
		Msg("Agent initialized")

	// --- IP detection and Redis EQP_INFO enrichment ---
	if cfg.Redis.Enabled {
		// Resolve VirtualIPList → first IP for Redis address
		if cfg.VirtualIPList == "" {
			return fmt.Errorf("redis enabled but virtual_ip_list is empty")
		}
		virtualIPs := strings.Split(cfg.VirtualIPList, ",")
		virtualIP := strings.TrimSpace(virtualIPs[0])
		if virtualIP == "" {
			return fmt.Errorf("redis enabled but first virtual IP is empty")
		}
		redisAddress := fmt.Sprintf("%s:%d", virtualIP, cfg.Redis.Port)

		log.Info().
			Str("virtual_ip", virtualIP).
			Str("redis_address", redisAddress).
			Msg("Redis address resolved from VirtualIPList")

		// Detect IP addresses
		ipInfo, ipErr := network.DetectIPs(cfg.PrivateIPAddressPattern, "")
		if ipErr != nil {
			return fmt.Errorf("redis enabled but failed to detect IP addresses: %w", ipErr)
		}

		log.Info().
			Str("ip_addr", ipInfo.IPAddr).
			Str("ip_addr_local", ipInfo.IPAddrLocal).
			Strs("all_ips", ipInfo.AllIPs).
			Msg("IP addresses detected")

		// Create SOCKS dialer if configured
		var dialFunc func(string, string) (net.Conn, error)
		if cfg.SOCKSProxy.Host != "" && cfg.SOCKSProxy.Port > 0 {
			dialFunc = network.DialerFunc(cfg.SOCKSProxy.Host, cfg.SOCKSProxy.Port)
			log.Info().
				Str("socks_host", cfg.SOCKSProxy.Host).
				Int("socks_port", cfg.SOCKSProxy.Port).
				Msg("SOCKS proxy configured for Redis")
		}

		// Fetch EQP_INFO from Redis
		info, fetchErr := eqpinfo.FetchEqpInfo(ctx, redisAddress, cfg.Redis, dialFunc, ipInfo.IPAddr, ipInfo.IPAddrLocal)
		if fetchErr != nil {
			return fmt.Errorf("redis enabled but failed to fetch EQP_INFO: %w", fetchErr)
		}
		if info == nil {
			return fmt.Errorf("redis enabled but EQP_INFO not found for %s:%s", ipInfo.IPAddr, ipInfo.IPAddrLocal)
		}

		cfg.EqpInfo = &config.EqpInfoConfig{
			Process:  info.Process,
			EqpModel: info.EqpModel,
			EqpID:    info.EqpID,
			Line:     info.Line,
			LineDesc: info.LineDesc,
			Index:    info.Index,
		}
		agentID = info.EqpID
		log.Info().
			Str("eqp_id", info.EqpID).
			Str("process", info.Process).
			Str("eqp_model", info.EqpModel).
			Str("line", info.Line).
			Str("line_desc", info.LineDesc).
			Str("index", info.Index).
			Msg("EQP_INFO loaded from Redis")
	}
	// --- END IP detection and Redis EQP_INFO enrichment ---

	// Create collector registry with default collectors
	registry := collector.DefaultRegistry()

	// Configure collectors
	if err := registry.Configure(cfg.Collectors); err != nil {
		return fmt.Errorf("failed to configure collectors: %w", err)
	}

	// Create sender based on configuration
	snd, err := sender.NewSender(cfg)
	if err != nil {
		return fmt.Errorf("failed to create sender: %w", err)
	}
	defer func() {
		log.Info().Msg("Closing sender")
		if err := snd.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing sender")
		}
	}()

	// Log sender-specific information
	if cfg.SenderType == "file" {
		log.Info().
			Str("file_path", cfg.File.FilePath).
			Bool("console", cfg.File.Console).
			Msg("Using file sender")
	} else {
		log.Info().
			Strs("brokers", cfg.Kafka.Brokers).
			Str("topic", cfg.Kafka.Topic).
			Msg("Connected to Kafka")
	}

	// Create scheduler
	sched := scheduler.New(registry, snd, agentID, hostname, cfg.Agent.Tags)

	// Start scheduler
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Set up configuration watcher for hot reload
	var watcher *config.Watcher
	var watcherMu sync.Mutex

	watcher, err = config.NewWatcher(configPath, func(newCfg *config.Config) {
		watcherMu.Lock()
		defer watcherMu.Unlock()

		log.Info().Msg("Applying configuration changes")

		// Update logging level
		if err := logger.Init(newCfg.Logging); err != nil {
			log.Error().Err(err).Msg("Failed to update logging configuration")
		}

		// Update collector configurations
		if err := registry.Configure(newCfg.Collectors); err != nil {
			log.Error().Err(err).Msg("Failed to update collector configurations")
		}

		log.Info().Msg("Configuration updated")
	})

	if err != nil {
		log.Warn().Err(err).Msg("Failed to create configuration watcher, hot reload disabled")
	} else {
		if err := watcher.Start(); err != nil {
			log.Warn().Err(err).Msg("Failed to start configuration watcher")
		} else {
			defer func() {
				log.Info().Msg("Stopping configuration watcher")
				if err := watcher.Stop(); err != nil {
					log.Error().Err(err).Msg("Error stopping configuration watcher")
				}
			}()
		}
	}

	// Wait for context cancellation (shutdown signal)
	<-ctx.Done()
	log.Info().Msg("Received shutdown signal")

	// Stop scheduler (waits for all collectors to finish)
	sched.Stop()

	return nil
}
