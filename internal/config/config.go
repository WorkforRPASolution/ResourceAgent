// Package config provides configuration management for the ResourceAgent.
package config

import (
	"time"

	"resourceagent/internal/logger"
)

// Config is the root configuration structure.
type Config struct {
	Agent      AgentConfig                `json:"agent"`
	SenderType string                     `json:"sender_type"` // "kafka" or "file"
	Kafka      KafkaConfig                `json:"kafka"`
	File       FileConfig                 `json:"file"`
	Collectors map[string]CollectorConfig `json:"collectors"`
	Logging    logger.Config              `json:"logging"`
}

// FileConfig contains settings for the file sender.
type FileConfig struct {
	FilePath   string `json:"file_path"`   // Path to the metrics log file
	MaxSizeMB  int    `json:"max_size_mb"` // Maximum file size in MB before rotation
	MaxBackups int    `json:"max_backups"` // Number of backup files to keep
	Console    bool   `json:"console"`     // Also output to console
	Pretty     bool   `json:"pretty"`      // Pretty print JSON
}

// AgentConfig contains general agent settings.
type AgentConfig struct {
	ID       string            `json:"id"`
	Hostname string            `json:"hostname"`
	Tags     map[string]string `json:"tags"`
}

// KafkaConfig contains Kafka connection settings.
type KafkaConfig struct {
	Brokers        []string      `json:"brokers"`
	Topic          string        `json:"topic"`
	Compression    string        `json:"compression"`
	RequiredAcks   int           `json:"required_acks"`
	MaxRetries     int           `json:"max_retries"`
	RetryBackoff   time.Duration `json:"retry_backoff"`
	FlushFrequency time.Duration `json:"flush_frequency"`
	FlushMessages  int           `json:"flush_messages"`
	BatchSize      int           `json:"batch_size"`
	Timeout        time.Duration `json:"timeout"`
	EnableTLS      bool          `json:"enable_tls"`
	TLSCertFile    string        `json:"tls_cert_file"`
	TLSKeyFile     string        `json:"tls_key_file"`
	TLSCAFile      string        `json:"tls_ca_file"`
	SASLEnabled    bool          `json:"sasl_enabled"`
	SASLMechanism  string        `json:"sasl_mechanism"`
	SASLUser       string        `json:"sasl_user"`
	SASLPassword   string        `json:"sasl_password"`
}

// CollectorConfig contains settings for individual collectors.
type CollectorConfig struct {
	Enabled        bool          `json:"enabled"`
	Interval       time.Duration `json:"interval"`
	TopN           int           `json:"top_n,omitempty"`
	Disks          []string      `json:"disks,omitempty"`
	Interfaces     []string      `json:"interfaces,omitempty"`
	IncludeZones   []string      `json:"include_zones,omitempty"`
	WatchProcesses []string      `json:"watch_processes,omitempty"`
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Agent: AgentConfig{
			ID:   "",
			Tags: make(map[string]string),
		},
		SenderType: "kafka", // default for backward compatibility
		File: FileConfig{
			FilePath:   "logs/metrics.jsonl",
			MaxSizeMB:  50,
			MaxBackups: 3,
			Console:    true,
			Pretty:     false,
		},
		Kafka: KafkaConfig{
			Brokers:        []string{"localhost:9092"},
			Topic:          "factory-metrics",
			Compression:    "snappy",
			RequiredAcks:   1,
			MaxRetries:     3,
			RetryBackoff:   100 * time.Millisecond,
			FlushFrequency: 500 * time.Millisecond,
			FlushMessages:  100,
			BatchSize:      16384,
			Timeout:        10 * time.Second,
		},
		Collectors: map[string]CollectorConfig{
			"cpu": {
				Enabled:  true,
				Interval: 10 * time.Second,
			},
			"memory": {
				Enabled:  true,
				Interval: 10 * time.Second,
			},
			"disk": {
				Enabled:  true,
				Interval: 30 * time.Second,
			},
			"network": {
				Enabled:  true,
				Interval: 10 * time.Second,
			},
			"temperature": {
				Enabled:  true,
				Interval: 30 * time.Second,
			},
			"cpu_process": {
				Enabled:  true,
				Interval: 30 * time.Second,
				TopN:     10,
			},
			"memory_process": {
				Enabled:  true,
				Interval: 30 * time.Second,
				TopN:     10,
			},
		},
		Logging: logger.DefaultConfig(),
	}
}

// Merge applies non-zero values from other to this config.
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
	}

	// Merge Agent config
	if other.Agent.ID != "" {
		c.Agent.ID = other.Agent.ID
	}
	if other.Agent.Hostname != "" {
		c.Agent.Hostname = other.Agent.Hostname
	}
	for k, v := range other.Agent.Tags {
		c.Agent.Tags[k] = v
	}

	// Merge SenderType
	if other.SenderType != "" {
		c.SenderType = other.SenderType
	}

	// Merge File config
	if other.File.FilePath != "" {
		c.File.FilePath = other.File.FilePath
	}
	if other.File.MaxSizeMB != 0 {
		c.File.MaxSizeMB = other.File.MaxSizeMB
	}
	if other.File.MaxBackups != 0 {
		c.File.MaxBackups = other.File.MaxBackups
	}
	c.File.Console = other.File.Console
	c.File.Pretty = other.File.Pretty

	// Merge Kafka config
	if len(other.Kafka.Brokers) > 0 {
		c.Kafka.Brokers = other.Kafka.Brokers
	}
	if other.Kafka.Topic != "" {
		c.Kafka.Topic = other.Kafka.Topic
	}
	if other.Kafka.Compression != "" {
		c.Kafka.Compression = other.Kafka.Compression
	}
	if other.Kafka.RequiredAcks != 0 {
		c.Kafka.RequiredAcks = other.Kafka.RequiredAcks
	}
	if other.Kafka.MaxRetries != 0 {
		c.Kafka.MaxRetries = other.Kafka.MaxRetries
	}
	if other.Kafka.RetryBackoff != 0 {
		c.Kafka.RetryBackoff = other.Kafka.RetryBackoff
	}
	if other.Kafka.FlushFrequency != 0 {
		c.Kafka.FlushFrequency = other.Kafka.FlushFrequency
	}
	if other.Kafka.FlushMessages != 0 {
		c.Kafka.FlushMessages = other.Kafka.FlushMessages
	}
	if other.Kafka.BatchSize != 0 {
		c.Kafka.BatchSize = other.Kafka.BatchSize
	}
	if other.Kafka.Timeout != 0 {
		c.Kafka.Timeout = other.Kafka.Timeout
	}
	c.Kafka.EnableTLS = other.Kafka.EnableTLS
	if other.Kafka.TLSCertFile != "" {
		c.Kafka.TLSCertFile = other.Kafka.TLSCertFile
	}
	if other.Kafka.TLSKeyFile != "" {
		c.Kafka.TLSKeyFile = other.Kafka.TLSKeyFile
	}
	if other.Kafka.TLSCAFile != "" {
		c.Kafka.TLSCAFile = other.Kafka.TLSCAFile
	}
	c.Kafka.SASLEnabled = other.Kafka.SASLEnabled
	if other.Kafka.SASLMechanism != "" {
		c.Kafka.SASLMechanism = other.Kafka.SASLMechanism
	}
	if other.Kafka.SASLUser != "" {
		c.Kafka.SASLUser = other.Kafka.SASLUser
	}
	if other.Kafka.SASLPassword != "" {
		c.Kafka.SASLPassword = other.Kafka.SASLPassword
	}

	// Merge Collector configs
	for name, collectorCfg := range other.Collectors {
		if existing, ok := c.Collectors[name]; ok {
			existing.Enabled = collectorCfg.Enabled
			if collectorCfg.Interval != 0 {
				existing.Interval = collectorCfg.Interval
			}
			if collectorCfg.TopN != 0 {
				existing.TopN = collectorCfg.TopN
			}
			if len(collectorCfg.Disks) > 0 {
				existing.Disks = collectorCfg.Disks
			}
			if len(collectorCfg.Interfaces) > 0 {
				existing.Interfaces = collectorCfg.Interfaces
			}
			if len(collectorCfg.IncludeZones) > 0 {
				existing.IncludeZones = collectorCfg.IncludeZones
			}
			if len(collectorCfg.WatchProcesses) > 0 {
				existing.WatchProcesses = collectorCfg.WatchProcesses
			}
			c.Collectors[name] = existing
		} else {
			c.Collectors[name] = collectorCfg
		}
	}

	// Merge Logging config
	if other.Logging.Level != "" {
		c.Logging.Level = other.Logging.Level
	}
	if other.Logging.FilePath != "" {
		c.Logging.FilePath = other.Logging.FilePath
	}
	if other.Logging.MaxSizeMB != 0 {
		c.Logging.MaxSizeMB = other.Logging.MaxSizeMB
	}
	if other.Logging.MaxBackups != 0 {
		c.Logging.MaxBackups = other.Logging.MaxBackups
	}
	if other.Logging.MaxAgeDays != 0 {
		c.Logging.MaxAgeDays = other.Logging.MaxAgeDays
	}
	c.Logging.Compress = other.Logging.Compress
	c.Logging.Console = other.Logging.Console
}
