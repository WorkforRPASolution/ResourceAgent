package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// rawConfig is used for JSON unmarshaling with duration strings.
type rawConfig struct {
	Agent      AgentConfig                   `json:"agent"`
	SenderType string                        `json:"sender_type"`
	File       FileConfig                    `json:"file"`
	Kafka      rawKafkaConfig                `json:"kafka"`
	Collectors map[string]rawCollectorConfig `json:"collectors"`
	Logging    rawLoggingConfig              `json:"logging"`
}

type rawKafkaConfig struct {
	Brokers        []string `json:"brokers"`
	Topic          string   `json:"topic"`
	Compression    string   `json:"compression"`
	RequiredAcks   int      `json:"required_acks"`
	MaxRetries     int      `json:"max_retries"`
	RetryBackoff   string   `json:"retry_backoff"`
	FlushFrequency string   `json:"flush_frequency"`
	FlushMessages  int      `json:"flush_messages"`
	BatchSize      int      `json:"batch_size"`
	Timeout        string   `json:"timeout"`
	EnableTLS      bool     `json:"enable_tls"`
	TLSCertFile    string   `json:"tls_cert_file"`
	TLSKeyFile     string   `json:"tls_key_file"`
	TLSCAFile      string   `json:"tls_ca_file"`
	SASLEnabled    bool     `json:"sasl_enabled"`
	SASLMechanism  string   `json:"sasl_mechanism"`
	SASLUser       string   `json:"sasl_user"`
	SASLPassword   string   `json:"sasl_password"`
}

type rawCollectorConfig struct {
	Enabled        bool     `json:"enabled"`
	Interval       string   `json:"interval"`
	TopN           int      `json:"top_n,omitempty"`
	Disks          []string `json:"disks,omitempty"`
	Interfaces     []string `json:"interfaces,omitempty"`
	IncludeZones   []string `json:"include_zones,omitempty"`
	WatchProcesses []string `json:"watch_processes,omitempty"`
}

type rawLoggingConfig struct {
	Level      string `json:"level"`
	FilePath   string `json:"file_path"`
	MaxSizeMB  int    `json:"max_size_mb"`
	MaxBackups int    `json:"max_backups"`
	MaxAgeDays int    `json:"max_age_days"`
	Compress   bool   `json:"compress"`
	Console    bool   `json:"console"`
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
		Collectors: make(map[string]CollectorConfig),
	}

	// Convert Kafka config
	kafka := KafkaConfig{
		Brokers:       raw.Kafka.Brokers,
		Topic:         raw.Kafka.Topic,
		Compression:   raw.Kafka.Compression,
		RequiredAcks:  raw.Kafka.RequiredAcks,
		MaxRetries:    raw.Kafka.MaxRetries,
		FlushMessages: raw.Kafka.FlushMessages,
		BatchSize:     raw.Kafka.BatchSize,
		EnableTLS:     raw.Kafka.EnableTLS,
		TLSCertFile:   raw.Kafka.TLSCertFile,
		TLSKeyFile:    raw.Kafka.TLSKeyFile,
		TLSCAFile:     raw.Kafka.TLSCAFile,
		SASLEnabled:   raw.Kafka.SASLEnabled,
		SASLMechanism: raw.Kafka.SASLMechanism,
		SASLUser:      raw.Kafka.SASLUser,
		SASLPassword:  raw.Kafka.SASLPassword,
	}

	if raw.Kafka.RetryBackoff != "" {
		d, err := time.ParseDuration(raw.Kafka.RetryBackoff)
		if err != nil {
			return nil, fmt.Errorf("invalid retry_backoff duration: %w", err)
		}
		kafka.RetryBackoff = d
	}

	if raw.Kafka.FlushFrequency != "" {
		d, err := time.ParseDuration(raw.Kafka.FlushFrequency)
		if err != nil {
			return nil, fmt.Errorf("invalid flush_frequency duration: %w", err)
		}
		kafka.FlushFrequency = d
	}

	if raw.Kafka.Timeout != "" {
		d, err := time.ParseDuration(raw.Kafka.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid timeout duration: %w", err)
		}
		kafka.Timeout = d
	}

	cfg.Kafka = kafka

	// Convert Collector configs
	for name, rawColl := range raw.Collectors {
		coll := CollectorConfig{
			Enabled:        rawColl.Enabled,
			TopN:           rawColl.TopN,
			Disks:          rawColl.Disks,
			Interfaces:     rawColl.Interfaces,
			IncludeZones:   rawColl.IncludeZones,
			WatchProcesses: rawColl.WatchProcesses,
		}

		if rawColl.Interval != "" {
			d, err := time.ParseDuration(rawColl.Interval)
			if err != nil {
				return nil, fmt.Errorf("invalid interval for collector %s: %w", name, err)
			}
			coll.Interval = d
		}

		cfg.Collectors[name] = coll
	}

	// Convert Logging config
	cfg.Logging.Level = raw.Logging.Level
	cfg.Logging.FilePath = raw.Logging.FilePath
	cfg.Logging.MaxSizeMB = raw.Logging.MaxSizeMB
	cfg.Logging.MaxBackups = raw.Logging.MaxBackups
	cfg.Logging.MaxAgeDays = raw.Logging.MaxAgeDays
	cfg.Logging.Compress = raw.Logging.Compress
	cfg.Logging.Console = raw.Logging.Console

	return cfg, nil
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

// GetAgentID returns the configured agent ID or generates one from hostname.
func GetAgentID(cfg *Config) string {
	if cfg.Agent.ID != "" {
		return cfg.Agent.ID
	}
	return GetHostname(cfg)
}
