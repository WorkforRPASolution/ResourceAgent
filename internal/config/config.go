// Package config provides configuration management for the ResourceAgent.
package config

import (
	"fmt"
	"time"
)

// Config is the root configuration structure.
type Config struct {
	SenderType string                     `json:"SenderType"` // "kafka", "kafkarest", or "file"
	Kafka      KafkaConfig                `json:"Kafka"`
	File       FileConfig                 `json:"File"`
	VirtualAddressList      string                     `json:"VirtualAddressList"`
	Redis                   RedisConfig                `json:"Redis"`
	PrivateIPAddressPattern string                     `json:"PrivateIPAddressPattern"`
	SOCKSProxy              SOCKSConfig                `json:"SocksProxy"`
	ServiceDiscoveryPort    int                        `json:"ServiceDiscoveryPort"`
	ResourceMonitorTopic    string                     `json:"ResourceMonitorTopic"`
	TimeDiffSyncInterval    int                        `json:"TimeDiffSyncInterval"` // seconds, default 3600
	KafkaRestAddress        string                     `json:"-"` // runtime only, from ServiceDiscovery
	EqpInfo                 *EqpInfoConfig             `json:"-"` // runtime only, not serialized
}

// FileConfig contains settings for the file sender.
type FileConfig struct {
	FilePath   string `json:"FilePath"`
	MaxSizeMB  int    `json:"MaxSizeMB"`
	MaxBackups int    `json:"MaxBackups"`
	Console    bool   `json:"Console"`
	Pretty     bool   `json:"Pretty"`
	Format     string `json:"Format"` // Output format: "json" or "legacy" (default: "legacy")
}

// KafkaConfig contains Kafka connection settings.
type KafkaConfig struct {
	Brokers        []string      `json:"Brokers"`
	Topic          string        `json:"Topic"`
	Compression    string        `json:"Compression"`
	RequiredAcks   int           `json:"RequiredAcks"`
	MaxRetries     int           `json:"MaxRetries"`
	RetryBackoff   time.Duration `json:"RetryBackoff"`
	FlushFrequency time.Duration `json:"FlushFrequency"`
	FlushMessages  int           `json:"FlushMessages"`
	BatchSize      int           `json:"BatchSize"`
	Timeout        time.Duration `json:"Timeout"`
	EnableTLS      bool          `json:"EnableTLS"`
	TLSCertFile    string        `json:"TLSCertFile"`
	TLSKeyFile     string        `json:"TLSKeyFile"`
	TLSCAFile      string        `json:"TLSCAFile"`
	SASLEnabled    bool          `json:"SASLEnabled"`
	SASLMechanism  string        `json:"SASLMechanism"`
	SASLUser       string        `json:"SASLUser"`
	SASLPassword   string        `json:"SASLPassword"`
}

// CollectorConfig contains settings for individual collectors.
type CollectorConfig struct {
	Enabled        bool          `json:"Enabled"`
	Interval       time.Duration `json:"Interval"`
	TopN           int           `json:"TopN,omitempty"`
	Disks          []string      `json:"Disks,omitempty"`
	Interfaces     []string      `json:"Interfaces,omitempty"`
	IncludeZones   []string      `json:"IncludeZones,omitempty"`
	WatchProcesses     []string `json:"WatchProcesses,omitempty"`
	RequiredProcesses  []string `json:"RequiredProcesses,omitempty"`
	ForbiddenProcesses []string `json:"ForbiddenProcesses,omitempty"`
}

// DefaultRedisPassword is used when Password is empty in config.
const DefaultRedisPassword = "visuallove"

// RedisConfig contains Redis connection settings.
type RedisConfig struct {
	Port     int    `json:"Port"`
	Password string `json:"Password"`
	DB       int    `json:"DB"`
}

// ResolvePassword returns the configured password, or DefaultRedisPassword if empty.
func (r RedisConfig) ResolvePassword() string {
	if r.Password == "" {
		return DefaultRedisPassword
	}
	return r.Password
}

// SOCKSConfig contains SOCKS5 proxy settings.
type SOCKSConfig struct {
	Host string `json:"Host"`
	Port int    `json:"Port"`
}

// EqpInfoConfig contains equipment information from Redis (runtime only).
type EqpInfoConfig struct {
	Process  string
	EqpModel string
	EqpID    string
	Line     string
	LineDesc string
	Index    string
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		SenderType: "kafka", // default for backward compatibility
		File: FileConfig{
			FilePath:   "log/ResourceAgent/metrics.jsonl",
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
		Redis: RedisConfig{
			Port: 6379,
			DB:   10,
		},
		ServiceDiscoveryPort:    50009,
		ResourceMonitorTopic:    "process",
		TimeDiffSyncInterval:    3600,
	}
}

// Merge applies non-zero values from other to this config.
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
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
	if other.File.Format != "" {
		c.File.Format = other.File.Format
	}

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

	// Merge VirtualAddressList
	if other.VirtualAddressList != "" {
		c.VirtualAddressList = other.VirtualAddressList
	}

	// Merge Redis config
	if other.Redis.Port != 0 {
		c.Redis.Port = other.Redis.Port
	}
	if other.Redis.Password != "" {
		c.Redis.Password = other.Redis.Password
	}
	if other.Redis.DB != 0 {
		c.Redis.DB = other.Redis.DB
	}

	// Merge SOCKS proxy config
	if other.SOCKSProxy.Host != "" {
		c.SOCKSProxy.Host = other.SOCKSProxy.Host
	}
	if other.SOCKSProxy.Port != 0 {
		c.SOCKSProxy.Port = other.SOCKSProxy.Port
	}

	// Merge PrivateIPAddressPattern
	if other.PrivateIPAddressPattern != "" {
		c.PrivateIPAddressPattern = other.PrivateIPAddressPattern
	}

	// Merge ServiceDiscovery config
	if other.ServiceDiscoveryPort != 0 {
		c.ServiceDiscoveryPort = other.ServiceDiscoveryPort
	}
	if other.ResourceMonitorTopic != "" {
		c.ResourceMonitorTopic = other.ResourceMonitorTopic
	}
	if other.TimeDiffSyncInterval != 0 {
		c.TimeDiffSyncInterval = other.TimeDiffSyncInterval
	}
}

// MonitorConfig holds collectors-only configuration (for Monitor.json).
type MonitorConfig struct {
	Collectors map[string]CollectorConfig `json:"Collectors"`
}

// DefaultMonitorConfig returns a MonitorConfig with empty defaults.
// Use ApplyDefaults() with registry-provided defaults for full initialization.
func DefaultMonitorConfig() *MonitorConfig {
	return &MonitorConfig{
		Collectors: make(map[string]CollectorConfig),
	}
}

// ApplyDefaults fills in missing collector entries from the provided defaults.
// Existing entries are not overwritten.
func (mc *MonitorConfig) ApplyDefaults(defaults map[string]CollectorConfig) {
	for name, defCfg := range defaults {
		if _, exists := mc.Collectors[name]; !exists {
			mc.Collectors[name] = defCfg
		}
	}
}

// Merge applies non-zero values from other to this MonitorConfig.
func (mc *MonitorConfig) Merge(other *MonitorConfig) {
	if other == nil {
		return
	}
	for name, collectorCfg := range other.Collectors {
		if existing, ok := mc.Collectors[name]; ok {
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
			if len(collectorCfg.RequiredProcesses) > 0 {
				existing.RequiredProcesses = collectorCfg.RequiredProcesses
			}
			if len(collectorCfg.ForbiddenProcesses) > 0 {
				existing.ForbiddenProcesses = collectorCfg.ForbiddenProcesses
			}
			mc.Collectors[name] = existing
		} else {
			mc.Collectors[name] = collectorCfg
		}
	}
}

// ResolveTopic determines the Kafka topic name based on the topic mode and EQP_INFO.
func ResolveTopic(mode string, eqpInfo *EqpInfoConfig) string {
	switch mode {
	case "all":
		return "tp_all_all_resource"
	case "model":
		return fmt.Sprintf("tp_%s_%s_resource", eqpInfo.Process, eqpInfo.EqpModel)
	default: // "process" or any other value
		return fmt.Sprintf("tp_%s_all_resource", eqpInfo.Process)
	}
}
