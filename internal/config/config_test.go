package config

import (
	"encoding/json"
	"os"
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
	if cfg.ResourceMonitorTopic != "process" {
		t.Errorf("expected ResourceMonitorTopic='process', got %q", cfg.ResourceMonitorTopic)
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

// --- Parse Tests (PascalCase JSON) ---

func TestParse_WithRedisConfig(t *testing.T) {
	input := `{
		"VirtualAddressList": "10.20.30.40",
		"Redis": {
			"Port": 26379,
			"Password": "secret",
			"DB": 5
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.VirtualAddressList != "10.20.30.40" {
		t.Errorf("expected VirtualAddressList='10.20.30.40', got %q", cfg.VirtualAddressList)
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
		"ServiceDiscoveryPort": 60009,
		"ResourceMonitorTopic": "model"
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
		"SocksProxy": {
			"Host": "proxy.example.com",
			"Port": 1080
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
		"PrivateIPAddressPattern": "^10\\..*"
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
	input := `{
		"Agent": {
			"ID": "test-agent"
		},
		"SenderType": "file",
		"Kafka": {
			"Brokers": ["broker1:9092"],
			"Topic": "test-topic"
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.ServiceDiscoveryPort != 50009 {
		t.Errorf("expected ServiceDiscoveryPort=50009 for backward compat, got %d", cfg.ServiceDiscoveryPort)
	}
	if cfg.ResourceMonitorTopic != "process" {
		t.Errorf("expected ResourceMonitorTopic='process' for backward compat, got %q", cfg.ResourceMonitorTopic)
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
		VirtualAddressList: "10.20.30.40,10.20.30.41",
		Redis: RedisConfig{
			Port:     26379,
			Password: "pass123",
			DB:       3,
		},
	}

	base.Merge(other)

	if base.VirtualAddressList != "10.20.30.40,10.20.30.41" {
		t.Errorf("expected VirtualAddressList='10.20.30.40,10.20.30.41', got %q", base.VirtualAddressList)
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

// --- RedisConfig.ResolvePassword Tests ---

func TestRedisConfig_ResolvePassword_EmptyUsesDefault(t *testing.T) {
	cfg := RedisConfig{Password: ""}
	if cfg.ResolvePassword() != DefaultRedisPassword {
		t.Errorf("expected default password %q, got %q", DefaultRedisPassword, cfg.ResolvePassword())
	}
}

func TestRedisConfig_ResolvePassword_ExplicitValue(t *testing.T) {
	cfg := RedisConfig{Password: "custom123"}
	if cfg.ResolvePassword() != "custom123" {
		t.Errorf("expected 'custom123', got %q", cfg.ResolvePassword())
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
	if topic != "tp_ETCH_all_resource" {
		t.Errorf("expected 'tp_ETCH_all_resource' for unknown mode, got %q", topic)
	}
}

func TestResolveTopic_EmptyMode(t *testing.T) {
	eqpInfo := &EqpInfoConfig{Process: "ETCH", EqpModel: "LAM_4520XLE"}
	topic := ResolveTopic("", eqpInfo)
	if topic != "tp_ETCH_all_resource" {
		t.Errorf("expected 'tp_ETCH_all_resource' for empty mode, got %q", topic)
	}
}

func TestParse_FullConfig_WithAllNewFields(t *testing.T) {
	input := `{
		"Agent": {
			"ID": "full-test"
		},
		"VirtualAddressList": "10.0.0.1,10.0.0.2",
		"ServiceDiscoveryPort": 60009,
		"ResourceMonitorTopic": "model",
		"Redis": {
			"Port": 26379,
			"Password": "pw",
			"DB": 7
		},
		"PrivateIPAddressPattern": "^172\\.16\\..*",
		"SocksProxy": {
			"Host": "socks5.local",
			"Port": 8080
		},
		"Kafka": {
			"Topic": "metrics"
		}
	}`

	cfg, err := Parse([]byte(input))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if cfg.VirtualAddressList != "10.0.0.1,10.0.0.2" {
		t.Errorf("VirtualAddressList: got %q", cfg.VirtualAddressList)
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
	if cfg.Agent.ID != "full-test" {
		t.Errorf("Agent.ID: got %q", cfg.Agent.ID)
	}
	if cfg.Kafka.Topic != "metrics" {
		t.Errorf("Kafka.Topic: got %q", cfg.Kafka.Topic)
	}
}

// --- MonitorConfig Tests ---

func TestDefaultMonitorConfig(t *testing.T) {
	mc := DefaultMonitorConfig()

	if mc.Collectors == nil {
		t.Fatal("expected non-nil Collectors map")
	}

	cpu, ok := mc.Collectors["cpu"]
	if !ok {
		t.Fatal("expected 'cpu' collector in defaults")
	}
	if !cpu.Enabled {
		t.Error("expected cpu.Enabled=true")
	}
}

func TestParseMonitor(t *testing.T) {
	input := `{
		"Collectors": {
			"cpu": {
				"Enabled": true,
				"Interval": "5s"
			},
			"memory": {
				"Enabled": false,
				"Interval": "20s"
			}
		}
	}`

	mc, err := ParseMonitor([]byte(input))
	if err != nil {
		t.Fatalf("ParseMonitor failed: %v", err)
	}

	cpu, ok := mc.Collectors["cpu"]
	if !ok {
		t.Fatal("expected 'cpu' collector")
	}
	if !cpu.Enabled {
		t.Error("expected cpu.Enabled=true")
	}
	if cpu.Interval != 5*1e9 {
		t.Errorf("expected cpu.Interval=5s, got %v", cpu.Interval)
	}

	mem, ok := mc.Collectors["memory"]
	if !ok {
		t.Fatal("expected 'memory' collector")
	}
	if mem.Enabled {
		t.Error("expected memory.Enabled=false")
	}
}

func TestParseMonitor_EmptyJSON(t *testing.T) {
	mc, err := ParseMonitor([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseMonitor failed: %v", err)
	}

	// Should have defaults
	if _, ok := mc.Collectors["cpu"]; !ok {
		t.Error("expected default 'cpu' collector after parsing empty JSON")
	}
}

func TestMonitorConfig_Merge(t *testing.T) {
	base := DefaultMonitorConfig()
	other := &MonitorConfig{
		Collectors: map[string]CollectorConfig{
			"cpu": {Enabled: false, Interval: 5e9},
		},
	}

	base.Merge(other)

	cpu := base.Collectors["cpu"]
	if cpu.Enabled {
		t.Error("expected cpu.Enabled=false after merge")
	}
	if cpu.Interval != 5e9 {
		t.Errorf("expected cpu.Interval=5s after merge, got %v", cpu.Interval)
	}

	// memory should still exist from defaults
	if _, ok := base.Collectors["memory"]; !ok {
		t.Error("expected 'memory' collector preserved after merge")
	}
}

// --- ParseLogging Tests ---

func TestParseLogging(t *testing.T) {
	input := `{
		"Level": "debug",
		"FilePath": "/var/log/agent.log",
		"MaxSizeMB": 20,
		"MaxBackups": 3,
		"MaxAgeDays": 7,
		"Compress": false,
		"Console": true
	}`

	cfg, err := ParseLogging([]byte(input))
	if err != nil {
		t.Fatalf("ParseLogging failed: %v", err)
	}

	if cfg.Level != "debug" {
		t.Errorf("expected Level='debug', got %q", cfg.Level)
	}
	if cfg.FilePath != "/var/log/agent.log" {
		t.Errorf("expected FilePath='/var/log/agent.log', got %q", cfg.FilePath)
	}
	if cfg.MaxSizeMB != 20 {
		t.Errorf("expected MaxSizeMB=20, got %d", cfg.MaxSizeMB)
	}
	if cfg.Console != true {
		t.Error("expected Console=true")
	}
}

func TestParseLogging_EmptyJSON(t *testing.T) {
	cfg, err := ParseLogging([]byte(`{}`))
	if err != nil {
		t.Fatalf("ParseLogging failed: %v", err)
	}

	// Should have defaults
	if cfg.Level != "info" {
		t.Errorf("expected default Level='info', got %q", cfg.Level)
	}
}

// --- LoadSplit Tests ---

func TestLoadSplit_ThreeFiles(t *testing.T) {
	dir := t.TempDir()

	configJSON := `{
		"Agent": {"ID": "split-test"},
		"SenderType": "file",
		"File": {"FilePath": "out.jsonl"}
	}`
	monitorJSON := `{
		"Collectors": {
			"cpu": {"Enabled": true, "Interval": "3s"}
		}
	}`
	loggingJSON := `{
		"Level": "warn",
		"Console": true
	}`

	writeFile(t, dir+"/ResourceAgent.json", configJSON)
	writeFile(t, dir+"/Monitor.json", monitorJSON)
	writeFile(t, dir+"/Logging.json", loggingJSON)

	cfg, mc, lc, err := LoadSplit(dir+"/ResourceAgent.json", dir+"/Monitor.json", dir+"/Logging.json")
	if err != nil {
		t.Fatalf("LoadSplit failed: %v", err)
	}

	// Config assertions
	if cfg.Agent.ID != "split-test" {
		t.Errorf("expected Agent.ID='split-test', got %q", cfg.Agent.ID)
	}
	if cfg.SenderType != "file" {
		t.Errorf("expected SenderType='file', got %q", cfg.SenderType)
	}

	// MonitorConfig assertions
	cpu, ok := mc.Collectors["cpu"]
	if !ok {
		t.Fatal("expected 'cpu' collector")
	}
	if cpu.Interval != 3e9 {
		t.Errorf("expected cpu.Interval=3s, got %v", cpu.Interval)
	}

	// LoggingConfig assertions
	if lc.Level != "warn" {
		t.Errorf("expected Level='warn', got %q", lc.Level)
	}
	if !lc.Console {
		t.Error("expected Console=true")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
