package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"resourceagent/internal/logger"
)

// rawConfig is used for JSON unmarshaling with duration strings.
type rawConfig struct {
	Agent      AgentConfig    `json:"Agent"`
	SenderType string         `json:"SenderType"`
	File       FileConfig     `json:"File"`
	Kafka      rawKafkaConfig `json:"Kafka"`
	VirtualAddressList      string                        `json:"VirtualAddressList"`
	Redis                   RedisConfig                   `json:"Redis"`
	PrivateIPAddressPattern string                        `json:"PrivateIPAddressPattern"`
	SOCKSProxy              SOCKSConfig                   `json:"SocksProxy"`
	ServiceDiscoveryPort    int                           `json:"ServiceDiscoveryPort"`
	ResourceMonitorTopic    string                        `json:"ResourceMonitorTopic"`
}

type rawKafkaConfig struct {
	Brokers        []string `json:"Brokers"`
	Topic          string   `json:"Topic"`
	Compression    string   `json:"Compression"`
	RequiredAcks   int      `json:"RequiredAcks"`
	MaxRetries     int      `json:"MaxRetries"`
	RetryBackoff   string   `json:"RetryBackoff"`
	FlushFrequency string   `json:"FlushFrequency"`
	FlushMessages  int      `json:"FlushMessages"`
	BatchSize      int      `json:"BatchSize"`
	Timeout        string   `json:"Timeout"`
	EnableTLS      bool     `json:"EnableTLS"`
	TLSCertFile    string   `json:"TLSCertFile"`
	TLSKeyFile     string   `json:"TLSKeyFile"`
	TLSCAFile      string   `json:"TLSCAFile"`
	SASLEnabled    bool     `json:"SASLEnabled"`
	SASLMechanism  string   `json:"SASLMechanism"`
	SASLUser       string   `json:"SASLUser"`
	SASLPassword   string   `json:"SASLPassword"`
}

type rawCollectorConfig struct {
	Enabled            bool     `json:"Enabled"`
	Interval           string   `json:"Interval"`
	TopN               int      `json:"TopN,omitempty"`
	Disks              []string `json:"Disks,omitempty"`
	Interfaces         []string `json:"Interfaces,omitempty"`
	IncludeZones       []string `json:"IncludeZones,omitempty"`
	WatchProcesses     []string `json:"WatchProcesses,omitempty"`
	RequiredProcesses  []string `json:"RequiredProcesses,omitempty"`
	ForbiddenProcesses []string `json:"ForbiddenProcesses,omitempty"`
}

type rawLoggingConfig struct {
	Level      string `json:"Level"`
	FilePath   string `json:"FilePath"`
	MaxSizeMB  int    `json:"MaxSizeMB"`
	MaxBackups int    `json:"MaxBackups"`
	MaxAgeDays int    `json:"MaxAgeDays"`
	Compress   bool   `json:"Compress"`
	Console    bool   `json:"Console"`
}

// Load reads configuration from the specified file path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	return Parse(data)
}

// Parse parses configuration from JSON bytes.
func Parse(data []byte) (*Config, error) {
	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}

	cfg := DefaultConfig()
	parsed, err := convertRawConfig(&raw)
	if err != nil {
		return nil, err
	}

	cfg.Merge(parsed)
	return cfg, nil
}

func convertRawConfig(raw *rawConfig) (*Config, error) {
	cfg := &Config{
		Agent:      raw.Agent,
		SenderType: raw.SenderType,
		File:       raw.File,
	}

	// Convert Kafka config
	kafka, err := convertRawKafka(&raw.Kafka)
	if err != nil {
		return nil, err
	}
	cfg.Kafka = *kafka

	// Direct-mapped fields (no duration conversion needed)
	cfg.VirtualAddressList = raw.VirtualAddressList
	cfg.Redis = raw.Redis
	cfg.PrivateIPAddressPattern = raw.PrivateIPAddressPattern
	cfg.SOCKSProxy = raw.SOCKSProxy
	cfg.ServiceDiscoveryPort = raw.ServiceDiscoveryPort
	cfg.ResourceMonitorTopic = raw.ResourceMonitorTopic

	return cfg, nil
}

func convertRawKafka(raw *rawKafkaConfig) (*KafkaConfig, error) {
	kafka := &KafkaConfig{
		Brokers:       raw.Brokers,
		Topic:         raw.Topic,
		Compression:   raw.Compression,
		RequiredAcks:  raw.RequiredAcks,
		MaxRetries:    raw.MaxRetries,
		FlushMessages: raw.FlushMessages,
		BatchSize:     raw.BatchSize,
		EnableTLS:     raw.EnableTLS,
		TLSCertFile:   raw.TLSCertFile,
		TLSKeyFile:    raw.TLSKeyFile,
		TLSCAFile:     raw.TLSCAFile,
		SASLEnabled:   raw.SASLEnabled,
		SASLMechanism: raw.SASLMechanism,
		SASLUser:      raw.SASLUser,
		SASLPassword:  raw.SASLPassword,
	}

	if raw.RetryBackoff != "" {
		d, err := time.ParseDuration(raw.RetryBackoff)
		if err != nil {
			return nil, fmt.Errorf("invalid RetryBackoff duration: %w", err)
		}
		kafka.RetryBackoff = d
	}

	if raw.FlushFrequency != "" {
		d, err := time.ParseDuration(raw.FlushFrequency)
		if err != nil {
			return nil, fmt.Errorf("invalid FlushFrequency duration: %w", err)
		}
		kafka.FlushFrequency = d
	}

	if raw.Timeout != "" {
		d, err := time.ParseDuration(raw.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid Timeout duration: %w", err)
		}
		kafka.Timeout = d
	}

	return kafka, nil
}

func convertRawCollector(name string, raw *rawCollectorConfig) (*CollectorConfig, error) {
	coll := &CollectorConfig{
		Enabled:            raw.Enabled,
		TopN:               raw.TopN,
		Disks:              raw.Disks,
		Interfaces:         raw.Interfaces,
		IncludeZones:       raw.IncludeZones,
		WatchProcesses:     raw.WatchProcesses,
		RequiredProcesses:  raw.RequiredProcesses,
		ForbiddenProcesses: raw.ForbiddenProcesses,
	}

	if raw.Interval != "" {
		d, err := time.ParseDuration(raw.Interval)
		if err != nil {
			return nil, fmt.Errorf("invalid interval for collector %s: %w", name, err)
		}
		coll.Interval = d
	}

	return coll, nil
}

func convertRawLogging(raw *rawLoggingConfig) logger.Config {
	return logger.Config{
		Level:      raw.Level,
		FilePath:   raw.FilePath,
		MaxSizeMB:  raw.MaxSizeMB,
		MaxBackups: raw.MaxBackups,
		MaxAgeDays: raw.MaxAgeDays,
		Compress:   raw.Compress,
		Console:    raw.Console,
	}
}

// rawMonitorConfig is used for JSON unmarshaling of Monitor.json.
type rawMonitorConfig struct {
	Collectors map[string]rawCollectorConfig `json:"Collectors"`
}

// LoadMonitor reads monitor configuration from the specified file path.
func LoadMonitor(path string) (*MonitorConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read monitor config file: %w", err)
	}
	return ParseMonitor(data)
}

// ParseMonitor parses monitor configuration from JSON bytes.
func ParseMonitor(data []byte) (*MonitorConfig, error) {
	var raw rawMonitorConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse monitor config JSON: %w", err)
	}

	mc := DefaultMonitorConfig()

	if len(raw.Collectors) > 0 {
		parsed := &MonitorConfig{
			Collectors: make(map[string]CollectorConfig),
		}
		for name, rawColl := range raw.Collectors {
			coll, err := convertRawCollector(name, &rawColl)
			if err != nil {
				return nil, err
			}
			parsed.Collectors[name] = *coll
		}
		mc.Merge(parsed)
	}

	return mc, nil
}

// LoadLogging reads logging configuration from the specified file path.
func LoadLogging(path string) (*logger.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read logging config file: %w", err)
	}
	return ParseLogging(data)
}

// ParseLogging parses logging configuration from JSON bytes.
func ParseLogging(data []byte) (*logger.Config, error) {
	var raw rawLoggingConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse logging config JSON: %w", err)
	}

	def := logger.DefaultConfig()
	parsed := convertRawLogging(&raw)

	// Merge: apply non-zero parsed values over defaults
	if parsed.Level != "" {
		def.Level = parsed.Level
	}
	if parsed.FilePath != "" {
		def.FilePath = parsed.FilePath
	}
	if parsed.MaxSizeMB != 0 {
		def.MaxSizeMB = parsed.MaxSizeMB
	}
	if parsed.MaxBackups != 0 {
		def.MaxBackups = parsed.MaxBackups
	}
	if parsed.MaxAgeDays != 0 {
		def.MaxAgeDays = parsed.MaxAgeDays
	}
	def.Compress = parsed.Compress
	def.Console = parsed.Console

	return &def, nil
}

// LoadSplit loads configuration from three separate files:
// configPath (ResourceAgent.json), monitorPath (Monitor.json), loggingPath (Logging.json).
func LoadSplit(configPath, monitorPath, loggingPath string) (*Config, *MonitorConfig, *logger.Config, error) {
	cfg, err := Load(configPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load config: %w", err)
	}

	mc, err := LoadMonitor(monitorPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load monitor config: %w", err)
	}

	lc, err := LoadLogging(loggingPath)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load logging config: %w", err)
	}

	return cfg, mc, lc, nil
}

// GetHostname returns the configured hostname or the system hostname.
func GetHostname(cfg *Config) string {
	if cfg.Agent.Hostname != "" {
		return cfg.Agent.Hostname
	}
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

// GetAgentID returns the agent ID with priority: EqpInfo.EqpID > Agent.ID > hostname.
func GetAgentID(cfg *Config) string {
	if cfg.EqpInfo != nil && cfg.EqpInfo.EqpID != "" {
		return cfg.EqpInfo.EqpID
	}
	if cfg.Agent.ID != "" {
		return cfg.Agent.ID
	}
	return GetHostname(cfg)
}
