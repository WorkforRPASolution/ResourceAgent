package sender

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}

	err := s.Send(context.Background(), data)
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

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}

	s.Send(context.Background(), data)

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

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}

	err := s.Send(context.Background(), data)
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

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}

	err := s.Send(context.Background(), data)
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
		{Type: "cpu", Timestamp: time.Now(), AgentID: "a", Hostname: "h", Data: map[string]interface{}{"usage": 1.0}},
		{Type: "memory", Timestamp: time.Now(), AgentID: "a", Hostname: "h", Data: map[string]interface{}{"usage": 2.0}},
		{Type: "disk", Timestamp: time.Now(), AgentID: "a", Hostname: "h", Data: map[string]interface{}{"usage": 3.0}},
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
	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}
	err = s.Send(context.Background(), data)
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

	data := &collector.MetricData{
		Type:      "cpu",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      map[string]interface{}{"usage": 50.0},
	}

	err := s.Send(ctx, data)
	if err == nil {
		t.Fatal("expected error on context cancellation, got nil")
	}
}
