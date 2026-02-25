package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

var fileTestTimestamp = time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC)

func tempFileConfig(t *testing.T, format string) config.FileConfig {
	t.Helper()
	dir := t.TempDir()
	return config.FileConfig{
		FilePath:   filepath.Join(dir, "metrics.log"),
		MaxSizeMB:  10,
		MaxBackups: 1,
		Console:    false,
		Pretty:     false,
		Format:     format,
	}
}

// --- Constructor tests ---

func TestNewFileSender_DefaultFormat(t *testing.T) {
	cfg := tempFileConfig(t, "")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()
	if s.format != "legacy" {
		t.Errorf("expected default format 'legacy', got %q", s.format)
	}
}

func TestNewFileSender_JSONFormat(t *testing.T) {
	cfg := tempFileConfig(t, "json")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()
	if s.format != "json" {
		t.Errorf("expected format 'json', got %q", s.format)
	}
}

func TestNewFileSender_InvalidFormat(t *testing.T) {
	cfg := tempFileConfig(t, "xml")
	_, err := NewFileSender(cfg)
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported file format") {
		t.Errorf("expected 'unsupported file format' error, got: %v", err)
	}
}

// --- JSON format test ---

func TestFileSender_Send_JSONFormat(t *testing.T) {
	cfg := tempFileConfig(t, "json")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: fileTestTimestamp,
		Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
	}

	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	content, err := os.ReadFile(cfg.FilePath)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}

	// Should be valid JSON
	var result collector.MetricData
	if err := json.Unmarshal(bytes.TrimSpace(content), &result); err != nil {
		t.Fatalf("output is not valid JSON: %v\ncontent: %s", err, content)
	}
	if result.Type != "cpu" {
		t.Errorf("expected type 'cpu', got %q", result.Type)
	}
}

// --- Legacy format tests per collector type ---

func readLegacyOutput(t *testing.T, filePath string) []string {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("failed to read output file: %v", err)
	}
	raw := strings.TrimSpace(string(content))
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\n")
}

func TestFileSender_Legacy_CPU(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: fileTestTimestamp,
		Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	expected := "2026-02-24 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5"
	if lines[0] != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, lines[0])
	}
}

func TestFileSender_Legacy_Memory(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "memory",
		Timestamp: fileTestTimestamp,
		Data:      collector.MemoryData{UsagePercent: 75.0, TotalBytes: 16000000000, UsedBytes: 12000000000},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	expecteds := []string{
		"2026-02-24 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_pct,value:75",
		"2026-02-24 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_free_pct,value:25",
		"2026-02-24 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_size,value:12000000000",
	}
	for i, exp := range expecteds {
		if lines[i] != exp {
			t.Errorf("line %d:\n  expected: %s\n  got:      %s", i, exp, lines[i])
		}
	}
}

func TestFileSender_Legacy_Disk(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "disk",
		Timestamp: fileTestTimestamp,
		Data: collector.DiskData{
			Partitions: []collector.DiskPartition{
				{Mountpoint: "C:", UsagePercent: 60.0},
				{Mountpoint: "D:", UsagePercent: 30.0},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:disk") || !strings.Contains(lines[0], "metric:C:") {
		t.Errorf("unexpected line 0: %s", lines[0])
	}
	if !strings.Contains(lines[1], "category:disk") || !strings.Contains(lines[1], "metric:D:") {
		t.Errorf("unexpected line 1: %s", lines[1])
	}
}

func TestFileSender_Legacy_Network(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "network",
		Timestamp: fileTestTimestamp,
		Data: collector.NetworkData{
			Interfaces: []collector.NetworkInterface{
				{Name: "Ethernet", BytesRecv: 1000, BytesSent: 2000, BytesRecvRate: 500.5, BytesSentRate: 250.3},
				{Name: "Wi-Fi", BytesRecv: 3000, BytesSent: 4000, BytesRecvRate: 100.0, BytesSentRate: 50.0},
			},
			TCPInboundCount:  42,
			TCPOutboundCount: 38,
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	// 2 (all_inbound/all_outbound) + 2 interfaces * 2 (recv_rate/sent_rate) = 6 lines
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "proc:@system,metric:all_inbound,value:42") {
		t.Errorf("unexpected inbound line: %s", lines[0])
	}
	if !strings.Contains(lines[1], "proc:@system,metric:all_outbound,value:38") {
		t.Errorf("unexpected outbound line: %s", lines[1])
	}
	if !strings.Contains(lines[2], "proc:Ethernet,metric:recv_rate,value:500.5") {
		t.Errorf("unexpected Ethernet recv_rate line: %s", lines[2])
	}
	if !strings.Contains(lines[3], "proc:Ethernet,metric:sent_rate,value:250.3") {
		t.Errorf("unexpected Ethernet sent_rate line: %s", lines[3])
	}
	if !strings.Contains(lines[4], "proc:Wi-Fi,metric:recv_rate,value:100") {
		t.Errorf("unexpected Wi-Fi recv_rate line: %s", lines[4])
	}
	if !strings.Contains(lines[5], "proc:Wi-Fi,metric:sent_rate,value:50") {
		t.Errorf("unexpected Wi-Fi sent_rate line: %s", lines[5])
	}
}

func TestFileSender_Legacy_CPUProcess(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "cpu_process",
		Timestamp: fileTestTimestamp,
		Data: collector.ProcessCPUData{
			Processes: []collector.ProcessCPU{
				{PID: 1234, Name: "python.exe", CPUPercent: 12.5},
				{PID: 5678, Name: "java.exe", CPUPercent: 8.3},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "pid:1234,proc:python.exe,metric:used_pct,value:12.5") {
		t.Errorf("unexpected line 0: %s", lines[0])
	}
	if !strings.Contains(lines[1], "pid:5678,proc:java.exe,metric:used_pct,value:8.3") {
		t.Errorf("unexpected line 1: %s", lines[1])
	}
}

func TestFileSender_Legacy_MemoryProcess(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "memory_process",
		Timestamp: fileTestTimestamp,
		Data: collector.ProcessMemoryData{
			Processes: []collector.ProcessMemory{
				{PID: 1234, Name: "python.exe", RSS: 104857600},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:memory,pid:1234,proc:python.exe,metric:used,value:104857600") {
		t.Errorf("unexpected line: %s", lines[0])
	}
}

func TestFileSender_Legacy_Temperature(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "temperature",
		Timestamp: fileTestTimestamp,
		Data: collector.TemperatureData{
			Sensors: []collector.TemperatureSensor{
				{Name: "coretemp_0", Temperature: 65.0},
				{Name: "coretemp_1", Temperature: 70.0},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:temperature") || !strings.Contains(lines[0], "metric:coretemp_0") {
		t.Errorf("unexpected line 0: %s", lines[0])
	}
	if !strings.Contains(lines[1], "category:temperature") || !strings.Contains(lines[1], "metric:coretemp_1") {
		t.Errorf("unexpected line 1: %s", lines[1])
	}
}

func TestFileSender_Legacy_GPU(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	temp := 75.0
	coreLoad := 90.0
	memLoad := 60.0
	fanSpeed := 1800.0
	power := 320.5
	coreClock := 2520.0
	memClock := 1200.0
	data := &collector.MetricData{
		Type:      "gpu",
		Timestamp: fileTestTimestamp,
		Data: collector.GpuData{
			Gpus: []collector.GpuSensor{
				{
					Name: "RTX4090", Temperature: &temp, CoreLoad: &coreLoad, MemoryLoad: &memLoad,
					FanSpeed: &fanSpeed, Power: &power, CoreClock: &coreClock, MemoryClock: &memClock,
				},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 7 {
		t.Fatalf("expected 7 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:gpu") || !strings.Contains(lines[0], "RTX4090_temperature") {
		t.Errorf("unexpected line 0: %s", lines[0])
	}
	if !strings.Contains(lines[1], "RTX4090_core_load") {
		t.Errorf("unexpected line 1: %s", lines[1])
	}
	if !strings.Contains(lines[2], "RTX4090_memory_load") {
		t.Errorf("unexpected line 2: %s", lines[2])
	}
	if !strings.Contains(lines[3], "RTX4090_fan_speed") {
		t.Errorf("unexpected line 3: %s", lines[3])
	}
	if !strings.Contains(lines[4], "RTX4090_power") {
		t.Errorf("unexpected line 4: %s", lines[4])
	}
	if !strings.Contains(lines[5], "RTX4090_core_clock") {
		t.Errorf("unexpected line 5: %s", lines[5])
	}
	if !strings.Contains(lines[6], "RTX4090_memory_clock") {
		t.Errorf("unexpected line 6: %s", lines[6])
	}
}

func TestFileSender_Legacy_Fan(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "fan",
		Timestamp: fileTestTimestamp,
		Data: collector.FanData{
			Sensors: []collector.FanSensor{
				{Name: "cpu_fan", RPM: 1200},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:fan,pid:0,proc:@system,metric:cpu_fan,value:1200") {
		t.Errorf("unexpected line: %s", lines[0])
	}
}

func TestFileSender_Legacy_Voltage(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "voltage",
		Timestamp: fileTestTimestamp,
		Data: collector.VoltageData{
			Sensors: []collector.VoltageSensor{
				{Name: "vcore", Voltage: 1.25},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:voltage,pid:0,proc:@system,metric:vcore,value:1.25") {
		t.Errorf("unexpected line: %s", lines[0])
	}
}

func TestFileSender_Legacy_MotherboardTemp(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "motherboard_temp",
		Timestamp: fileTestTimestamp,
		Data: collector.MotherboardTempData{
			Sensors: []collector.MotherboardTempSensor{
				{Name: "mb_temp1", Temperature: 42.0},
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:motherboard_temp,pid:0,proc:@system,metric:mb_temp1,value:42") {
		t.Errorf("unexpected line: %s", lines[0])
	}
}

func TestFileSender_Legacy_StorageSmart(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	temp := 35.0
	remainLife := 98.0
	powerCycles := int64(150)
	data := &collector.MetricData{
		Type:      "storage_smart",
		Timestamp: fileTestTimestamp,
		Data: collector.StorageSmartData{
			Storages: []collector.StorageSmartSensor{
				{Name: "nvme0", Temperature: &temp, RemainingLife: &remainLife, PowerCycles: &powerCycles},
				{Name: "sda"}, // all nil → skip
			},
		},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "metric:nvme0_temperature,value:35") {
		t.Errorf("unexpected line 0: %s", lines[0])
	}
	if !strings.Contains(lines[1], "metric:nvme0_remaining_life,value:98") {
		t.Errorf("unexpected line 1: %s", lines[1])
	}
	if !strings.Contains(lines[2], "metric:nvme0_power_cycles,value:150") {
		t.Errorf("unexpected line 2: %s", lines[2])
	}
}

// --- Edge case tests ---

func TestFileSender_Legacy_UnknownType(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "unknown_type",
		Timestamp: fileTestTimestamp,
		Data:      nil,
	}
	err = s.Send(context.Background(), data)
	if err != nil {
		t.Fatalf("expected no error for unknown type, got: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 0 {
		t.Errorf("expected 0 lines for unknown type, got %d", len(lines))
	}
}

func TestFileSender_Legacy_ClosedSender(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	s.Close()

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: fileTestTimestamp,
		Data:      collector.CPUData{UsagePercent: 45.5},
	}
	err = s.Send(context.Background(), data)
	if err == nil {
		t.Fatal("expected error for closed sender, got nil")
	}
	if !strings.Contains(err.Error(), "sender is closed") {
		t.Errorf("expected 'sender is closed' error, got: %v", err)
	}
}

func TestFileSender_Legacy_GrokCompatible(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: fileTestTimestamp,
		Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
	}
	if err := s.Send(context.Background(), data); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	// Compare with direct EARSRow.ToLegacyString() output
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 EARSRow, got %d", len(rows))
	}
	expected := rows[0].ToLegacyString()
	if lines[0] != expected {
		t.Errorf("file output doesn't match EARSRow.ToLegacyString():\n  file:     %s\n  expected: %s", lines[0], expected)
	}
}

func TestFileSender_SendBatch_Legacy(t *testing.T) {
	cfg := tempFileConfig(t, "legacy")
	s, err := NewFileSender(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer s.Close()

	batch := []*collector.MetricData{
		{
			Type:      "cpu",
			Timestamp: fileTestTimestamp,
			Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
		},
		{
			Type:      "memory",
			Timestamp: fileTestTimestamp,
			Data:      collector.MemoryData{UsagePercent: 75.0, TotalBytes: 16000000000, UsedBytes: 12000000000},
		},
	}

	if err := s.SendBatch(context.Background(), batch); err != nil {
		t.Fatalf("SendBatch failed: %v", err)
	}

	lines := readLegacyOutput(t, cfg.FilePath)
	// cpu: 1 line + memory: 3 lines = 4 lines
	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (1 cpu + 3 memory), got %d", len(lines))
	}
	if !strings.Contains(lines[0], "category:cpu") {
		t.Errorf("expected cpu line first, got: %s", lines[0])
	}
	if !strings.Contains(lines[1], "category:memory") {
		t.Errorf("expected memory line, got: %s", lines[1])
	}
}
