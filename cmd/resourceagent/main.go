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
	"resourceagent/internal/timediff"
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
	const startupErrorLogDir = "log/ResourceAgent"

	if filepath.IsAbs(*configPath) {
		basePath := filepath.Dir(filepath.Dir(filepath.Dir(*configPath)))
		if err := os.Chdir(basePath); err != nil {
			service.ReportStartupError("ResourceAgent", fmt.Errorf("failed to chdir to %s: %w", basePath, err))
			fmt.Fprintf(os.Stderr, "Failed to change directory to %s: %v\n", basePath, err)
			os.Exit(1)
		}
	}

	// Detect service mode and suppress console output if no stdout available
	svcProbe := service.NewService(nil)
	if svcProbe.IsService() {
		logger.SetServiceMode(true)
	}

	// Load split configuration
	cfg, mc, lc, err := config.LoadSplit(*configPath, *monitorPath, *loggingPath)
	if err != nil {
		service.ReportStartupError("ResourceAgent", err)
		service.WriteStartupErrorFile(startupErrorLogDir, err)
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger from Logging.json
	if err := logger.Init(*lc); err != nil {
		service.ReportStartupError("ResourceAgent", err)
		service.WriteStartupErrorFile(startupErrorLogDir, err)
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

// infraResult holds the output of setupInfrastructure.
type infraResult struct {
	agentID      string
	timeDiffFunc func() int64
	syncer       *timediff.Syncer
}

// setupInfrastructure resolves Redis EQP_INFO, ServiceDiscovery, and TimeDiff.
// For sender_type="file", returns defaults (no Redis dependency).
func setupInfrastructure(ctx context.Context, cfg *config.Config, agentID string) (*infraResult, error) {
	log := logger.WithComponent("main")
	result := &infraResult{
		agentID:      agentID,
		timeDiffFunc: func() int64 { return 0 },
	}

	if strings.ToLower(cfg.SenderType) == "file" {
		return result, nil
	}

	// 1. VirtualAddressList 검증
	if cfg.VirtualAddressList == "" {
		return nil, fmt.Errorf("sender_type=%q requires VirtualAddressList", cfg.SenderType)
	}
	virtualIPs := strings.Split(cfg.VirtualAddressList, ",")
	virtualIP := strings.TrimSpace(virtualIPs[0])
	if virtualIP == "" {
		return nil, fmt.Errorf("sender_type=%q but first virtual IP is empty", cfg.SenderType)
	}

	// 2. Create SOCKS dialer if configured
	var dialFunc func(string, string) (net.Conn, error)
	if cfg.SOCKSProxy.Host != "" && cfg.SOCKSProxy.Port > 0 {
		dialFunc = network.DialerFunc(cfg.SOCKSProxy.Host, cfg.SOCKSProxy.Port)
		log.Info().
			Str("socks_host", cfg.SOCKSProxy.Host).
			Int("socks_port", cfg.SOCKSProxy.Port).
			Msg("SOCKS proxy configured")
	}

	// 3. Redis address 결정
	redisAddress := fmt.Sprintf("%s:%d", virtualIP, cfg.Redis.Port)
	log.Info().
		Str("virtual_ip", virtualIP).
		Str("redis_address", redisAddress).
		Msg("Redis address resolved from VirtualAddressList")

	// 4. 연결 기반 IP 감지
	detectedIP, dialErr := network.DetectIPByDial(redisAddress, dialFunc)
	if dialErr != nil {
		log.Warn().Err(dialErr).Msg("DetectIPByDial failed, falling back to interface-based detection")
		detectedIP = ""
	} else {
		log.Info().Str("detected_ip", detectedIP).Msg("IP detected via connection to Redis")
	}

	// 5. IP 감지
	ipInfo, ipErr := network.DetectIPs(cfg.PrivateIPAddressPattern, detectedIP)
	if ipErr != nil {
		return nil, fmt.Errorf("failed to detect IP addresses: %w", ipErr)
	}
	log.Info().
		Str("ip_addr", ipInfo.IPAddr).
		Str("ip_addr_local", ipInfo.IPAddrLocal).
		Strs("all_ips", ipInfo.AllIPs).
		Msg("IP addresses detected")

	// 6. Redis EQP_INFO 취득 (필수)
	info, fetchErr := eqpinfo.FetchEqpInfo(ctx, redisAddress, cfg.Redis, dialFunc, ipInfo.IPAddr, ipInfo.IPAddrLocal)
	if fetchErr != nil {
		return nil, fmt.Errorf("failed to fetch EQP_INFO from Redis: %w", fetchErr)
	}
	if info == nil {
		return nil, fmt.Errorf("EQP_INFO not found for %s:%s", ipInfo.IPAddr, ipInfo.IPAddrLocal)
	}

	cfg.EqpInfo = &config.EqpInfoConfig{
		Process:  info.Process,
		EqpModel: info.EqpModel,
		EqpID:    info.EqpID,
		Line:     info.Line,
		LineDesc: info.LineDesc,
		Index:    info.Index,
	}
	result.agentID = info.EqpID
	log.Info().
		Str("eqp_id", info.EqpID).
		Str("process", info.Process).
		Str("eqp_model", info.EqpModel).
		Str("line", info.Line).
		Str("line_desc", info.LineDesc).
		Str("index", info.Index).
		Msg("EQP_INFO loaded from Redis")

	// 7. ServiceDiscovery 호출 (필수)
	services, sdErr := discovery.FetchServices(ctx, virtualIP, cfg.ServiceDiscoveryPort, info.Index, dialFunc)
	if sdErr != nil {
		return nil, fmt.Errorf("ServiceDiscovery failed: %w", sdErr)
	}

	// 8. KafkaRest 주소 추출
	kafkaRestAddr, krErr := discovery.GetKafkaRestAddress(services)
	if krErr != nil {
		return nil, fmt.Errorf("failed to get KafkaRest address: %w", krErr)
	}
	cfg.KafkaRestAddress = kafkaRestAddr
	log.Info().
		Str("kafkarest_addr", kafkaRestAddr).
		Msg("KafkaRest address resolved from ServiceDiscovery")

	// 9. TimeDiff Syncer 시작
	syncer := timediff.NewSyncer(redisAddress, cfg.Redis, dialFunc, cfg.EqpInfo.EqpID, cfg.TimeDiffSyncInterval)
	if err := syncer.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start TimeDiff syncer: %w", err)
	}
	result.syncer = syncer
	result.timeDiffFunc = syncer.GetDiff

	return result, nil
}

// setupCollectors creates the collector registry and configures it from MonitorConfig.
func setupCollectors(mc *config.MonitorConfig) *collector.Registry {
	registry := collector.DefaultRegistry()
	mc.ApplyDefaults(registry.DefaultConfigs())
	return registry
}

// setupSender creates the sender and logs sender-specific information.
func setupSender(cfg *config.Config, lc *logger.Config, timeDiffFunc func() int64) (sender.Sender, error) {
	log := logger.WithComponent("main")

	// Consolidate console setting: Logging.json Console is the master switch
	cfg.File.Console = lc.Console

	snd, err := sender.NewSender(cfg, timeDiffFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to create sender: %w", err)
	}

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

	return snd, nil
}

// setupWatchers creates hot-reload watchers for Monitor.json and Logging.json.
// Returns a cleanup function that stops all started watchers.
func setupWatchers(registry *collector.Registry, sched *scheduler.Scheduler, snd sender.Sender,
	monitorPath, loggingPath string) func() {

	log := logger.WithComponent("main")
	var watcherMu sync.Mutex
	var cleanups []func()

	// Monitor.json watcher
	monitorWatcher, err := config.NewMonitorWatcher(monitorPath, func(newMC *config.MonitorConfig) {
		watcherMu.Lock()
		defer watcherMu.Unlock()

		log.Info().Msg("Applying monitor configuration changes")

		newMC.ApplyDefaults(registry.DefaultConfigs())

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
			cleanups = append(cleanups, func() {
				log.Info().Msg("Stopping monitor watcher")
				if err := monitorWatcher.Stop(); err != nil {
					log.Error().Err(err).Msg("Error stopping monitor watcher")
				}
			})
		}
	}

	// Logging.json watcher
	loggingWatcher, err := config.NewLoggingWatcher(loggingPath, func(newLC *logger.Config) {
		watcherMu.Lock()
		defer watcherMu.Unlock()

		log.Info().Msg("Applying logging configuration changes")

		if err := logger.Init(*newLC); err != nil {
			log.Error().Err(err).Msg("Failed to update logging configuration")
			return
		}

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
			cleanups = append(cleanups, func() {
				log.Info().Msg("Stopping logging watcher")
				if err := loggingWatcher.Stop(); err != nil {
					log.Error().Err(err).Msg("Error stopping logging watcher")
				}
			})
		}
	}

	return func() {
		// Stop in reverse order
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}
}

func run(ctx context.Context, cfg *config.Config, mc *config.MonitorConfig, lc *logger.Config, monitorPath, loggingPath string) error {
	log := logger.WithComponent("main")

	agentID := config.GetAgentID(cfg)
	hostname := config.GetHostname()

	log.Info().
		Str("agent_id", agentID).
		Str("hostname", hostname).
		Msg("Agent initialized")

	// Phase 1: Infrastructure (Redis, EQP_INFO, ServiceDiscovery, TimeDiff)
	infra, err := setupInfrastructure(ctx, cfg, agentID)
	if err != nil {
		return err
	}
	if infra.syncer != nil {
		defer infra.syncer.Stop()
	}

	// Phase 2: LHM Provider
	lhmProvider := collector.GetLhmProvider()
	if err := lhmProvider.Start(ctx); err != nil {
		log.Warn().Err(err).Msg("LhmHelper daemon failed to start, LHM-based collectors will return empty data")
	}
	defer lhmProvider.Stop()

	// Phase 3: Collectors
	registry := setupCollectors(mc)
	if err := registry.Configure(mc.Collectors); err != nil {
		return fmt.Errorf("failed to configure collectors: %w", err)
	}

	// Phase 4: Sender
	snd, err := setupSender(cfg, lc, infra.timeDiffFunc)
	if err != nil {
		return err
	}
	defer func() {
		log.Info().Msg("Closing sender")
		if err := snd.Close(); err != nil {
			log.Error().Err(err).Msg("Error closing sender")
		}
	}()

	// Phase 5: Scheduler
	sched := scheduler.New(registry, snd, infra.agentID, hostname)
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Phase 6: Watchers
	cleanupWatchers := setupWatchers(registry, sched, snd, monitorPath, loggingPath)
	defer cleanupWatchers()

	// Wait for context cancellation (shutdown signal)
	<-ctx.Done()
	log.Info().Msg("Received shutdown signal")

	// Stop scheduler (waits for all collectors to finish)
	sched.Stop()

	return nil
}
