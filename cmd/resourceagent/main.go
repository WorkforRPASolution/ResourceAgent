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
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/discovery"
	"resourceagent/internal/eqpinfo"
	"resourceagent/internal/heartbeat"
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

	// Validate all configurations before proceeding
	if err := config.ValidateConfig(cfg); err != nil {
		service.ReportStartupError("ResourceAgent", err)
		service.WriteStartupErrorFile(startupErrorLogDir, err)
		fmt.Fprintf(os.Stderr, "Configuration validation failed: %v\n", err)
		os.Exit(1)
	}
	if err := config.ValidateMonitorConfig(mc); err != nil {
		service.ReportStartupError("ResourceAgent", err)
		service.WriteStartupErrorFile(startupErrorLogDir, err)
		fmt.Fprintf(os.Stderr, "Monitor configuration validation failed: %v\n", err)
		os.Exit(1)
	}
	if err := config.ValidateLoggingConfig(lc); err != nil {
		service.ReportStartupError("ResourceAgent", err)
		service.WriteStartupErrorFile(startupErrorLogDir, err)
		fmt.Fprintf(os.Stderr, "Logging configuration validation failed: %v\n", err)
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
		Str("build_time", buildTime).
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
	virtualIP    string
	dialFunc     func(string, string) (net.Conn, error)
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

	result.virtualIP = virtualIP
	result.dialFunc = dialFunc

	if dialFunc != nil {
		return setupWithProxy(ctx, cfg, result, virtualIP, redisAddress, dialFunc)
	}
	return setupWithoutProxy(ctx, cfg, result, virtualIP, redisAddress)
}

// setupWithProxy implements Flow A: proxy path.
// Uses EARSInterfaceSrv to get external IP, then NIC-based pattern matching for inner IP.
func setupWithProxy(ctx context.Context, cfg *config.Config, result *infraResult,
	virtualIP, redisAddress string, dialFunc func(string, string) (net.Conn, error)) (*infraResult, error) {

	log := logger.WithComponent("main")

	// 1. ServiceDiscovery (index="0") → KafkaRest + EARSInterfaceSrv
	services, err := discovery.FetchServices(ctx, virtualIP, cfg.ServiceDiscoveryPort, "0", dialFunc)
	if err != nil {
		return nil, fmt.Errorf("ServiceDiscovery (index=0) failed: %w", err)
	}

	_, krErr := discovery.GetKafkaRestAddress(services)
	if krErr != nil {
		return nil, fmt.Errorf("failed to get KafkaRest address from initial ServiceDiscovery: %w", krErr)
	}

	earsIfAddr, eiErr := discovery.GetEARSInterfaceSrvAddress(services)
	if eiErr != nil {
		return nil, fmt.Errorf("failed to get EARSInterfaceSrv address: %w", eiErr)
	}
	log.Info().Str("ears_interface_addr", earsIfAddr).Msg("EARSInterfaceSrv address resolved")

	// 2. FetchExternalIP → 외부 IP
	externalIP, err := discovery.FetchExternalIP(ctx, earsIfAddr, dialFunc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch external IP: %w", err)
	}
	log.Info().Str("external_ip", externalIP).Msg("External IP fetched via EARSInterfaceSrv")

	// 3. DetectIPs (NIC 기반, overrideIP 없음) → MatchingIPs
	ipInfo, err := network.DetectIPs(cfg.PrivateIPAddressPattern, "")
	if err != nil {
		return nil, fmt.Errorf("failed to detect IP addresses: %w", err)
	}
	log.Info().
		Strs("all_ips", ipInfo.AllIPs).
		Strs("matching_ips", ipInfo.MatchingIPs).
		Msg("NIC IPs detected for proxy flow")

	// 4. IP 후보 생성
	candidates := buildCandidatesProxy(externalIP, ipInfo)
	log.Info().Int("candidate_count", len(candidates)).Msg("IP candidates for EQP_INFO lookup")

	// 5. FetchEqpInfoMulti
	info, matched, err := eqpinfo.FetchEqpInfoMulti(ctx, redisAddress, cfg.Redis, dialFunc, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch EQP_INFO from Redis: %w", err)
	}
	if info == nil {
		return nil, fmt.Errorf("EQP_INFO not found for any candidate (proxy flow, external=%s)", externalIP)
	}
	log.Info().
		Str("matched_ip", matched.IPAddr).
		Str("matched_local", matched.IPAddrLocal).
		Msg("EQP_INFO matched")

	applyEqpInfo(cfg, result, info)

	// 6. ServiceDiscovery (실제 index) → KafkaRest 갱신
	services, err = discovery.FetchServices(ctx, virtualIP, cfg.ServiceDiscoveryPort, info.Index, dialFunc)
	if err != nil {
		return nil, fmt.Errorf("ServiceDiscovery (index=%s) failed: %w", info.Index, err)
	}

	kafkaRestAddr, err := discovery.GetKafkaRestAddress(services)
	if err != nil {
		return nil, fmt.Errorf("failed to get KafkaRest address: %w", err)
	}
	cfg.KafkaRestAddress = kafkaRestAddr
	log.Info().Str("kafkarest_addr", kafkaRestAddr).Msg("KafkaRest address resolved")

	// 7. TimeDiff Syncer
	return startTimeDiffSyncer(ctx, cfg, result, redisAddress, dialFunc)
}

// setupWithoutProxy implements Flow B: no-proxy path.
// Uses DetectIPByDial for external IP, inner_ip="_" fixed.
func setupWithoutProxy(ctx context.Context, cfg *config.Config, result *infraResult,
	virtualIP, redisAddress string) (*infraResult, error) {

	log := logger.WithComponent("main")

	// 1. 연결 기반 IP 감지
	detectedIP, dialErr := network.DetectIPByDial(redisAddress, nil)
	if dialErr != nil {
		log.Warn().Err(dialErr).Msg("DetectIPByDial failed, falling back to all NIC IPs")
		detectedIP = ""
	} else {
		log.Info().Str("detected_ip", detectedIP).Msg("IP detected via connection to Redis")
	}

	// 2. NIC IP 목록
	ipInfo, err := network.DetectIPs("", "")
	if err != nil {
		return nil, fmt.Errorf("failed to detect IP addresses: %w", err)
	}
	log.Info().Strs("all_ips", ipInfo.AllIPs).Msg("NIC IPs detected for no-proxy flow")

	// 3. IP 후보 생성
	candidates := buildCandidatesNoProxy(detectedIP, ipInfo.AllIPs)
	log.Info().Int("candidate_count", len(candidates)).Msg("IP candidates for EQP_INFO lookup")

	// 4. FetchEqpInfoMulti
	info, matched, err := eqpinfo.FetchEqpInfoMulti(ctx, redisAddress, cfg.Redis, nil, candidates)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch EQP_INFO from Redis: %w", err)
	}
	if info == nil {
		return nil, fmt.Errorf("EQP_INFO not found for any candidate (no-proxy flow)")
	}
	log.Info().
		Str("matched_ip", matched.IPAddr).
		Str("matched_local", matched.IPAddrLocal).
		Msg("EQP_INFO matched")

	applyEqpInfo(cfg, result, info)

	// 5. ServiceDiscovery
	services, err := discovery.FetchServices(ctx, virtualIP, cfg.ServiceDiscoveryPort, info.Index, nil)
	if err != nil {
		return nil, fmt.Errorf("ServiceDiscovery failed: %w", err)
	}

	kafkaRestAddr, err := discovery.GetKafkaRestAddress(services)
	if err != nil {
		return nil, fmt.Errorf("failed to get KafkaRest address: %w", err)
	}
	cfg.KafkaRestAddress = kafkaRestAddr
	log.Info().Str("kafkarest_addr", kafkaRestAddr).Msg("KafkaRest address resolved")

	// 6. TimeDiff Syncer
	return startTimeDiffSyncer(ctx, cfg, result, redisAddress, nil)
}

// applyEqpInfo sets EqpInfo on config and result from Redis lookup.
func applyEqpInfo(cfg *config.Config, result *infraResult, info *eqpinfo.EqpInfo) {
	log := logger.WithComponent("main")
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
}

// startTimeDiffSyncer creates and starts a TimeDiff syncer.
func startTimeDiffSyncer(ctx context.Context, cfg *config.Config, result *infraResult,
	redisAddress string, dialFunc func(string, string) (net.Conn, error)) (*infraResult, error) {

	syncer := timediff.NewSyncer(redisAddress, cfg.Redis, dialFunc, cfg.EqpInfo.EqpID, cfg.TimeDiffSyncInterval)
	if err := syncer.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to start TimeDiff syncer: %w", err)
	}
	result.syncer = syncer
	result.timeDiffFunc = syncer.GetDiff
	return result, nil
}

// buildCandidatesProxy builds IP candidates for proxy flow.
// externalIP is determined by EARSInterfaceSrv. Inner IPs come from NIC pattern matching.
func buildCandidatesProxy(externalIP string, ipInfo *network.IPInfo) []eqpinfo.IPCandidate {
	if len(ipInfo.MatchingIPs) > 0 {
		candidates := make([]eqpinfo.IPCandidate, 0, len(ipInfo.MatchingIPs))
		for _, inner := range ipInfo.MatchingIPs {
			candidates = append(candidates, eqpinfo.IPCandidate{IPAddr: externalIP, IPAddrLocal: inner})
		}
		return candidates
	}
	return []eqpinfo.IPCandidate{{IPAddr: externalIP, IPAddrLocal: "_"}}
}

// buildCandidatesNoProxy builds IP candidates for no-proxy flow.
// inner_ip is always "_". If DetectIPByDial succeeded, use that single IP.
// Otherwise, try all NIC IPs as external IP.
func buildCandidatesNoProxy(detectedIP string, allIPs []string) []eqpinfo.IPCandidate {
	if detectedIP != "" {
		return []eqpinfo.IPCandidate{{IPAddr: detectedIP, IPAddrLocal: "_"}}
	}
	candidates := make([]eqpinfo.IPCandidate, 0, len(allIPs))
	for _, ip := range allIPs {
		candidates = append(candidates, eqpinfo.IPCandidate{IPAddr: ip, IPAddrLocal: "_"})
	}
	return candidates
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
		topic := config.ResolveTopic(cfg.ResourceMonitorTopic, cfg.EqpInfo)
		log.Info().
			Str("topic", topic).
			Msg("Using Kafka sender")
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

	// Phase 1.5: Heartbeat
	var hb *heartbeat.Sender
	if cfg.EqpInfo != nil {
		redisAddr := fmt.Sprintf("%s:%d", infra.virtualIP, cfg.Redis.Port)
		hb = heartbeat.NewSender(redisAddr, cfg.Redis, infra.dialFunc,
			cfg.EqpInfo.Process, cfg.EqpInfo.EqpModel, cfg.EqpInfo.EqpID)
		hb.Start(ctx)
		defer hb.Stop()
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

	// Phase 4.5: Address Refresher
	if cfg.UpdateServerAddressInterval > 0 && strings.ToLower(cfg.SenderType) != "file" {
		if kafkaSender, ok := snd.(*sender.KafkaSender); ok {
			refresher := discovery.NewRefresher(discovery.RefresherConfig{
				Interval: cfg.UpdateServerAddressInterval,
			})

			senderType := strings.ToLower(cfg.SenderType)
			switch senderType {
			case "kafkarest":
				refresher.SetTransportFactory(func(addr string) (discovery.Closeable, error) {
					return sender.NewBufferedHTTPTransport(addr, cfg.SOCKSProxy, cfg.Batch)
				})
			case "kafka":
				refresher.SetTransportFactory(func(addr string) (discovery.Closeable, error) {
					brokerAddr, err := sender.ResolveBrokerAddr(addr, cfg.Kafka.BrokerPort)
					if err != nil {
						return nil, err
					}
					return sender.NewSaramaTransport([]string{brokerAddr}, cfg.Kafka, cfg.Batch, cfg.SOCKSProxy)
				})
			}

			refresher.SetFetchAddr(func(fetchCtx context.Context) (string, error) {
				services, err := discovery.FetchServices(fetchCtx, infra.virtualIP,
					cfg.ServiceDiscoveryPort, cfg.EqpInfo.Index, infra.dialFunc)
				if err != nil {
					return "", err
				}
				return discovery.GetKafkaRestAddress(services)
			})

			refresher.SetSwapTransport(func(newT discovery.Closeable) (discovery.Closeable, error) {
				kt, ok := newT.(sender.KafkaTransport)
				if !ok {
					return nil, fmt.Errorf("invalid transport type")
				}
				return kafkaSender.SwapTransport(kt)
			})

			refresher.Start(ctx, cfg.KafkaRestAddress)
			defer refresher.Stop()

			log.Info().
				Dur("interval", cfg.UpdateServerAddressInterval).
				Msg("Address refresher started")
		}
	}

	// Phase 5: Scheduler
	sched := scheduler.New(registry, snd, infra.agentID, hostname)
	if err := sched.Start(ctx); err != nil {
		return fmt.Errorf("failed to start scheduler: %w", err)
	}

	// Connect heartbeat watchdog to scheduler activity
	if hb != nil {
		hb.SetHealthCheck(func() (string, string) {
			last := sched.LastActivity()
			if last.IsZero() {
				return "OK", "" // 아직 첫 수집 전
			}
			if time.Since(last) > heartbeat.StalenessThreshold {
				return "WARN", "no_collection"
			}
			return "OK", ""
		})
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
