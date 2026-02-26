// Package main is the entry point for the ResourceAgent application.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/discovery"
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
		configPath  = flag.String("config", "conf/ResourceAgent/ResourceAgent.json", "Path to main configuration file")
		monitorPath = flag.String("monitor", "conf/ResourceAgent/Monitor.json", "Path to monitor configuration file")
		loggingPath = flag.String("logging", "conf/ResourceAgent/Logging.json", "Path to logging configuration file")
		showVersion = flag.Bool("version", false, "Show version information")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("ResourceAgent %s (built %s)\n", version, buildTime)
		os.Exit(0)
	}

	// Derive basePath from config path and change working directory.
	// When running as a Windows service, the cwd is C:\Windows\System32.
	// Absolute config path (e.g. D:\EARS\EEGAgent\conf\ResourceAgent\ResourceAgent.json)
	// means service mode — extract basePath (3 levels up from config file) and chdir.
	// Relative path means interactive/dev mode — basePath stays as ".".
	if filepath.IsAbs(*configPath) {
		basePath := filepath.Dir(filepath.Dir(filepath.Dir(*configPath)))
		if err := os.Chdir(basePath); err != nil {
			service.ReportStartupError("ResourceAgent", fmt.Errorf("failed to chdir to %s: %w", basePath, err))
			fmt.Fprintf(os.Stderr, "Failed to change directory to %s: %v\n", basePath, err)
			os.Exit(1)
		}
	}

	// Load split configuration
	cfg, mc, lc, err := config.LoadSplit(*configPath, *monitorPath, *loggingPath)
	if err != nil {
		service.ReportStartupError("ResourceAgent", err)
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger from Logging.json
	if err := logger.Init(*lc); err != nil {
		service.ReportStartupError("ResourceAgent", err)
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	log := logger.WithComponent("main")
	log.Info().
		Str("version", version).
		Str("config", *configPath).
		Str("monitor", *monitorPath).
		Str("logging", *loggingPath).
		Msg("Starting ResourceAgent")

	// Create and run service
	svc := service.NewService(func(ctx context.Context) error {
		return run(ctx, cfg, mc, lc, *monitorPath, *loggingPath)
	})

	if err := svc.Run(context.Background()); err != nil {
		log.Fatal().Err(err).Msg("Service exited with error")
	}

	log.Info().Msg("ResourceAgent stopped")
}

func run(ctx context.Context, cfg *config.Config, mc *config.MonitorConfig, lc *logger.Config, monitorPath, loggingPath string) error {
	log := logger.WithComponent("main")

	// Get agent identification
	agentID := config.GetAgentID(cfg)
	hostname := config.GetHostname(cfg)

	log.Info().
		Str("agent_id", agentID).
		Str("hostname", hostname).
		Msg("Agent initialized")

	// --- Redis EQP_INFO + ServiceDiscovery (sender_type != "file" 일 때 필수) ---
	if strings.ToLower(cfg.SenderType) != "file" {
		// 1. VirtualAddressList 검증
		if cfg.VirtualAddressList == "" {
			return fmt.Errorf("sender_type=%q requires VirtualAddressList", cfg.SenderType)
		}
		virtualIPs := strings.Split(cfg.VirtualAddressList, ",")
		virtualIP := strings.TrimSpace(virtualIPs[0])
		if virtualIP == "" {
			return fmt.Errorf("sender_type=%q but first virtual IP is empty", cfg.SenderType)
		}

		// 2. IP 감지
		ipInfo, ipErr := network.DetectIPs(cfg.PrivateIPAddressPattern, "")
		if ipErr != nil {
			return fmt.Errorf("failed to detect IP addresses: %w", ipErr)
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
				Msg("SOCKS proxy configured")
		}

		// 3. Redis EQP_INFO 취득 (필수)
		redisAddress := fmt.Sprintf("%s:%d", virtualIP, cfg.Redis.Port)
		log.Info().
			Str("virtual_ip", virtualIP).
			Str("redis_address", redisAddress).
			Msg("Redis address resolved from VirtualAddressList")

		info, fetchErr := eqpinfo.FetchEqpInfo(ctx, redisAddress, cfg.Redis, dialFunc, ipInfo.IPAddr, ipInfo.IPAddrLocal)
		if fetchErr != nil {
			return fmt.Errorf("failed to fetch EQP_INFO from Redis: %w", fetchErr)
		}
		if info == nil {
			return fmt.Errorf("EQP_INFO not found for %s:%s", ipInfo.IPAddr, ipInfo.IPAddrLocal)
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

		// 4. ServiceDiscovery 호출 (필수)
		services, sdErr := discovery.FetchServices(ctx, virtualIP, cfg.ServiceDiscoveryPort, info.Index, dialFunc)
		if sdErr != nil {
			return fmt.Errorf("ServiceDiscovery failed: %w", sdErr)
		}

		// 5. KafkaRest 주소 추출
		kafkaRestAddr, krErr := discovery.GetKafkaRestAddress(services)
		if krErr != nil {
			return fmt.Errorf("failed to get KafkaRest address: %w", krErr)
		}
		cfg.KafkaRestAddress = kafkaRestAddr
		log.Info().
			Str("kafkarest_addr", kafkaRestAddr).
			Msg("KafkaRest address resolved from ServiceDiscovery")
	}
	// --- END Redis EQP_INFO + ServiceDiscovery ---

	// Create collector registry with default collectors
	registry := collector.DefaultRegistry()

	// Configure collectors from MonitorConfig
	if err := registry.Configure(mc.Collectors); err != nil {
		return fmt.Errorf("failed to configure collectors: %w", err)
	}

	// Consolidate console setting: Logging.json Console is the master switch
	cfg.File.Console = lc.Console

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
	switch strings.ToLower(cfg.SenderType) {
	case "file":
		log.Info().
			Str("file_path", cfg.File.FilePath).
			Bool("console", cfg.File.Console).
			Msg("Using file sender")
	case "kafkarest":
		topic := config.ResolveTopic(cfg.ResourceMonitorTopic, cfg.EqpInfo)
		log.Info().
			Str("kafkarest_addr", cfg.KafkaRestAddress).
			Str("topic", topic).
			Msg("Using KafkaRest sender")
	default:
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

	// Set up Monitor.json watcher for hot reload
	var watcherMu sync.Mutex

	monitorWatcher, err := config.NewMonitorWatcher(monitorPath, func(newMC *config.MonitorConfig) {
		watcherMu.Lock()
		defer watcherMu.Unlock()

		log.Info().Msg("Applying monitor configuration changes")

		if err := registry.Configure(newMC.Collectors); err != nil {
			log.Error().Err(err).Msg("Failed to update collector configurations")
			return
		}

		sched.Reconfigure()
		log.Info().Msg("Monitor configuration updated")
	})

	if err != nil {
		log.Warn().Err(err).Msg("Failed to create monitor watcher, hot reload disabled")
	} else {
		if err := monitorWatcher.Start(); err != nil {
			log.Warn().Err(err).Msg("Failed to start monitor watcher")
		} else {
			defer func() {
				log.Info().Msg("Stopping monitor watcher")
				if err := monitorWatcher.Stop(); err != nil {
					log.Error().Err(err).Msg("Error stopping monitor watcher")
				}
			}()
		}
	}

	// Set up Logging.json watcher for hot reload
	loggingWatcher, err := config.NewLoggingWatcher(loggingPath, func(newLC *logger.Config) {
		watcherMu.Lock()
		defer watcherMu.Unlock()

		log.Info().Msg("Applying logging configuration changes")

		if err := logger.Init(*newLC); err != nil {
			log.Error().Err(err).Msg("Failed to update logging configuration")
			return
		}

		// Sync console setting to FileSender (Logging.json Console is the master switch)
		if fs, ok := snd.(*sender.FileSender); ok {
			fs.SetConsole(newLC.Console)
			log.Info().Bool("console", newLC.Console).Msg("FileSender console updated")
		}

		log.Info().Msg("Logging configuration updated")
	})

	if err != nil {
		log.Warn().Err(err).Msg("Failed to create logging watcher, hot reload disabled")
	} else {
		if err := loggingWatcher.Start(); err != nil {
			log.Warn().Err(err).Msg("Failed to start logging watcher")
		} else {
			defer func() {
				log.Info().Msg("Stopping logging watcher")
				if err := loggingWatcher.Stop(); err != nil {
					log.Error().Err(err).Msg("Error stopping logging watcher")
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
