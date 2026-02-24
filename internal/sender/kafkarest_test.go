package sender

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

func newTestKafkaRestSender(t *testing.T, handler http.HandlerFunc) (*KafkaRestSender, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	eqpInfo := &config.EqpInfoConfig{
		Process:  "PROCESS1",
		EqpModel: "MODEL1",
		EqpID:    "EQP001",
		Line:     "LINE1",
		LineDesc: "Desc",
		Index:    "42",
	}

	s, err := NewKafkaRestSender(server.URL, "tp_all_all_resource", eqpInfo, config.SOCKSConfig{})
	if err != nil {
		t.Fatalf("failed to create KafkaRestSender: %v", err)
	}

	return s, server
}

func newCPUData() *collector.MetricData {
	return &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      collector.CPUData{UsagePercent: 50.0, CoreCount: 4},
	}
}

func TestKafkaRestSender_Send_Success(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	err := s.Send(context.Background(), newCPUData())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Content-Type
	if receivedContentType != "application/vnd.kafka.json.v2+json" {
		t.Errorf("expected Content-Type=application/vnd.kafka.json.v2+json, got %q", receivedContentType)
	}

	// Verify body is valid KafkaMessageWrapper2
	var wrapper KafkaMessageWrapper2
	if err := json.Unmarshal(receivedBody, &wrapper); err != nil {
		t.Fatalf("received body is not valid KafkaMessageWrapper2: %v", err)
	}
	if len(wrapper.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(wrapper.Records))
	}
	if wrapper.Records[0].Key != "EQP001" {
		t.Errorf("expected key=EQP001, got %s", wrapper.Records[0].Key)
	}
	if wrapper.Records[0].Value.Process != "PROCESS1" {
		t.Errorf("expected process=PROCESS1, got %s", wrapper.Records[0].Value.Process)
	}
}

func TestKafkaRestSender_Send_TopicInURL(t *testing.T) {
	var receivedPath string

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	s.Send(context.Background(), newCPUData())

	if receivedPath != "/topics/tp_all_all_resource" {
		t.Errorf("expected path=/topics/tp_all_all_resource, got %s", receivedPath)
	}
}

func TestKafkaRestSender_Send_RetryOnFailure(t *testing.T) {
	var attempts int32

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&attempts, 1)
		if count <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	err := s.Send(context.Background(), newCPUData())
	if err != nil {
		t.Fatalf("expected success after retry, got error: %v", err)
	}

	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", atomic.LoadInt32(&attempts))
	}
}

func TestKafkaRestSender_Send_MaxRetriesExhausted(t *testing.T) {
	var attempts int32

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&attempts, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	defer server.Close()
	defer s.Close()

	err := s.Send(context.Background(), newCPUData())
	if err == nil {
		t.Fatal("expected error after max retries, got nil")
	}

	// 1 initial + 2 retries = 3 total attempts
	if atomic.LoadInt32(&attempts) != 3 {
		t.Errorf("expected 3 total attempts, got %d", atomic.LoadInt32(&attempts))
	}
}

func TestKafkaRestSender_SendBatch(t *testing.T) {
	var requests int32

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	batch := []*collector.MetricData{
		{Type: "cpu", Timestamp: time.Now(), AgentID: "a", Hostname: "h",
			Data: collector.CPUData{UsagePercent: 50.0, CoreCount: 4}},
		{Type: "cpu", Timestamp: time.Now(), AgentID: "a", Hostname: "h",
			Data: collector.CPUData{UsagePercent: 60.0, CoreCount: 4}},
		{Type: "cpu", Timestamp: time.Now(), AgentID: "a", Hostname: "h",
			Data: collector.CPUData{UsagePercent: 70.0, CoreCount: 4}},
	}

	err := s.SendBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if atomic.LoadInt32(&requests) != 3 {
		t.Errorf("expected 3 requests for 3 metrics, got %d", atomic.LoadInt32(&requests))
	}
}

func TestKafkaRestSender_Close(t *testing.T) {
	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	err := s.Close()
	if err != nil {
		t.Fatalf("unexpected error on Close: %v", err)
	}

	// Send after close should fail
	err = s.Send(context.Background(), newCPUData())
	if err == nil {
		t.Fatal("expected error when sending after Close, got nil")
	}
}

func TestKafkaRestSender_ContextCancelled(t *testing.T) {
	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := s.Send(ctx, newCPUData())
	if err == nil {
		t.Fatal("expected error on context cancellation, got nil")
	}
}

// --- Legacy format verification tests ---

func TestKafkaRestSender_Send_LegacyPlainTextRaw(t *testing.T) {
	var receivedBody []byte

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Date(2026, 2, 24, 10, 30, 45, 123000000, time.UTC),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      collector.CPUData{UsagePercent: 45.5, CoreCount: 4},
	}

	s.Send(context.Background(), data)

	var wrapper KafkaMessageWrapper2
	json.Unmarshal(receivedBody, &wrapper)

	raw := wrapper.Records[0].Value.Raw
	// raw must be plain text (Grok format), not JSON
	if strings.HasPrefix(raw, "{") {
		t.Errorf("raw should be plain text, not JSON: %q", raw)
	}
	expected := "2026-02-24 10:30:45,123 category:cpu,pid:0,proc:@system,metric:total_used_pct,value:45.5"
	if raw != expected {
		t.Errorf("raw mismatch:\n  expected: %s\n  got:      %s", expected, raw)
	}

	// diff should be 0
	if wrapper.Records[0].Value.Diff != 0 {
		t.Errorf("diff should be 0, got %d", wrapper.Records[0].Value.Diff)
	}
}

func TestKafkaRestSender_Send_MemoryMultipleRecords(t *testing.T) {
	var receivedBody []byte

	s, server := newTestKafkaRestSender(t, func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	data := &collector.MetricData{
		Type:      "memory",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      collector.MemoryData{UsagePercent: 75.0, TotalBytes: 16000000000},
	}

	s.Send(context.Background(), data)

	var wrapper KafkaMessageWrapper2
	json.Unmarshal(receivedBody, &wrapper)

	// Memory should produce 3 records
	if len(wrapper.Records) != 3 {
		t.Fatalf("expected 3 records for memory, got %d", len(wrapper.Records))
	}

	// All records should have the same key and process
	for i, rec := range wrapper.Records {
		if rec.Key != "EQP001" {
			t.Errorf("record[%d] key: expected EQP001, got %s", i, rec.Key)
		}
		if rec.Value.Process != "PROCESS1" {
			t.Errorf("record[%d] process: expected PROCESS1, got %s", i, rec.Value.Process)
		}
		// Each raw should be plain text
		if strings.HasPrefix(rec.Value.Raw, "{") {
			t.Errorf("record[%d] raw should be plain text: %q", i, rec.Value.Raw)
		}
	}
}
