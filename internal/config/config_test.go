package config

import (
	"encoding/json"
	"testing"
)

// --- Default Config Tests ---

func TestDefaultConfig_HasRedisDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Redis.DB != 10 {
		t.Errorf("expected Redis.DB=10, got %d", cfg.Redis.DB)
	}
	if cfg.Redis.Port != 6379 {
		t.Errorf("expected Redis.Port=6379, got %d", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "" {
		t.Errorf("expected Redis.Password='', got %q", cfg.Redis.Password)
	}
}

func TestDefaultConfig_HasServiceDiscoveryDefaults(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.ServiceDiscoveryPort != 50009 {
		t.Errorf("expected ServiceDiscoveryPort=50009, got %d", cfg.ServiceDiscoveryPort)
	}
	if cfg.ResourceMonitorTopic != "all" {
		t.Errorf("expected ResourceMonitorTopic='all', got %q", cfg.ResourceMonitorTopic)
	}
	if cfg.KafkaRestAddress != "" {
		t.Errorf("expected KafkaRestAddress='', got %q", cfg.KafkaRestAddress)
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
		"virtual_ip_list": "10.20.30.40",
		"redis": {
			"port": 26379,
			"password": "secret",
			"db": 5
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.VirtualIPList != "10.20.30.40" {
		t.Errorf("expected VirtualIPList='10.20.30.40', got %q", cfg.VirtualIPList)
	}
	if cfg.Redis.Port != 26379 {
		t.Errorf("expected Redis.Port=26379, got %d", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "secret" {
		t.Errorf("expected Redis.Password='secret', got %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 5 {
		t.Errorf("expected Redis.DB=5, got %d", cfg.Redis.DB)
	}
}

func TestParse_WithServiceDiscoveryConfig(t *testing.T) {
	input := `{
		"service_discovery_port": 60009,
		"resource_monitor_topic": "model"
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.ServiceDiscoveryPort != 60009 {
		t.Errorf("expected ServiceDiscoveryPort=60009, got %d", cfg.ServiceDiscoveryPort)
	}
	if cfg.ResourceMonitorTopic != "model" {
		t.Errorf("expected ResourceMonitorTopic='model', got %q", cfg.ResourceMonitorTopic)
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
	if cfg.ServiceDiscoveryPort != 50009 {
		t.Errorf("expected ServiceDiscoveryPort=50009 for backward compat, got %d", cfg.ServiceDiscoveryPort)
	}
	if cfg.ResourceMonitorTopic != "all" {
		t.Errorf("expected ResourceMonitorTopic='all' for backward compat, got %q", cfg.ResourceMonitorTopic)
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
		VirtualIPList: "10.20.30.40,10.20.30.41",
		Redis: RedisConfig{
			Port:     26379,
			Password: "pass123",
			DB:       3,
		},
	}

	base.Merge(other)

	if base.VirtualIPList != "10.20.30.40,10.20.30.41" {
		t.Errorf("expected VirtualIPList='10.20.30.40,10.20.30.41', got %q", base.VirtualIPList)
	}
	if base.Redis.Port != 26379 {
		t.Errorf("expected Redis.Port=26379, got %d", base.Redis.Port)
	}
	if base.Redis.Password != "pass123" {
		t.Errorf("expected Redis.Password='pass123', got %q", base.Redis.Password)
	}
	if base.Redis.DB != 3 {
		t.Errorf("expected Redis.DB=3, got %d", base.Redis.DB)
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
	base.Redis.Port = 26379
	base.Redis.DB = 5
	base.SOCKSProxy.Host = "existing.socks"
	base.SOCKSProxy.Port = 9999
	base.PrivateIPAddressPattern = "^10\\..*"

	// Merge with empty/zero values should not overwrite
	other := &Config{}

	base.Merge(other)

	if base.Redis.Port != 26379 {
		t.Errorf("expected Redis.Port preserved, got %d", base.Redis.Port)
	}
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

func TestMerge_ServiceDiscoveryConfig(t *testing.T) {
	base := DefaultConfig()
	other := &Config{
		ServiceDiscoveryPort: 60009,
		ResourceMonitorTopic: "model",
	}

	base.Merge(other)

	if base.ServiceDiscoveryPort != 60009 {
		t.Errorf("expected ServiceDiscoveryPort=60009, got %d", base.ServiceDiscoveryPort)
	}
	if base.ResourceMonitorTopic != "model" {
		t.Errorf("expected ResourceMonitorTopic='model', got %q", base.ResourceMonitorTopic)
	}
}

// --- ResolveTopic Tests ---

func TestResolveTopic_AllMode(t *testing.T) {
	eqpInfo := &EqpInfoConfig{Process: "ETCH", EqpModel: "LAM_4520XLE"}
	topic := ResolveTopic("all", eqpInfo)
	if topic != "tp_all_all_resource" {
		t.Errorf("expected 'tp_all_all_resource', got %q", topic)
	}
}

func TestResolveTopic_ModelMode(t *testing.T) {
	eqpInfo := &EqpInfoConfig{Process: "ETCH", EqpModel: "LAM_4520XLE"}
	topic := ResolveTopic("model", eqpInfo)
	if topic != "tp_ETCH_LAM_4520XLE_resource" {
		t.Errorf("expected 'tp_ETCH_LAM_4520XLE_resource', got %q", topic)
	}
}

func TestResolveTopic_ProcessMode(t *testing.T) {
	eqpInfo := &EqpInfoConfig{Process: "ETCH", EqpModel: "LAM_4520XLE"}
	topic := ResolveTopic("process", eqpInfo)
	if topic != "tp_ETCH_all_resource" {
		t.Errorf("expected 'tp_ETCH_all_resource', got %q", topic)
	}
}

func TestResolveTopic_DefaultMode(t *testing.T) {
	eqpInfo := &EqpInfoConfig{Process: "ETCH", EqpModel: "LAM_4520XLE"}
	topic := ResolveTopic("unknown", eqpInfo)
	if topic != "tp_all_all_resource" {
		t.Errorf("expected 'tp_all_all_resource' for unknown mode, got %q", topic)
	}
}

func TestResolveTopic_EmptyMode(t *testing.T) {
	eqpInfo := &EqpInfoConfig{Process: "ETCH", EqpModel: "LAM_4520XLE"}
	topic := ResolveTopic("", eqpInfo)
	if topic != "tp_all_all_resource" {
		t.Errorf("expected 'tp_all_all_resource' for empty mode, got %q", topic)
	}
}

func TestParse_FullConfig_WithAllNewFields(t *testing.T) {
	input := `{
		"agent": {
			"id": "full-test"
		},
		"virtual_ip_list": "10.0.0.1,10.0.0.2",
		"service_discovery_port": 60009,
		"resource_monitor_topic": "model",
		"redis": {
			"port": 26379,
			"password": "pw",
			"db": 7
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
	if cfg.VirtualIPList != "10.0.0.1,10.0.0.2" {
		t.Errorf("VirtualIPList: got %q", cfg.VirtualIPList)
	}
	if cfg.ServiceDiscoveryPort != 60009 {
		t.Errorf("ServiceDiscoveryPort: got %d", cfg.ServiceDiscoveryPort)
	}
	if cfg.ResourceMonitorTopic != "model" {
		t.Errorf("ResourceMonitorTopic: got %q", cfg.ResourceMonitorTopic)
	}
	if cfg.Redis.Port != 26379 {
		t.Errorf("Redis.Port: got %d", cfg.Redis.Port)
	}
	if cfg.Redis.Password != "pw" {
		t.Errorf("Redis.Password: got %q", cfg.Redis.Password)
	}
	if cfg.Redis.DB != 7 {
		t.Errorf("Redis.DB: got %d", cfg.Redis.DB)
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
