package sender

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"
	"time"

	"resourceagent/internal/collector"
)

var testTimestamp = time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC)

// --- FormatLegacyTimestamp / FormatJSONTimestamp ---

func TestFormatLegacyTimestamp(t *testing.T) {
	result := FormatLegacyTimestamp(testTimestamp)
	expected := "2026-02-24 10:30:45,123"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatLegacyTimestamp_ZeroMillis(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	result := FormatLegacyTimestamp(ts)
	expected := "2026-01-01 00:00:00,000"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatJSONTimestamp(t *testing.T) {
	result := FormatJSONTimestamp(testTimestamp)
	expected := "2026-02-24T10:30:45.123"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// --- ConvertToEARSRows ---

func TestConvertToEARSRows_CPU(t *testing.T) {
	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: testTimestamp,
		Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	r := rows[0]
	assertRow(t, r, "cpu", 0, "@system", "total_used_pct", 45.5)
}

func TestConvertToEARSRows_Memory(t *testing.T) {
	data := &collector.MetricData{
		Type:      "memory",
		Timestamp: testTimestamp,
		Data:      collector.MemoryData{UsagePercent: 75.0, TotalBytes: 16000000000},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "memory", 0, "@system", "total_used_pct", 75.0)
	assertRow(t, rows[1], "memory", 0, "@system", "total_free_pct", 25.0)
	assertRow(t, rows[2], "memory", 0, "@system", "total_used_size", 16000000000)
}

func TestConvertToEARSRows_Disk(t *testing.T) {
	data := &collector.MetricData{
		Type:      "disk",
		Timestamp: testTimestamp,
		Data: collector.DiskData{
			Partitions: []collector.DiskPartition{
				{Mountpoint: "C:", UsagePercent: 60.0},
				{Mountpoint: "D:", UsagePercent: 30.0},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "disk", 0, "@system", "C:", 60.0)
	assertRow(t, rows[1], "disk", 0, "@system", "D:", 30.0)
}

func TestConvertToEARSRows_Network(t *testing.T) {
	data := &collector.MetricData{
		Type:      "network",
		Timestamp: testTimestamp,
		Data: collector.NetworkData{
			Interfaces: []collector.NetworkInterface{
				{Name: "eth0", BytesRecv: 1000, BytesSent: 2000},
				{Name: "eth1", BytesRecv: 3000, BytesSent: 4000},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "network", 0, "@system", "all_inbound", 4000)  // 1000+3000
	assertRow(t, rows[1], "network", 0, "@system", "all_outbound", 6000) // 2000+4000
}

func TestConvertToEARSRows_CPUProcess(t *testing.T) {
	data := &collector.MetricData{
		Type:      "cpu_process",
		Timestamp: testTimestamp,
		Data: collector.ProcessCPUData{
			Processes: []collector.ProcessCPU{
				{PID: 1234, Name: "python.exe", CPUPercent: 12.5},
				{PID: 5678, Name: "java.exe", CPUPercent: 8.3},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "cpu", 1234, "python.exe", "used_pct", 12.5)
	assertRow(t, rows[1], "cpu", 5678, "java.exe", "used_pct", 8.3)
}

func TestConvertToEARSRows_MemoryProcess(t *testing.T) {
	data := &collector.MetricData{
		Type:      "memory_process",
		Timestamp: testTimestamp,
		Data: collector.ProcessMemoryData{
			Processes: []collector.ProcessMemory{
				{PID: 1234, Name: "python.exe", RSS: 104857600},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertRow(t, rows[0], "memory", 1234, "python.exe", "used", 104857600)
}

func TestConvertToEARSRows_Temperature(t *testing.T) {
	data := &collector.MetricData{
		Type:      "temperature",
		Timestamp: testTimestamp,
		Data: collector.TemperatureData{
			Sensors: []collector.TemperatureSensor{
				{Name: "coretemp_0", Temperature: 65.0},
				{Name: "coretemp_1", Temperature: 70.0},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "temperature", 0, "@system", "coretemp_0", 65.0)
	assertRow(t, rows[1], "temperature", 0, "@system", "coretemp_1", 70.0)
}

func TestConvertToEARSRows_GPU(t *testing.T) {
	temp := 75.0
	coreLoad := 90.0
	memLoad := 60.0

	data := &collector.MetricData{
		Type:      "gpu",
		Timestamp: testTimestamp,
		Data: collector.GpuData{
			Gpus: []collector.GpuSensor{
				{Name: "RTX4090", Temperature: &temp, CoreLoad: &coreLoad, MemoryLoad: &memLoad},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "gpu", 0, "@system", "RTX4090_temperature", 75.0)
	assertRow(t, rows[1], "gpu", 0, "@system", "RTX4090_core_load", 90.0)
	assertRow(t, rows[2], "gpu", 0, "@system", "RTX4090_memory_load", 60.0)
}

func TestConvertToEARSRows_GPU_NilFields(t *testing.T) {
	temp := 75.0
	data := &collector.MetricData{
		Type:      "gpu",
		Timestamp: testTimestamp,
		Data: collector.GpuData{
			Gpus: []collector.GpuSensor{
				{Name: "RTX4090", Temperature: &temp, CoreLoad: nil, MemoryLoad: nil},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (only temperature), got %d", len(rows))
	}
	assertRow(t, rows[0], "gpu", 0, "@system", "RTX4090_temperature", 75.0)
}

func TestConvertToEARSRows_Fan(t *testing.T) {
	data := &collector.MetricData{
		Type:      "fan",
		Timestamp: testTimestamp,
		Data: collector.FanData{
			Sensors: []collector.FanSensor{
				{Name: "cpu_fan", RPM: 1200},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertRow(t, rows[0], "fan", 0, "@system", "cpu_fan", 1200)
}

func TestConvertToEARSRows_Voltage(t *testing.T) {
	data := &collector.MetricData{
		Type:      "voltage",
		Timestamp: testTimestamp,
		Data: collector.VoltageData{
			Sensors: []collector.VoltageSensor{
				{Name: "vcore", Voltage: 1.25},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertRow(t, rows[0], "voltage", 0, "@system", "vcore", 1.25)
}

func TestConvertToEARSRows_MotherboardTemp(t *testing.T) {
	data := &collector.MetricData{
		Type:      "motherboard_temp",
		Timestamp: testTimestamp,
		Data: collector.MotherboardTempData{
			Sensors: []collector.MotherboardTempSensor{
				{Name: "mb_temp1", Temperature: 42.0},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertRow(t, rows[0], "motherboard_temp", 0, "@system", "mb_temp1", 42.0)
}

func TestConvertToEARSRows_StorageSmart(t *testing.T) {
	temp := 35.0
	data := &collector.MetricData{
		Type:      "storage_smart",
		Timestamp: testTimestamp,
		Data: collector.StorageSmartData{
			Storages: []collector.StorageSmartSensor{
				{Name: "nvme0", Temperature: &temp},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	assertRow(t, rows[0], "storage_smart", 0, "@system", "nvme0", 35.0)
}

func TestConvertToEARSRows_StorageSmart_NilTemp(t *testing.T) {
	data := &collector.MetricData{
		Type:      "storage_smart",
		Timestamp: testTimestamp,
		Data: collector.StorageSmartData{
			Storages: []collector.StorageSmartSensor{
				{Name: "nvme0", Temperature: nil},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for nil temperature, got %d", len(rows))
	}
}

func TestConvertToEARSRows_UnknownType(t *testing.T) {
	data := &collector.MetricData{
		Type:      "unknown_type",
		Timestamp: testTimestamp,
		Data:      nil,
	}
	rows := ConvertToEARSRows(data)
	if rows != nil {
		t.Fatalf("expected nil for unknown type, got %d rows", len(rows))
	}
}

// --- ToLegacyString ---

func TestToLegacyString_Format(t *testing.T) {
	row := EARSRow{
		Timestamp: testTimestamp,
		Category:  "cpu",
		PID:       0,
		ProcName:  "@system",
		Metric:    "total_used_pct",
		Value:     45.5,
	}
	result := row.ToLegacyString()
	expected := "2026-02-24 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5"
	if result != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, result)
	}
}

func TestToLegacyString_GrokPatternMatch(t *testing.T) {
	// Simplified Grok pattern as Go regex
	grokPattern := regexp.MustCompile(
		`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3}) category:(\w+),pid:(\d+),proc:([^,]+),metric:([^,]+),value:(.+)$`,
	)

	rows := []EARSRow{
		{Timestamp: testTimestamp, Category: "cpu", PID: 0, ProcName: "@system", Metric: "total_used_pct", Value: 45.5},
		{Timestamp: testTimestamp, Category: "memory", PID: 0, ProcName: "@system", Metric: "total_used_size", Value: 16000000000},
		{Timestamp: testTimestamp, Category: "cpu", PID: 1234, ProcName: "python.exe", Metric: "used_pct", Value: 12.5},
		{Timestamp: testTimestamp, Category: "disk", PID: 0, ProcName: "@system", Metric: "C:", Value: 60.0},
	}

	for i, row := range rows {
		s := row.ToLegacyString()
		if !grokPattern.MatchString(s) {
			t.Errorf("row %d: Grok pattern does not match: %q", i, s)
		}
	}
}

func TestToLegacyString_LargeIntegerValue(t *testing.T) {
	row := EARSRow{
		Timestamp: testTimestamp,
		Category:  "memory",
		PID:       0,
		ProcName:  "@system",
		Metric:    "total_used_size",
		Value:     16000000000,
	}
	result := row.ToLegacyString()
	// Extract the value portion and verify no scientific notation
	valueStr := formatValue(16000000000)
	if regexp.MustCompile(`[eE]`).MatchString(valueStr) {
		t.Errorf("value should not use scientific notation: %q", valueStr)
	}
	expected := "2026-02-24 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_size,value:16000000000"
	if result != expected {
		t.Errorf("expected:\n  %s\ngot:\n  %s", expected, result)
	}
}

// --- ToParsedData ---

func TestToParsedData_Structure(t *testing.T) {
	row := EARSRow{
		Timestamp: testTimestamp,
		Category:  "cpu",
		PID:       0,
		ProcName:  "@system",
		Metric:    "total_used_pct",
		Value:     45.5,
	}
	pdl := row.ToParsedData("PROCESS1")

	if pdl.Timestamp != "2026-02-24T10:30:45.123" {
		t.Errorf("timestamp: expected 2026-02-24T10:30:45.123, got %s", pdl.Timestamp)
	}
	if len(pdl.Data) != 6 {
		t.Fatalf("expected 6 ParsedData entries, got %d", len(pdl.Data))
	}

	expected := []ParsedData{
		{Name: "EARS_PROCESS", Value: "PROCESS1", Type: "String"},
		{Name: "EARS_CATEGORY", Value: "cpu", Type: "String"},
		{Name: "EARS_PID", Value: "0", Type: "Int"},
		{Name: "EARS_PROCNAME", Value: "@system", Type: "String"},
		{Name: "EARS_METRIC", Value: "total_used_pct", Type: "String"},
		{Name: "EARS_VALUE", Value: "45.5", Type: "Double"},
	}
	for i, e := range expected {
		if pdl.Data[i] != e {
			t.Errorf("Data[%d]: expected %+v, got %+v", i, e, pdl.Data[i])
		}
	}
}

func TestToParsedData_JSONMarshal(t *testing.T) {
	row := EARSRow{
		Timestamp: testTimestamp,
		Category:  "cpu",
		PID:       1234,
		ProcName:  "python.exe",
		Metric:    "used_pct",
		Value:     12.5,
	}
	pdl := row.ToParsedData("ARSAgent")

	b, err := json.Marshal(pdl)
	if err != nil {
		t.Fatalf("failed to marshal ParsedDataList: %v", err)
	}

	// Verify it can be unmarshaled back
	var result ParsedDataList
	if err := json.Unmarshal(b, &result); err != nil {
		t.Fatalf("failed to unmarshal ParsedDataList: %v", err)
	}
	if result.Timestamp != pdl.Timestamp {
		t.Errorf("timestamp mismatch after round-trip")
	}
	if len(result.Data) != 6 {
		t.Fatalf("expected 6 entries after round-trip, got %d", len(result.Data))
	}

	// Verify EARS_PROCESS is included
	found := false
	for _, d := range result.Data {
		if d.Name == "EARS_PROCESS" && d.Value == "ARSAgent" {
			found = true
			break
		}
	}
	if !found {
		t.Error("EARS_PROCESS not found in ParsedDataList")
	}
}

// --- Pointer type data ---

func TestConvertToEARSRows_PointerData(t *testing.T) {
	cpuData := &collector.CPUData{UsagePercent: 55.0, CoreCount: 8}
	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: testTimestamp,
		Data:      cpuData, // pointer type
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row for pointer data, got %d", len(rows))
	}
	assertRow(t, rows[0], "cpu", 0, "@system", "total_used_pct", 55.0)
}

// --- Helper ---

func assertRow(t *testing.T, row EARSRow, category string, pid int, procName, metric string, value float64) {
	t.Helper()
	if row.Category != category {
		t.Errorf("category: expected %q, got %q", category, row.Category)
	}
	if row.PID != pid {
		t.Errorf("pid: expected %d, got %d", pid, row.PID)
	}
	if row.ProcName != procName {
		t.Errorf("procName: expected %q, got %q", procName, row.ProcName)
	}
	if row.Metric != metric {
		t.Errorf("metric: expected %q, got %q", metric, row.Metric)
	}
	if row.Value != value {
		t.Errorf("value: expected %v, got %v", value, row.Value)
	}
	if row.Timestamp.IsZero() {
		t.Error("timestamp should not be zero")
	}
}

// Suppress unused import warning
var _ = fmt.Sprintf
