package sender

import (
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

func newTestMetricData() *collector.MetricData {
	return &collector.MetricData{
		Type:      "CPU",
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 0, time.UTC),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}
}

func newTestEqpInfo() *config.EqpInfoConfig {
	return &config.EqpInfoConfig{
		Process:  "PROCESS1",
		EqpModel: "MODEL1",
		EqpID:    "EQP001",
		Line:     "LINE1",
		LineDesc: "Line Description 1",
		Index:    "42",
	}
}

func newTestCPUMetricData() *collector.MetricData {
	return &collector.MetricData{
		Type:      "CPU",
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
	}
}

func newTestMemoryMetricData() *collector.MetricData {
	return &collector.MetricData{
		Type:      "Memory",
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      collector.MemoryData{UsagePercent: 75.0, TotalBytes: 16000000000},
	}
}

// --- RawFormatter tests ---

func TestGrokRawFormatter(t *testing.T) {
	row := EARSRow{
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC),
		Category:  "cpu",
		PID:       0,
		ProcName:  "@system",
		Metric:    "total_used_pct",
		Value:     45.5,
	}

	f := GrokRawFormatter{}
	raw, err := f.FormatRaw(row, "PROCESS1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "2026-02-24 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5"
	if raw != expected {
		t.Errorf("mismatch:\n  expected: %s\n  got:      %s", expected, raw)
	}
}

func TestGrokRawFormatter_IgnoresProcess(t *testing.T) {
	row := EARSRow{
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC),
		Category:  "cpu",
		PID:       0,
		ProcName:  "@system",
		Metric:    "total_used_pct",
		Value:     45.5,
	}

	f := GrokRawFormatter{}
	raw1, _ := f.FormatRaw(row, "PROCESS1")
	raw2, _ := f.FormatRaw(row, "DIFFERENT")

	if raw1 != raw2 {
		t.Errorf("GrokRawFormatter should not use process parameter:\n  raw1: %s\n  raw2: %s", raw1, raw2)
	}
}

func TestJSONRawFormatter(t *testing.T) {
	row := EARSRow{
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC),
		Category:  "cpu",
		PID:       0,
		ProcName:  "@system",
		Metric:    "total_used_pct",
		Value:     45.5,
	}

	f := JSONRawFormatter{}
	raw, err := f.FormatRaw(row, "PROCESS1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var pdl ParsedDataList
	if err := json.Unmarshal([]byte(raw), &pdl); err != nil {
		t.Fatalf("raw is not valid ParsedDataList JSON: %v", err)
	}

	if pdl.ISOTimestamp != "2026-02-24T10:30:45.123" {
		t.Errorf("timestamp: expected 2026-02-24T10:30:45.123, got %s", pdl.ISOTimestamp)
	}

	if len(pdl.Parsed) != 6 {
		t.Fatalf("expected 6 ParsedData entries, got %d", len(pdl.Parsed))
	}

	// Verify EARS_PROCESS is in ParsedDataList
	found := false
	for _, d := range pdl.Parsed {
		if d.Field == "EARS_PROCESS" && d.Value == "PROCESS1" && d.DataFormat == "String" {
			found = true
		}
	}
	if !found {
		t.Error("EARS_PROCESS not found in ParsedDataList")
	}
}

// --- PrepareRecords tests ---

func TestPrepareRecords_GrokFormat(t *testing.T) {
	data := newTestCPUMetricData()
	eqpInfo := newTestEqpInfo()

	records, err := PrepareRecords(data, eqpInfo, 0, GrokRawFormatter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.Key != "EQP001" {
		t.Errorf("key: expected EQP001, got %s", rec.Key)
	}
	if rec.Value.Process != "PROCESS1" {
		t.Errorf("process: expected PROCESS1, got %s", rec.Value.Process)
	}
	if rec.Value.Line != "LINE1" {
		t.Errorf("line: expected LINE1, got %s", rec.Value.Line)
	}
	if rec.Value.EqpID != "EQP001" {
		t.Errorf("eqpid: expected EQP001, got %s", rec.Value.EqpID)
	}
	if rec.Value.Model != "MODEL1" {
		t.Errorf("model: expected MODEL1, got %s", rec.Value.Model)
	}

	expected := "2026-02-24 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5"
	if rec.Value.Raw != expected {
		t.Errorf("raw mismatch:\n  expected: %s\n  got:      %s", expected, rec.Value.Raw)
	}
}

func TestPrepareRecords_JSONFormat(t *testing.T) {
	data := newTestCPUMetricData()
	eqpInfo := newTestEqpInfo()

	records, err := PrepareRecords(data, eqpInfo, 0, JSONRawFormatter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(records))
	}

	rec := records[0]
	if rec.Value.Process != "PROCESS1" {
		t.Errorf("process: expected PROCESS1, got %s", rec.Value.Process)
	}

	// raw should be a valid ParsedDataList JSON
	var pdl ParsedDataList
	if err := json.Unmarshal([]byte(rec.Value.Raw), &pdl); err != nil {
		t.Fatalf("raw is not valid ParsedDataList JSON: %v", err)
	}

	if pdl.ISOTimestamp != "2026-02-24T10:30:45.123" {
		t.Errorf("iso_timestamp: expected 2026-02-24T10:30:45.123, got %s", pdl.ISOTimestamp)
	}

	// EARS_PROCESS should be in ParsedDataList
	found := false
	for _, d := range pdl.Parsed {
		if d.Field == "EARS_PROCESS" && d.Value == "PROCESS1" {
			found = true
		}
	}
	if !found {
		t.Error("EARS_PROCESS not found in ParsedDataList")
	}
}

func TestPrepareRecords_MultipleRows(t *testing.T) {
	data := newTestMemoryMetricData()
	eqpInfo := newTestEqpInfo()

	records, err := PrepareRecords(data, eqpInfo, 0, GrokRawFormatter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Memory produces 3 rows
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	// All records should share the same key
	for i, rec := range records {
		if rec.Key != "EQP001" {
			t.Errorf("record[%d] key: expected EQP001, got %s", i, rec.Key)
		}
		if rec.Value.Process != "PROCESS1" {
			t.Errorf("record[%d] process: expected PROCESS1, got %s", i, rec.Value.Process)
		}
	}
}

func TestPrepareRecords_ESIDFormat(t *testing.T) {
	data := newTestMemoryMetricData()
	eqpInfo := newTestEqpInfo()

	records, err := PrepareRecords(data, eqpInfo, 0, GrokRawFormatter{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tsMs := data.Timestamp.UnixMilli()
	for i, rec := range records {
		expected := fmt.Sprintf("PROCESS1:EQP001-Memory-%d-%d", tsMs, i)
		if rec.Value.ESID != expected {
			t.Errorf("record[%d] ESID: expected %s, got %s", i, expected, rec.Value.ESID)
		}
	}
}

func TestPrepareRecords_DiffZero(t *testing.T) {
	data := newTestCPUMetricData()
	eqpInfo := newTestEqpInfo()

	records, _ := PrepareRecords(data, eqpInfo, 0, GrokRawFormatter{})
	if records[0].Value.Diff != 0 {
		t.Errorf("diff should be 0, got %d", records[0].Value.Diff)
	}
}

func TestPrepareRecords_DiffPassedThrough(t *testing.T) {
	data := newTestCPUMetricData()
	eqpInfo := newTestEqpInfo()

	records, _ := PrepareRecords(data, eqpInfo, 1234, GrokRawFormatter{})
	if records[0].Value.Diff != 1234 {
		t.Errorf("diff should be 1234, got %d", records[0].Value.Diff)
	}
}

func TestPrepareRecords_NegativeDiff(t *testing.T) {
	data := newTestCPUMetricData()
	eqpInfo := newTestEqpInfo()

	records, _ := PrepareRecords(data, eqpInfo, -5678, JSONRawFormatter{})
	if records[0].Value.Diff != -5678 {
		t.Errorf("diff should be -5678, got %d", records[0].Value.Diff)
	}
}

func TestPrepareRecords_ErrNoRows(t *testing.T) {
	data := &collector.MetricData{
		Type:      "Unknown",
		Timestamp: time.Now(),
		Data:      nil,
	}
	eqpInfo := newTestEqpInfo()

	_, err := PrepareRecords(data, eqpInfo, 0, GrokRawFormatter{})
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
	if !errors.Is(err, ErrNoRows) {
		t.Errorf("expected ErrNoRows, got %v", err)
	}
}

func TestPrepareRecords_TimestampPreserved(t *testing.T) {
	data := newTestCPUMetricData()
	eqpInfo := newTestEqpInfo()

	records, _ := PrepareRecords(data, eqpInfo, 0, GrokRawFormatter{})
	if !records[0].Timestamp.Equal(data.Timestamp) {
		t.Errorf("timestamp: expected %v, got %v", data.Timestamp, records[0].Timestamp)
	}
}

// --- generateESID tests ---

func TestGenerateESID(t *testing.T) {
	esid := generateESID("ARSAgent", "EQP001", "cpu", 1708593045123, 0)
	expected := "ARSAgent:EQP001-cpu-1708593045123-0"
	if esid != expected {
		t.Errorf("expected %s, got %s", expected, esid)
	}

	esid2 := generateESID("ARSAgent", "EQP001", "memory", 1708593045123, 2)
	expected2 := "ARSAgent:EQP001-memory-1708593045123-2"
	if esid2 != expected2 {
		t.Errorf("expected %s, got %s", expected2, esid2)
	}
}

func TestGenerateESID_NoDuplicateAcrossTypes(t *testing.T) {
	ts := int64(1708593045123)
	esidCPU := generateESID("PROC", "EQP01", "cpu", ts, 0)
	esidMem := generateESID("PROC", "EQP01", "memory", ts, 0)
	if esidCPU == esidMem {
		t.Errorf("ESID should differ across metric types, both got %s", esidCPU)
	}
}

// --- KafkaValue JSON structure test ---

func TestKafkaValue_JSONFields(t *testing.T) {
	kv := KafkaValue{
		Process: "PROCESS1",
		Line:    "LINE1",
		EqpID:   "EQP001",
		Model:   "MODEL1",
		Diff:    42,
		ESID:    "test-esid",
		Raw:     "test-raw",
	}

	b, err := json.Marshal(kv)
	if err != nil {
		t.Fatalf("failed to marshal KafkaValue: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(b, &raw)

	requiredFields := []string{"process", "line", "eqpid", "model", "diff", "esid", "raw"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("KafkaValue JSON must have '%s' field", field)
		}
	}
}

func TestKafkaMessageWrapper2_JSONStructure(t *testing.T) {
	wrapper := KafkaMessageWrapper2{
		Records: []KafkaMessage2{
			{
				Key: "EQP001",
				Value: KafkaValue{
					Process: "PROCESS1",
					Line:    "LINE1",
					EqpID:   "EQP001",
					Model:   "MODEL1",
					Diff:    0,
					ESID:    "test",
					Raw:     "test",
				},
			},
		},
	}

	b, err := json.Marshal(wrapper)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]interface{}
	json.Unmarshal(b, &raw)

	records, ok := raw["records"]
	if !ok {
		t.Fatal("JSON must contain 'records' key")
	}

	recordsArr, ok := records.([]interface{})
	if !ok {
		t.Fatal("'records' must be an array")
	}
	if len(recordsArr) != 1 {
		t.Fatalf("expected 1 record, got %d", len(recordsArr))
	}

	record := recordsArr[0].(map[string]interface{})
	if _, ok := record["key"]; !ok {
		t.Error("record must have 'key' field")
	}

	value := record["value"].(map[string]interface{})
	requiredFields := []string{"process", "line", "eqpid", "model", "diff", "esid", "raw"}
	for _, field := range requiredFields {
		if _, ok := value[field]; !ok {
			t.Errorf("value must have '%s' field", field)
		}
	}
}
