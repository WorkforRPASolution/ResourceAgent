package config

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"resourceagent/internal/logger"
)

// ValidationError represents a single configuration validation error.
type ValidationError struct {
	Field   string // e.g. "Kafka.BrokerPort"
	Value   string // string representation of the actual value
	Message string // human-readable description
}

// ValidationErrors is a slice of ValidationError that implements the error interface.
type ValidationErrors []ValidationError

func (ve ValidationErrors) Error() string {
	n := len(ve)
	noun := "error"
	if n != 1 {
		noun = "errors"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "config validation failed (%d %s):", n, noun)
	for i, e := range ve {
		fmt.Fprintf(&b, "\n  [%d] %s=%q: %s", i+1, e.Field, e.Value, e.Message)
	}
	return b.String()
}

// ValidateConfig validates the main ResourceAgent.json configuration.
func ValidateConfig(cfg *Config) error {
	var errs ValidationErrors

	// --- P0: required ---

	// SenderType
	senderType := strings.ToLower(cfg.SenderType)
	switch senderType {
	case "kafka", "kafkarest", "file":
		// ok
	default:
		errs = append(errs, ValidationError{
			Field:   "SenderType",
			Value:   cfg.SenderType,
			Message: "must be one of: kafka, kafkarest, file",
		})
	}

	// VirtualAddressList required for non-file senders
	if senderType != "file" && cfg.VirtualAddressList == "" {
		errs = append(errs, ValidationError{
			Field:   "VirtualAddressList",
			Value:   "",
			Message: fmt.Sprintf("required when SenderType=%q", cfg.SenderType),
		})
	}

	// Port ranges
	validatePort(&errs, "Kafka.BrokerPort", cfg.Kafka.BrokerPort)
	validatePort(&errs, "Redis.Port", cfg.Redis.Port)
	validatePort(&errs, "ServiceDiscoveryPort", cfg.ServiceDiscoveryPort)

	// SOCKSProxy.Port — only if configured (>0 or Host set)
	if cfg.SOCKSProxy.Port > 0 {
		if cfg.SOCKSProxy.Port > 65535 {
			errs = append(errs, ValidationError{
				Field:   "SOCKSProxy.Port",
				Value:   fmt.Sprintf("%d", cfg.SOCKSProxy.Port),
				Message: "port must be between 1 and 65535",
			})
		}
	}

	// File.Format
	switch strings.ToLower(cfg.File.Format) {
	case "", "json", "grok", "legacy":
		// ok
	default:
		errs = append(errs, ValidationError{
			Field:   "File.Format",
			Value:   cfg.File.Format,
			Message: `must be one of: "", "json", "grok", "legacy"`,
		})
	}

	// --- P1: important ---

	// Kafka.Compression
	switch strings.ToLower(cfg.Kafka.Compression) {
	case "", "none", "snappy", "gzip", "lz4", "zstd":
		// ok
	default:
		errs = append(errs, ValidationError{
			Field:   "Kafka.Compression",
			Value:   cfg.Kafka.Compression,
			Message: `must be one of: "", "none", "snappy", "gzip", "lz4", "zstd"`,
		})
	}

	// Kafka.RequiredAcks
	switch cfg.Kafka.RequiredAcks {
	case 0, 1, -1:
		// ok
	default:
		errs = append(errs, ValidationError{
			Field:   "Kafka.RequiredAcks",
			Value:   fmt.Sprintf("%d", cfg.Kafka.RequiredAcks),
			Message: "must be one of: 0, 1, -1",
		})
	}

	// Batch fields
	if cfg.Batch.FlushMessages <= 0 {
		errs = append(errs, ValidationError{
			Field:   "Batch.FlushMessages",
			Value:   fmt.Sprintf("%d", cfg.Batch.FlushMessages),
			Message: "must be > 0",
		})
	}
	if cfg.Batch.MaxBatchSize <= 0 {
		errs = append(errs, ValidationError{
			Field:   "Batch.MaxBatchSize",
			Value:   fmt.Sprintf("%d", cfg.Batch.MaxBatchSize),
			Message: "must be > 0",
		})
	}
	if cfg.Batch.MaxRetries < 0 {
		errs = append(errs, ValidationError{
			Field:   "Batch.MaxRetries",
			Value:   fmt.Sprintf("%d", cfg.Batch.MaxRetries),
			Message: "must be >= 0",
		})
	}

	// PrivateIPAddressPattern regex
	if cfg.PrivateIPAddressPattern != "" {
		if _, err := regexp.Compile(cfg.PrivateIPAddressPattern); err != nil {
			errs = append(errs, ValidationError{
				Field:   "PrivateIPAddressPattern",
				Value:   cfg.PrivateIPAddressPattern,
				Message: fmt.Sprintf("invalid regex: %v", err),
			})
		}
	}

	// SOCKSProxy Host/Port mutual dependency
	hasHost := cfg.SOCKSProxy.Host != ""
	hasPort := cfg.SOCKSProxy.Port > 0
	if hasHost != hasPort {
		errs = append(errs, ValidationError{
			Field:   "SOCKSProxy",
			Value:   fmt.Sprintf("Host=%q, Port=%d", cfg.SOCKSProxy.Host, cfg.SOCKSProxy.Port),
			Message: "Host and Port must both be set or both be empty",
		})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ValidateMonitorConfig validates Monitor.json configuration.
func ValidateMonitorConfig(mc *MonitorConfig) error {
	var errs ValidationErrors

	for name, cc := range mc.Collectors {
		if !cc.Enabled {
			continue
		}
		if cc.Interval < time.Second {
			errs = append(errs, ValidationError{
				Field:   fmt.Sprintf("Collectors.%s.Interval", name),
				Value:   cc.Interval.String(),
				Message: "must be >= 1s for enabled collectors",
			})
		}
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// ValidateLoggingConfig validates Logging.json configuration.
func ValidateLoggingConfig(lc *logger.Config) error {
	var errs ValidationErrors

	validLevels := map[string]bool{
		"trace": true, "debug": true, "info": true, "warn": true,
		"error": true, "fatal": true, "panic": true, "disabled": true,
	}
	if !validLevels[strings.ToLower(lc.Level)] {
		errs = append(errs, ValidationError{
			Field:   "Level",
			Value:   lc.Level,
			Message: `must be one of: trace, debug, info, warn, error, fatal, panic, disabled`,
		})
	}

	if lc.MaxSizeMB <= 0 {
		errs = append(errs, ValidationError{
			Field:   "MaxSizeMB",
			Value:   fmt.Sprintf("%d", lc.MaxSizeMB),
			Message: "must be > 0",
		})
	}

	if lc.MaxBackups < 0 {
		errs = append(errs, ValidationError{
			Field:   "MaxBackups",
			Value:   fmt.Sprintf("%d", lc.MaxBackups),
			Message: "must be >= 0",
		})
	}

	if lc.MaxAgeDays < 0 {
		errs = append(errs, ValidationError{
			Field:   "MaxAgeDays",
			Value:   fmt.Sprintf("%d", lc.MaxAgeDays),
			Message: "must be >= 0",
		})
	}

	if len(errs) > 0 {
		return errs
	}
	return nil
}

// validatePort checks if a port is in the valid range [1, 65535].
func validatePort(errs *ValidationErrors, field string, port int) {
	if port < 1 || port > 65535 {
		*errs = append(*errs, ValidationError{
			Field:   field,
			Value:   fmt.Sprintf("%d", port),
			Message: "port must be between 1 and 65535",
		})
	}
}
