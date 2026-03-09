package config

import (
	"strings"
	"testing"
	"time"

	"resourceagent/internal/logger"
)

// --- Step 1: ValidationError type ---

func TestValidationErrors_Error_SingleError(t *testing.T) {
	errs := ValidationErrors{
		{Field: "SenderType", Value: "invalid", Message: `must be one of: kafka, kafkarest, file`},
	}

	got := errs.Error()

	if !strings.Contains(got, "1 error") {
		t.Errorf("expected '1 error' in message, got: %s", got)
	}
	if !strings.Contains(got, `SenderType="invalid"`) {
		t.Errorf("expected field=value format, got: %s", got)
	}
}

func TestValidationErrors_Error_MultipleErrors(t *testing.T) {
	errs := ValidationErrors{
		{Field: "SenderType", Value: "invalid", Message: `must be one of: kafka, kafkarest, file`},
		{Field: "Kafka.BrokerPort", Value: "99999", Message: "port must be between 1 and 65535"},
	}

	got := errs.Error()

	if !strings.Contains(got, "2 errors") {
		t.Errorf("expected '2 errors' in message, got: %s", got)
	}
	if !strings.Contains(got, "[1]") || !strings.Contains(got, "[2]") {
		t.Errorf("expected numbered errors, got: %s", got)
	}
}

// --- Step 2: ValidateConfig P0 ---

func TestValidateConfig_ValidDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	// DefaultConfig has SenderType="kafka" but empty VirtualAddressList.
	// For kafka sender, VirtualAddressList is required — so set it.
	cfg.VirtualAddressList = "10.0.0.1"

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("DefaultConfig should pass validation, got: %v", err)
	}
}

func TestValidateConfig_InvalidSenderType(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SenderType = "invalid"
	cfg.VirtualAddressList = "10.0.0.1"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid SenderType")
	}
	assertFieldError(t, err, "SenderType")
}

func TestValidateConfig_NonFile_MissingVirtualAddressList(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SenderType = "kafka"
	cfg.VirtualAddressList = ""

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for missing VirtualAddressList with kafka sender")
	}
	assertFieldError(t, err, "VirtualAddressList")
}

func TestValidateConfig_File_NoVirtualAddressList(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SenderType = "file"
	cfg.VirtualAddressList = ""

	err := ValidateConfig(cfg)
	if err != nil {
		t.Errorf("file sender should not require VirtualAddressList, got: %v", err)
	}
}

func TestValidateConfig_InvalidPorts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.Kafka.BrokerPort = 99999
	cfg.Redis.Port = 0
	cfg.ServiceDiscoveryPort = -1

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected errors for invalid ports")
	}
	errs := err.(ValidationErrors)
	assertFieldError(t, errs, "Kafka.BrokerPort")
	assertFieldError(t, errs, "Redis.Port")
	assertFieldError(t, errs, "ServiceDiscoveryPort")
}

func TestValidateConfig_InvalidFileFormat(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SenderType = "file"
	cfg.File.Format = "xml"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid File.Format")
	}
	assertFieldError(t, err, "File.Format")
}

func TestValidateConfig_MultipleErrors(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SenderType = "invalid"
	cfg.Kafka.BrokerPort = 99999

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected multiple errors")
	}
	errs := err.(ValidationErrors)
	if len(errs) < 2 {
		t.Errorf("expected at least 2 errors, got %d", len(errs))
	}
}

// --- Step 3: ValidateConfig P1 ---

func TestValidateConfig_InvalidCompression(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.Kafka.Compression = "brotli"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid compression")
	}
	assertFieldError(t, err, "Kafka.Compression")
}

func TestValidateConfig_InvalidRequiredAcks(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.Kafka.RequiredAcks = 2

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid RequiredAcks")
	}
	assertFieldError(t, err, "Kafka.RequiredAcks")
}

func TestValidateConfig_InvalidBatchFields(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.Batch.FlushMessages = 0
	cfg.Batch.MaxBatchSize = -1
	cfg.Batch.MaxRetries = -1

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected errors for invalid batch fields")
	}
	errs := err.(ValidationErrors)
	assertFieldError(t, errs, "Batch.FlushMessages")
	assertFieldError(t, errs, "Batch.MaxBatchSize")
	assertFieldError(t, errs, "Batch.MaxRetries")
}

func TestValidateConfig_InvalidRegexPattern(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.PrivateIPAddressPattern = "[invalid"

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid regex pattern")
	}
	assertFieldError(t, err, "PrivateIPAddressPattern")
}

func TestValidateConfig_SOCKSProxy_Mismatch(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.SOCKSProxy.Host = "proxy.local"
	cfg.SOCKSProxy.Port = 0

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for SOCKS proxy host without port")
	}
	assertFieldError(t, err, "SOCKSProxy")
}

func TestValidateConfig_SOCKSProxy_PortOutOfRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.VirtualAddressList = "10.0.0.1"
	cfg.SOCKSProxy.Host = "proxy.local"
	cfg.SOCKSProxy.Port = 70000

	err := ValidateConfig(cfg)
	if err == nil {
		t.Fatal("expected error for SOCKS proxy port out of range")
	}
	assertFieldError(t, err, "SOCKSProxy.Port")
}

// --- Step 4: ValidateMonitorConfig ---

func TestValidateMonitorConfig_ValidConfig(t *testing.T) {
	mc := &MonitorConfig{
		Collectors: map[string]CollectorConfig{
			"CPU": {Enabled: true, Interval: 5 * time.Second},
		},
	}

	err := ValidateMonitorConfig(mc)
	if err != nil {
		t.Errorf("valid monitor config should pass, got: %v", err)
	}
}

func TestValidateMonitorConfig_TooShortInterval(t *testing.T) {
	mc := &MonitorConfig{
		Collectors: map[string]CollectorConfig{
			"CPU": {Enabled: true, Interval: 500 * time.Millisecond},
		},
	}

	err := ValidateMonitorConfig(mc)
	if err == nil {
		t.Fatal("expected error for interval < 1s")
	}
	assertFieldError(t, err, "Collectors.CPU.Interval")
}

func TestValidateMonitorConfig_DisabledCollector_ShortIntervalOK(t *testing.T) {
	mc := &MonitorConfig{
		Collectors: map[string]CollectorConfig{
			"CPU": {Enabled: false, Interval: 100 * time.Millisecond},
		},
	}

	err := ValidateMonitorConfig(mc)
	if err != nil {
		t.Errorf("disabled collector should skip interval check, got: %v", err)
	}
}

// --- Step 5: ValidateLoggingConfig ---

func TestValidateLoggingConfig_ValidDefault(t *testing.T) {
	lc := logger.DefaultConfig()

	err := ValidateLoggingConfig(&lc)
	if err != nil {
		t.Errorf("default logging config should pass, got: %v", err)
	}
}

func TestValidateLoggingConfig_InvalidLevel(t *testing.T) {
	lc := logger.DefaultConfig()
	lc.Level = "verbose"

	err := ValidateLoggingConfig(&lc)
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	assertFieldError(t, err, "Level")
}

func TestValidateLoggingConfig_InvalidMaxSizeMB(t *testing.T) {
	lc := logger.DefaultConfig()
	lc.MaxSizeMB = 0

	err := ValidateLoggingConfig(&lc)
	if err == nil {
		t.Fatal("expected error for MaxSizeMB <= 0")
	}
	assertFieldError(t, err, "MaxSizeMB")
}

func TestValidateLoggingConfig_InvalidMaxBackupsAndAge(t *testing.T) {
	lc := logger.DefaultConfig()
	lc.MaxBackups = -1
	lc.MaxAgeDays = -1

	err := ValidateLoggingConfig(&lc)
	if err == nil {
		t.Fatal("expected errors for negative MaxBackups/MaxAgeDays")
	}
	errs := err.(ValidationErrors)
	assertFieldError(t, errs, "MaxBackups")
	assertFieldError(t, errs, "MaxAgeDays")
}

// --- helpers ---

func assertFieldError(t *testing.T, err error, field string) {
	t.Helper()
	errs, ok := err.(ValidationErrors)
	if !ok {
		t.Fatalf("expected ValidationErrors, got %T: %v", err, err)
	}
	for _, ve := range errs {
		if ve.Field == field {
			return
		}
	}
	t.Errorf("expected error for field %q, but not found in: %v", field, err)
}
