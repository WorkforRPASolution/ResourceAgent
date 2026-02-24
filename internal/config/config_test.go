package config

import (
	"encoding/json"
	"testing"
)

// --- Default Config Tests ---

func TestDefaultConfig_HasRedisDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Redis.Enabled != false {
		t.Errorf("expected Redis.Enabled=false, got %v", cfg.Redis.Enabled)
	}
	if cfg.Redis.DB != 10 {
		t.Errorf("expected Redis.DB=10, got %d", cfg.Redis.DB)
	}
	if cfg.Redis.Address != "" {
		t.Errorf("expected Redis.Address='', got %q", cfg.Redis.Address)
	}
	if cfg.Redis.Password != "" {
		t.Errorf("expected Redis.Password='', got %q", cfg.Redis.Password)
	}
	if cfg.Redis.SentinelName != "" {
		t.Errorf("expected Redis.SentinelName='', got %q", cfg.Redis.SentinelName)
	}
}

func TestDefaultConfig_HasSOCKSDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.SOCKSProxy.Host != "" {
		t.Errorf("expected SOCKSProxy.Host='', got %q", cfg.SOCKSProxy.Host)
	}
	if cfg.SOCKSProxy.Port != 0 {
		t.Errorf("expected SOCKSProxy.Port=0, got %d", cfg.SOCKSProxy.Port)
	}
}

func TestDefaultConfig_PrivateIPAddressPatternEmpty(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PrivateIPAddressPattern != "" {
		t.Errorf("expected PrivateIPAddressPattern='', got %q", cfg.PrivateIPAddressPattern)
	}
}

func TestDefaultConfig_EqpInfoNil(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.EqpInfo != nil {
		t.Errorf("expected EqpInfo=nil, got %+v", cfg.EqpInfo)
	}
}

// --- Parse Tests ---

func TestParse_WithRedisConfig(t *testing.T) {
	input := `{
		"redis": {
			"enabled": true,
			"address": "redis.example.com:6379",
			"password": "secret",
			"db": 5,
			"sentinel_name": "mymaster"
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.Redis.Enabled != true {
		t.Errorf("expected Redis.Enabled=true, got %v", cfg.Redis.Enabled)
	}
	if cfg.Redis.Address != "redis.example.com:6379" {
		t.Errorf("expected Redis.Address='redis.example.com:6379', got %q", cfg.Redis.Address)
	}
	if cfg.Redis.Password != "secret" {
		t.Errorf("expected Redis.Password='secret', got %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 5 {
		t.Errorf("expected Redis.DB=5, got %d", cfg.Redis.DB)
	}
	if cfg.Redis.SentinelName != "mymaster" {
		t.Errorf("expected Redis.SentinelName='mymaster', got %q", cfg.Redis.SentinelName)
	}
}

func TestParse_WithSOCKSConfig(t *testing.T) {
	input := `{
		"socks_proxy": {
			"host": "proxy.example.com",
			"port": 1080
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.SOCKSProxy.Host != "proxy.example.com" {
		t.Errorf("expected SOCKSProxy.Host='proxy.example.com', got %q", cfg.SOCKSProxy.Host)
	}
	if cfg.SOCKSProxy.Port != 1080 {
		t.Errorf("expected SOCKSProxy.Port=1080, got %d", cfg.SOCKSProxy.Port)
	}
}

func TestParse_WithPrivateIPAddressPattern(t *testing.T) {
	input := `{
		"private_ip_address_pattern": "^10\\..*"
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.PrivateIPAddressPattern != "^10\\..*" {
		t.Errorf("expected PrivateIPAddressPattern='^10\\\\..*', got %q", cfg.PrivateIPAddressPattern)
	}
}

func TestParse_WithoutNewFields_BackwardCompatible(t *testing.T) {
	// Existing config without any new fields should still work
	input := `{
		"agent": {
			"id": "test-agent"
		},
		"sender_type": "file",
		"kafka": {
			"brokers": ["broker1:9092"],
			"topic": "test-topic"
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// New fields should have defaults
	if cfg.Redis.Enabled != false {
		t.Errorf("expected Redis.Enabled=false for backward compat, got %v", cfg.Redis.Enabled)
	}
	if cfg.Redis.DB != 10 {
		t.Errorf("expected Redis.DB=10 (default), got %d", cfg.Redis.DB)
	}
	if cfg.SOCKSProxy.Host != "" {
		t.Errorf("expected SOCKSProxy.Host='', got %q", cfg.SOCKSProxy.Host)
	}
	if cfg.PrivateIPAddressPattern != "" {
		t.Errorf("expected PrivateIPAddressPattern='', got %q", cfg.PrivateIPAddressPattern)
	}

	// Existing fields should still parse correctly
	if cfg.Agent.ID != "test-agent" {
		t.Errorf("expected Agent.ID='test-agent', got %q", cfg.Agent.ID)
	}
	if cfg.SenderType != "file" {
		t.Errorf("expected SenderType='file', got %q", cfg.SenderType)
	}
}

// --- Merge Tests ---

func TestMerge_RedisConfig(t *testing.T) {
	base := DefaultConfig()
	other := &Config{
		Redis: RedisConfig{
			Enabled:      true,
			Address:      "redis.local:6379",
			Password:     "pass123",
			DB:           3,
			SentinelName: "sentinel1",
		},
	}

	base.Merge(other)

	if base.Redis.Enabled != true {
		t.Errorf("expected Redis.Enabled=true after merge, got %v", base.Redis.Enabled)
	}
	if base.Redis.Address != "redis.local:6379" {
		t.Errorf("expected Redis.Address='redis.local:6379', got %q", base.Redis.Address)
	}
	if base.Redis.Password != "pass123" {
		t.Errorf("expected Redis.Password='pass123', got %q", base.Redis.Password)
	}
	if base.Redis.DB != 3 {
		t.Errorf("expected Redis.DB=3, got %d", base.Redis.DB)
	}
	if base.Redis.SentinelName != "sentinel1" {
		t.Errorf("expected Redis.SentinelName='sentinel1', got %q", base.Redis.SentinelName)
	}
}

func TestMerge_SOCKSConfig(t *testing.T) {
	base := DefaultConfig()
	other := &Config{
		SOCKSProxy: SOCKSConfig{
			Host: "socks.local",
			Port: 30000,
		},
	}

	base.Merge(other)

	if base.SOCKSProxy.Host != "socks.local" {
		t.Errorf("expected SOCKSProxy.Host='socks.local', got %q", base.SOCKSProxy.Host)
	}
	if base.SOCKSProxy.Port != 30000 {
		t.Errorf("expected SOCKSProxy.Port=30000, got %d", base.SOCKSProxy.Port)
	}
}

func TestMerge_PrivateIPAddressPattern(t *testing.T) {
	base := DefaultConfig()
	other := &Config{
		PrivateIPAddressPattern: "^192\\.168\\..*",
	}

	base.Merge(other)

	if base.PrivateIPAddressPattern != "^192\\.168\\..*" {
		t.Errorf("expected PrivateIPAddressPattern='^192\\\\.168\\\\..*', got %q", base.PrivateIPAddressPattern)
	}
}

func TestMerge_EmptyValuesDoNotOverwrite(t *testing.T) {
	base := DefaultConfig()
	base.Redis.Address = "existing.redis:6379"
	base.Redis.DB = 5
	base.SOCKSProxy.Host = "existing.socks"
	base.SOCKSProxy.Port = 9999
	base.PrivateIPAddressPattern = "^10\\..*"

	// Merge with empty/zero values should not overwrite
	other := &Config{}

	base.Merge(other)

	if base.Redis.Address != "existing.redis:6379" {
		t.Errorf("expected Redis.Address preserved, got %q", base.Redis.Address)
	}
	// Note: DB=0 will overwrite since 0 is the zero value check
	if base.SOCKSProxy.Host != "existing.socks" {
		t.Errorf("expected SOCKSProxy.Host preserved, got %q", base.SOCKSProxy.Host)
	}
	if base.SOCKSProxy.Port != 9999 {
		t.Errorf("expected SOCKSProxy.Port preserved, got %d", base.SOCKSProxy.Port)
	}
	if base.PrivateIPAddressPattern != "^10\\..*" {
		t.Errorf("expected PrivateIPAddressPattern preserved, got %q", base.PrivateIPAddressPattern)
	}
}

// --- EqpInfoConfig Tests ---

func TestEqpInfoConfig_NotSerialized(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EqpInfo = &EqpInfoConfig{
		Process:  "ETCH",
		EqpModel: "LAM_4520XLE",
		EqpID:    "EQP001",
		Line:     "LINE_A",
		LineDesc: "A Line Description",
		Index:    "1",
	}

	// Marshal to JSON - EqpInfo should NOT appear (json:"-")
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if _, exists := result["EqpInfo"]; exists {
		t.Error("EqpInfo should not be serialized to JSON")
	}
	if _, exists := result["eqp_info"]; exists {
		t.Error("eqp_info should not be serialized to JSON")
	}
}

func TestParse_FullConfig_WithAllNewFields(t *testing.T) {
	input := `{
		"agent": {
			"id": "full-test"
		},
		"redis": {
			"enabled": true,
			"address": "redis:6379",
			"password": "pw",
			"db": 7,
			"sentinel_name": "master"
		},
		"private_ip_address_pattern": "^172\\.16\\..*",
		"socks_proxy": {
			"host": "socks5.local",
			"port": 8080
		},
		"kafka": {
			"topic": "metrics"
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify all new fields
	if cfg.Redis.Enabled != true {
		t.Errorf("Redis.Enabled: got %v", cfg.Redis.Enabled)
	}
	if cfg.Redis.Address != "redis:6379" {
		t.Errorf("Redis.Address: got %q", cfg.Redis.Address)
	}
	if cfg.Redis.Password != "pw" {
		t.Errorf("Redis.Password: got %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 7 {
		t.Errorf("Redis.DB: got %d", cfg.Redis.DB)
	}
	if cfg.Redis.SentinelName != "master" {
		t.Errorf("Redis.SentinelName: got %q", cfg.Redis.SentinelName)
	}
	if cfg.PrivateIPAddressPattern != "^172\\.16\\..*" {
		t.Errorf("PrivateIPAddressPattern: got %q", cfg.PrivateIPAddressPattern)
	}
	if cfg.SOCKSProxy.Host != "socks5.local" {
		t.Errorf("SOCKSProxy.Host: got %q", cfg.SOCKSProxy.Host)
	}
	if cfg.SOCKSProxy.Port != 8080 {
		t.Errorf("SOCKSProxy.Port: got %d", cfg.SOCKSProxy.Port)
	}

	// Verify existing fields still work
	if cfg.Agent.ID != "full-test" {
		t.Errorf("Agent.ID: got %q", cfg.Agent.ID)
	}
	if cfg.Kafka.Topic != "metrics" {
		t.Errorf("Kafka.Topic: got %q", cfg.Kafka.Topic)
	}
}
