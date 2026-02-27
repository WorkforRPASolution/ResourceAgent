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
		Data:      collector.MemoryData{UsagePercent: 75.0, TotalBytes: 16000000000, UsedBytes: 12000000000},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "memory", 0, "@system", "total_used_pct", 75.0)
	assertRow(t, rows[1], "memory", 0, "@system", "total_free_pct", 25.0)
	assertRow(t, rows[2], "memory", 0, "@system", "total_used_size", 12000000000)
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
				{Name: "Ethernet", BytesRecv: 1000, BytesSent: 2000, BytesRecvRate: 500.5, BytesSentRate: 250.3},
				{Name: "Wi-Fi", BytesRecv: 3000, BytesSent: 4000, BytesRecvRate: 100.0, BytesSentRate: 50.0},
			},
			TCPInboundCount:  42,
			TCPOutboundCount: 38,
		},
	}
	rows := ConvertToEARSRows(data)
	// 2 (all_inbound/all_outbound) + 2 interfaces * 2 (recv_rate/sent_rate) = 6 rows
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "network", 0, "@system", "all_inbound", 42)
	assertRow(t, rows[1], "network", 0, "@system", "all_outbound", 38)
	// Interface-level rate metrics with proc=NIC name
	assertRow(t, rows[2], "network", 0, "Ethernet", "recv_rate", 500.5)
	assertRow(t, rows[3], "network", 0, "Ethernet", "sent_rate", 250.3)
	assertRow(t, rows[4], "network", 0, "Wi-Fi", "recv_rate", 100.0)
	assertRow(t, rows[5], "network", 0, "Wi-Fi", "sent_rate", 50.0)
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
	fanSpeed := 1800.0
	power := 320.5
	coreClock := 2520.0
	memClock := 1200.0

	data := &collector.MetricData{
		Type:      "gpu",
		Timestamp: testTimestamp,
		Data: collector.GpuData{
			Gpus: []collector.GpuSensor{
				{
					Name: "RTX4090", Temperature: &temp, CoreLoad: &coreLoad, MemoryLoad: &memLoad,
					FanSpeed: &fanSpeed, Power: &power, CoreClock: &coreClock, MemoryClock: &memClock,
				},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 7 {
		t.Fatalf("expected 7 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "gpu", 0, "@system", "RTX4090_temperature", 75.0)
	assertRow(t, rows[1], "gpu", 0, "@system", "RTX4090_core_load", 90.0)
	assertRow(t, rows[2], "gpu", 0, "@system", "RTX4090_memory_load", 60.0)
	assertRow(t, rows[3], "gpu", 0, "@system", "RTX4090_fan_speed", 1800.0)
	assertRow(t, rows[4], "gpu", 0, "@system", "RTX4090_power", 320.5)
	assertRow(t, rows[5], "gpu", 0, "@system", "RTX4090_core_clock", 2520.0)
	assertRow(t, rows[6], "gpu", 0, "@system", "RTX4090_memory_clock", 1200.0)
}

func TestConvertToEARSRows_GPU_NilFields(t *testing.T) {
	temp := 75.0
	data := &collector.MetricData{
		Type:      "gpu",
		Timestamp: testTimestamp,
		Data: collector.GpuData{
			Gpus: []collector.GpuSensor{
				{Name: "RTX4090", Temperature: &temp},
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
	remainLife := 98.0
	mediaErr := int64(0)
	powerCycles := int64(150)
	unsafeShut := int64(3)
	powerOnHrs := int64(8760)
	totalWritten := int64(50000000000)

	data := &collector.MetricData{
		Type:      "storage_smart",
		Timestamp: testTimestamp,
		Data: collector.StorageSmartData{
			Storages: []collector.StorageSmartSensor{
				{
					Name: "nvme0", Type: "NVMe", Temperature: &temp, RemainingLife: &remainLife,
					MediaErrors: &mediaErr, PowerCycles: &powerCycles, UnsafeShutdowns: &unsafeShut,
					PowerOnHours: &powerOnHrs, TotalBytesWritten: &totalWritten,
				},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 7 {
		t.Fatalf("expected 7 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "storage_smart", 0, "@system", "nvme0_temperature", 35.0)
	assertRow(t, rows[1], "storage_smart", 0, "@system", "nvme0_remaining_life", 98.0)
	assertRow(t, rows[2], "storage_smart", 0, "@system", "nvme0_media_errors", 0)
	assertRow(t, rows[3], "storage_smart", 0, "@system", "nvme0_power_cycles", 150)
	assertRow(t, rows[4], "storage_smart", 0, "@system", "nvme0_unsafe_shutdowns", 3)
	assertRow(t, rows[5], "storage_smart", 0, "@system", "nvme0_power_on_hours", 8760)
	assertRow(t, rows[6], "storage_smart", 0, "@system", "nvme0_total_bytes_written", 50000000000)
}

func TestConvertToEARSRows_StorageSmart_NilFields(t *testing.T) {
	data := &collector.MetricData{
		Type:      "storage_smart",
		Timestamp: testTimestamp,
		Data: collector.StorageSmartData{
			Storages: []collector.StorageSmartSensor{
				{Name: "nvme0"},
			},
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for all-nil fields, got %d", len(rows))
	}
}

func TestConvertToEARSRows_Uptime(t *testing.T) {
	data := &collector.MetricData{
		Type:      "uptime",
		Timestamp: testTimestamp,
		Data: collector.UptimeData{
			BootTimeUnix:  1740614400,
			BootTimeStr:   "2025-02-27T09:00:00",
			UptimeMinutes: 1440.5,
		},
	}
	rows := ConvertToEARSRows(data)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	assertRow(t, rows[0], "uptime", 0, "@system", "boot_time_unix", 1740614400)
	assertRow(t, rows[1], "uptime", 0, "@system", "uptime_minutes", 1440.5)
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
		{Timestamp: testTimestamp, Category: "memory", PID: 0, ProcName: "@system", Metric: "total_used_size", Value: 12000000000},
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
		Value:     12000000000,
	}
	result := row.ToLegacyString()
	// Extract the value portion and verify no scientific notation
	valueStr := formatValue(12000000000)
	if regexp.MustCompile(`[eE]`).MatchString(valueStr) {
		t.Errorf("value should not use scientific notation: %q", valueStr)
	}
	expected := "2026-02-24 10:30:45,123 category:memory,pid:0,proc:@system,metric:total_used_size,value:12000000000"
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

// --- sanitizeName ---

func TestSanitizeName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		// Parentheses removed
		{"Intel(R) HD Graphics 530_power", "IntelR_HD_Graphics_530_power"},
		// Spaces → _, hash removed
		{"CPU Core #1 Distance to TjMax", "CPU_Core_1_Distance_to_TjMax"},
		// Spaces → _
		{"Intel Core i7-6700 - Core Max", "Intel_Core_i7-6700_-_Core_Max"},
		// Spaces → _
		{"Samsung SSD 860 PRO 256GB_temperature", "Samsung_SSD_860_PRO_256GB_temperature"},
		// Drive letter unchanged
		{"C:", "C:"},
		// Already clean name unchanged
		{"total_used_pct", "total_used_pct"},
		// @ preserved (used in @system)
		{"@system", "@system"},
		// Dot preserved
		{"python3.11", "python3.11"},
		// Consecutive special chars → single _
		{"Fan  ##2", "Fan_2"},
		// Empty string
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeName(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestToLegacyString_SpecialCharsInMetric(t *testing.T) {
	grokPattern := regexp.MustCompile(
		`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3}) category:(\w+),pid:(\d+),proc:([^,]+),metric:([^,]+),value:(.+)$`,
	)

	tests := []struct {
		name           string
		row            EARSRow
		expectedMetric string
		expectedProc   string
	}{
		{
			name: "GPU with parentheses and spaces",
			row: EARSRow{
				Timestamp: testTimestamp, Category: "gpu", PID: 0,
				ProcName: "@system", Metric: "Intel(R) HD Graphics 530_power",
			},
			expectedMetric: "IntelR_HD_Graphics_530_power",
			expectedProc:   "@system",
		},
		{
			name: "Temperature sensor with hash",
			row: EARSRow{
				Timestamp: testTimestamp, Category: "temperature", PID: 0,
				ProcName: "@system", Metric: "CPU Core #1 Distance to TjMax",
			},
			expectedMetric: "CPU_Core_1_Distance_to_TjMax",
			expectedProc:   "@system",
		},
		{
			name: "Network interface with spaces",
			row: EARSRow{
				Timestamp: testTimestamp, Category: "network", PID: 0,
				ProcName: "Loopback Pseudo-Interface 1", Metric: "recv_rate", Value: 100.0,
			},
			expectedMetric: "recv_rate",
			expectedProc:   "Loopback_Pseudo-Interface_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := tt.row.ToLegacyString()
			if !grokPattern.MatchString(s) {
				t.Errorf("Grok pattern does not match: %q", s)
			}
			matches := grokPattern.FindStringSubmatch(s)
			if matches == nil {
				t.Fatalf("no regex match for %q", s)
			}
			proc := matches[4]
			metric := matches[5]
			if metric != tt.expectedMetric {
				t.Errorf("metric: expected %q, got %q", tt.expectedMetric, metric)
			}
			if proc != tt.expectedProc {
				t.Errorf("proc: expected %q, got %q", tt.expectedProc, proc)
			}
		})
	}
}

func TestConvertToEARSRows_ProcessWatch(t *testing.T) {
	data := &collector.MetricData{
		Type:      "ProcessWatch",
		Timestamp: testTimestamp,
		Data: collector.ProcessWatchData{
			Statuses: []collector.ProcessWatchStatus{
				{Name: "mes.exe", PID: 1234, Running: true, Type: "required"},
				{Name: "scada.exe", PID: 0, Running: false, Type: "required"},
				{Name: "torrent.exe", PID: 5678, Running: true, Type: "forbidden"},
				{Name: "teamviewer.exe", PID: 0, Running: false, Type: "forbidden"},
			},
		},
	}

	rows := ConvertToEARSRows(data)
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(rows))
	}

	assertRow(t, rows[0], "process_watch", 1234, "mes.exe", "required", 1)
	assertRow(t, rows[1], "process_watch", 0, "scada.exe", "required_alert", 0)
	assertRow(t, rows[2], "process_watch", 5678, "torrent.exe", "forbidden_alert", 1)
	assertRow(t, rows[3], "process_watch", 0, "teamviewer.exe", "forbidden", 0)
}

func TestConvertToEARSRows_ProcessWatch_Empty(t *testing.T) {
	data := &collector.MetricData{
		Type:      "ProcessWatch",
		Timestamp: testTimestamp,
		Data:      collector.ProcessWatchData{Statuses: []collector.ProcessWatchStatus{}},
	}

	rows := ConvertToEARSRows(data)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows, got %d", len(rows))
	}
}

func TestConvertToEARSRows_ProcessWatch_LegacyString(t *testing.T) {
	data := &collector.MetricData{
		Type:      "ProcessWatch",
		Timestamp: testTimestamp,
		Data: collector.ProcessWatchData{
			Statuses: []collector.ProcessWatchStatus{
				{Name: "mes.exe", PID: 1234, Running: true, Type: "required"},
			},
		},
	}

	rows := ConvertToEARSRows(data)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}

	legacy := rows[0].ToLegacyString()
	expected := "2026-02-24 10:30:45,123 category:process_watch,pid:1234,proc:mes.exe,metric:required,value:1"
	if legacy != expected {
		t.Errorf("ToLegacyString:\n  got:  %q\n  want: %q", legacy, expected)
	}
}

func TestConvertToEARSRows_ProcessWatch_JSONRoundtrip(t *testing.T) {
	data := &collector.MetricData{
		Type:      "ProcessWatch",
		Timestamp: testTimestamp,
		Data: collector.ProcessWatchData{
			Statuses: []collector.ProcessWatchStatus{
				{Name: "mes.exe", PID: 1234, Running: true, Type: "required"},
			},
		},
	}

	// JSON roundtrip: marshal then unmarshal
	b, err := json.Marshal(data.Data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var roundtripped collector.MetricData
	roundtripped.Type = "ProcessWatch"
	roundtripped.Timestamp = testTimestamp
	var pwData collector.ProcessWatchData
	if err := json.Unmarshal(b, &pwData); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	roundtripped.Data = pwData

	rows := ConvertToEARSRows(&roundtripped)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row after roundtrip, got %d", len(rows))
	}
	assertRow(t, rows[0], "process_watch", 1234, "mes.exe", "required", 1)
}

// Suppress unused import warning
var _ = fmt.Sprintf
