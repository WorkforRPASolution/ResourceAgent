package sender

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

func newTestMetricData() *collector.MetricData {
	return &collector.MetricData{
		Type:      "cpu",
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

func TestWrapMetricData_ProducesValidJSON(t *testing.T) {
	data := newTestMetricData()
	eqpInfo := newTestEqpInfo()

	result, err := WrapMetricData(data, eqpInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper KafkaMessageWrapper2
	if err := json.Unmarshal(result, &wrapper); err != nil {
		t.Fatalf("result is not valid JSON: %v", err)
	}

	if len(wrapper.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(wrapper.Records))
	}

	record := wrapper.Records[0]
	if record.Key != "EQP001" {
		t.Errorf("expected key=EQP001, got %s", record.Key)
	}
	if record.Value.Process != "PROCESS1" {
		t.Errorf("expected process=PROCESS1, got %s", record.Value.Process)
	}
}

func TestWrapMetricData_ContainsCorrectEqpInfo(t *testing.T) {
	data := newTestMetricData()
	eqpInfo := newTestEqpInfo()

	result, err := WrapMetricData(data, eqpInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper KafkaMessageWrapper2
	if err := json.Unmarshal(result, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	val := wrapper.Records[0].Value
	if val.Process != "PROCESS1" {
		t.Errorf("process: expected PROCESS1, got %s", val.Process)
	}
	if val.Line != "LINE1" {
		t.Errorf("line: expected LINE1, got %s", val.Line)
	}
	if val.EqpID != "EQP001" {
		t.Errorf("eqpid: expected EQP001, got %s", val.EqpID)
	}
	if val.Model != "MODEL1" {
		t.Errorf("model: expected MODEL1, got %s", val.Model)
	}
}

func TestWrapMetricData_RawContainsOriginalData(t *testing.T) {
	data := newTestMetricData()
	eqpInfo := newTestEqpInfo()

	result, err := WrapMetricData(data, eqpInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper KafkaMessageWrapper2
	if err := json.Unmarshal(result, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	rawStr := wrapper.Records[0].Value.Raw
	if rawStr == "" {
		t.Fatal("raw field is empty")
	}

	// The raw field should be a valid JSON string containing the original MetricData
	var rawData collector.MetricData
	if err := json.Unmarshal([]byte(rawStr), &rawData); err != nil {
		t.Fatalf("raw field is not valid MetricData JSON: %v", err)
	}

	if rawData.Type != "cpu" {
		t.Errorf("raw type: expected cpu, got %s", rawData.Type)
	}
	if rawData.AgentID != "test-agent" {
		t.Errorf("raw agent_id: expected test-agent, got %s", rawData.AgentID)
	}
	if rawData.Hostname != "test-host" {
		t.Errorf("raw hostname: expected test-host, got %s", rawData.Hostname)
	}
}

func TestWrapMetricData_ESIDFormat(t *testing.T) {
	data := newTestMetricData()
	eqpInfo := newTestEqpInfo()

	result, err := WrapMetricData(data, eqpInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper KafkaMessageWrapper2
	if err := json.Unmarshal(result, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	esid := wrapper.Records[0].Value.ESID
	// Expected format: "{eqpid}_{type}_{timestamp}"
	// Timestamp format: 20060102150405
	expectedESID := fmt.Sprintf("EQP001_cpu_%s", data.Timestamp.Format("20060102150405"))
	if esid != expectedESID {
		t.Errorf("ESID: expected %s, got %s", expectedESID, esid)
	}

	// Also verify the format pattern
	parts := strings.Split(esid, "_")
	if len(parts) < 3 {
		t.Errorf("ESID should have at least 3 parts separated by '_', got %d parts: %s", len(parts), esid)
	}
	if parts[0] != "EQP001" {
		t.Errorf("ESID first part should be EqpID, got %s", parts[0])
	}
	if parts[1] != "cpu" {
		t.Errorf("ESID second part should be metric type, got %s", parts[1])
	}
}

func TestWrapMetricData_DiffIsReasonable(t *testing.T) {
	// Use current time so diff should be very small
	data := &collector.MetricData{
		Type:      "memory",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage_percent": 75.0},
	}
	eqpInfo := newTestEqpInfo()

	result, err := WrapMetricData(data, eqpInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var wrapper KafkaMessageWrapper2
	if err := json.Unmarshal(result, &wrapper); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	diff := wrapper.Records[0].Value.Diff
	// Diff should be within 5 seconds (5000 ms) for a just-created timestamp
	if diff < 0 || diff > 5000 {
		t.Errorf("diff should be between 0 and 5000ms, got %d", diff)
	}
}

func TestKafkaMessageWrapper2_JSONStructure(t *testing.T) {
	data := newTestMetricData()
	eqpInfo := newTestEqpInfo()

	result, err := WrapMetricData(data, eqpInfo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Unmarshal to generic map to verify raw JSON structure
	var raw map[string]interface{}
	if err := json.Unmarshal(result, &raw); err != nil {
		t.Fatalf("failed to unmarshal as map: %v", err)
	}

	// Must have "records" key
	records, ok := raw["records"]
	if !ok {
		t.Fatal("JSON must contain 'records' key")
	}

	// "records" must be an array
	recordsArr, ok := records.([]interface{})
	if !ok {
		t.Fatal("'records' must be an array")
	}

	// Must contain exactly one element
	if len(recordsArr) != 1 {
		t.Fatalf("'records' array should have 1 element, got %d", len(recordsArr))
	}

	// Element must have "key" and "value"
	record, ok := recordsArr[0].(map[string]interface{})
	if !ok {
		t.Fatal("record element must be an object")
	}

	if _, ok := record["key"]; !ok {
		t.Error("record must have 'key' field")
	}
	if _, ok := record["value"]; !ok {
		t.Error("record must have 'value' field")
	}

	// Value must have the expected fields
	value, ok := record["value"].(map[string]interface{})
	if !ok {
		t.Fatal("'value' must be an object")
	}

	requiredFields := []string{"process", "line", "eqpid", "model", "diff", "esid", "raw"}
	for _, field := range requiredFields {
		if _, ok := value[field]; !ok {
			t.Errorf("value must have '%s' field", field)
		}
	}
}
