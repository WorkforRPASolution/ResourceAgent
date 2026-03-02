package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

func newTestHTTPSender(t *testing.T, handler http.HandlerFunc) (*KafkaSender, *httptest.Server) {
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

	transport, err := NewHTTPTransport(server.URL, config.SOCKSConfig{})
	if err != nil {
		t.Fatalf("failed to create HTTPTransport: %v", err)
	}

	s := NewKafkaSender(transport, "tp_all_all_resource", eqpInfo, func() int64 { return 0 }, GrokRawFormatter{})
	return s, server
}

func newCPUData() *collector.MetricData {
	return &collector.MetricData{
		Type:      "CPU",
		Timestamp: time.Now(),
		AgentID:   "test-agent",
		Hostname:  "test-host",
		Data:      collector.CPUData{UsagePercent: 50.0, CoreCount: 4},
	}
}

func TestHTTPTransport_Send_Success(t *testing.T) {
	var receivedBody []byte
	var receivedContentType string

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestHTTPTransport_Send_TopicInURL(t *testing.T) {
	var receivedPath string

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestHTTPTransport_Send_RetryOnFailure(t *testing.T) {
	var attempts int32

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestHTTPTransport_Send_MaxRetriesExhausted(t *testing.T) {
	var attempts int32

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestHTTPTransport_SendBatch(t *testing.T) {
	var requests int32
	var totalRecords int32

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&requests, 1)
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		atomic.AddInt32(&totalRecords, int32(len(wrapper.Records)))
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	batch := []*collector.MetricData{
		{Type: "CPU", Timestamp: time.Now(), AgentID: "a", Hostname: "h",
			Data: collector.CPUData{UsagePercent: 50.0, CoreCount: 4}},
		{Type: "CPU", Timestamp: time.Now(), AgentID: "a", Hostname: "h",
			Data: collector.CPUData{UsagePercent: 60.0, CoreCount: 4}},
		{Type: "CPU", Timestamp: time.Now(), AgentID: "a", Hostname: "h",
			Data: collector.CPUData{UsagePercent: 70.0, CoreCount: 4}},
	}

	err := s.SendBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// SendBatch now aggregates all records into 1 Deliver call
	if atomic.LoadInt32(&requests) != 1 {
		t.Errorf("expected 1 request (aggregated), got %d", atomic.LoadInt32(&requests))
	}
	if atomic.LoadInt32(&totalRecords) != 3 {
		t.Errorf("expected 3 total records, got %d", atomic.LoadInt32(&totalRecords))
	}
}

func TestHTTPTransport_Close(t *testing.T) {
	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
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

func TestHTTPTransport_ContextCancelled(t *testing.T) {
	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
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

// --- Grok format verification tests ---

func TestHTTPTransport_Send_GrokPlainTextRaw(t *testing.T) {
	var receivedBody []byte

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	data := &collector.MetricData{
		Type:      "CPU",
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

func TestHTTPTransport_Send_MemoryMultipleRecords(t *testing.T) {
	var receivedBody []byte

	s, server := newTestHTTPSender(t, func(w http.ResponseWriter, r *http.Request) {
		receivedBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()
	defer s.Close()

	data := &collector.MetricData{
		Type:      "Memory",
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

// --- BufferedHTTPTransport helpers ---

func newTestBatchConfig() config.BatchConfig {
	return config.BatchConfig{
		FlushFrequency: 100 * time.Millisecond,
		FlushMessages:  100,
		MaxBatchSize:   500,
		MaxRetries:     2,
		RetryBackoff:   10 * time.Millisecond,
	}
}

func newTestBufferedTransport(t *testing.T, handler http.HandlerFunc, batchCfg config.BatchConfig) (*BufferedHTTPTransport, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	transport, err := NewBufferedHTTPTransport(server.URL, config.SOCKSConfig{}, batchCfg)
	if err != nil {
		t.Fatalf("failed to create BufferedHTTPTransport: %v", err)
	}
	return transport, server
}

func makeTestRecords(n int) []KafkaRecord {
	records := make([]KafkaRecord, n)
	for i := range records {
		records[i] = KafkaRecord{
			Key: fmt.Sprintf("KEY%d", i),
			Value: KafkaValue{
				Process: "PROC", Line: "LINE", EqpID: "EQP",
				Model: "MODEL", Diff: 0, ESID: fmt.Sprintf("esid-%d", i),
				Raw: fmt.Sprintf("raw-%d", i),
			},
			Timestamp: time.Now(),
		}
	}
	return records
}

// T1: 타이머 flush
func TestBufferedHTTPTransport_TimerFlush(t *testing.T) {
	var mu sync.Mutex
	var receivedRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 100 * time.Millisecond
	batchCfg.FlushMessages = 1000

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		receivedRecords += len(wrapper.Records)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	transport.Deliver(context.Background(), "test-topic", makeTestRecords(3))

	mu.Lock()
	immediate := receivedRecords
	mu.Unlock()
	if immediate != 0 {
		t.Errorf("expected 0 records immediately after Deliver, got %d", immediate)
	}

	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedRecords != 3 {
		t.Errorf("expected 3 records after timer flush, got %d", receivedRecords)
	}

	transport.Close()
}

// T2: 메시지 수 임계값 flush
func TestBufferedHTTPTransport_FlushOnMessageCount(t *testing.T) {
	var mu sync.Mutex
	var receivedRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 10 * time.Second
	batchCfg.FlushMessages = 3

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		receivedRecords += len(wrapper.Records)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	transport.Deliver(context.Background(), "test-topic", makeTestRecords(1))
	transport.Deliver(context.Background(), "test-topic", makeTestRecords(1))
	transport.Deliver(context.Background(), "test-topic", makeTestRecords(1))

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if receivedRecords != 3 {
		t.Errorf("expected 3 records after threshold flush, got %d", receivedRecords)
	}

	transport.Close()
}

// T3: 여러 Deliver()가 하나의 HTTP POST로 배치됨
func TestBufferedHTTPTransport_BatchesMultipleDelivers(t *testing.T) {
	var mu sync.Mutex
	var httpPostCount int32
	var totalRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 100 * time.Millisecond
	batchCfg.FlushMessages = 1000

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		httpPostCount++
		totalRecords += len(wrapper.Records)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	for i := 0; i < 5; i++ {
		transport.Deliver(context.Background(), "test-topic", makeTestRecords(1))
	}

	time.Sleep(250 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if httpPostCount != 1 {
		t.Errorf("expected 1 HTTP POST (batched), got %d", httpPostCount)
	}
	if totalRecords != 5 {
		t.Errorf("expected 5 total records, got %d", totalRecords)
	}

	transport.Close()
}

// T4: Close()가 잔여 버퍼를 flush
func TestBufferedHTTPTransport_CloseFlushesBuffer(t *testing.T) {
	var mu sync.Mutex
	var totalRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 10 * time.Second
	batchCfg.FlushMessages = 1000

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		totalRecords += len(wrapper.Records)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	transport.Deliver(context.Background(), "test-topic", makeTestRecords(5))

	time.Sleep(50 * time.Millisecond)
	mu.Lock()
	before := totalRecords
	mu.Unlock()
	if before != 0 {
		t.Errorf("expected 0 records before Close, got %d", before)
	}

	transport.Close()

	mu.Lock()
	defer mu.Unlock()
	if totalRecords != 5 {
		t.Errorf("expected 5 records after Close, got %d", totalRecords)
	}
}

// T5: MaxBatchSize로 분할
func TestBufferedHTTPTransport_MaxBatchSizeSplits(t *testing.T) {
	var mu sync.Mutex
	var httpPostCount int32
	var totalRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 10 * time.Second
	batchCfg.FlushMessages = 1000
	batchCfg.MaxBatchSize = 3

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		httpPostCount++
		totalRecords += len(wrapper.Records)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	transport.Deliver(context.Background(), "test-topic", makeTestRecords(10))
	transport.Close()

	mu.Lock()
	defer mu.Unlock()
	if httpPostCount != 4 {
		t.Errorf("expected 4 HTTP POSTs, got %d", httpPostCount)
	}
	if totalRecords != 10 {
		t.Errorf("expected 10 total records, got %d", totalRecords)
	}
}

// T6: 동시성 안전
func TestBufferedHTTPTransport_ConcurrentDeliver(t *testing.T) {
	var mu sync.Mutex
	var totalRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 50 * time.Millisecond

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		totalRecords += len(wrapper.Records)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	var wg sync.WaitGroup
	for g := 0; g < 10; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				transport.Deliver(context.Background(), "test-topic", makeTestRecords(1))
			}
		}()
	}
	wg.Wait()
	transport.Close()

	mu.Lock()
	defer mu.Unlock()
	if totalRecords != 1000 {
		t.Errorf("expected 1000 total records, got %d", totalRecords)
	}
}

// T7: Deliver()는 nil을 즉시 리턴
func TestBufferedHTTPTransport_DeliverReturnsNil(t *testing.T) {
	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 10 * time.Second

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()
	defer transport.Close()

	err := transport.Deliver(context.Background(), "test-topic", makeTestRecords(1))
	if err != nil {
		t.Errorf("expected nil error from Deliver, got %v", err)
	}
}

// T8: HTTP 에러 시 다음 배치에 영향 없음
func TestBufferedHTTPTransport_ErrorDoesNotBlockNextBatch(t *testing.T) {
	var mu sync.Mutex
	var callCount int
	var successRecords int

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 50 * time.Millisecond
	batchCfg.MaxRetries = 0

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var wrapper KafkaMessageWrapper2
		json.Unmarshal(body, &wrapper)
		mu.Lock()
		callCount++
		c := callCount
		mu.Unlock()

		if c == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
		} else {
			mu.Lock()
			successRecords += len(wrapper.Records)
			mu.Unlock()
			w.WriteHeader(http.StatusOK)
		}
	}, batchCfg)
	defer server.Close()

	transport.Deliver(context.Background(), "test-topic", makeTestRecords(2))
	time.Sleep(100 * time.Millisecond)

	transport.Deliver(context.Background(), "test-topic", makeTestRecords(3))
	time.Sleep(100 * time.Millisecond)

	transport.Close()

	mu.Lock()
	defer mu.Unlock()
	if successRecords != 3 {
		t.Errorf("expected 3 success records in second batch, got %d", successRecords)
	}
}

// T9: Content-Type 및 URL 경로 검증
func TestBufferedHTTPTransport_ContentTypeAndPath(t *testing.T) {
	var receivedContentType string
	var receivedPath string

	batchCfg := newTestBatchConfig()
	batchCfg.FlushFrequency = 50 * time.Millisecond

	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	transport.Deliver(context.Background(), "tp_all_all_resource", makeTestRecords(1))
	time.Sleep(100 * time.Millisecond)
	transport.Close()

	if receivedContentType != "application/vnd.kafka.json.v2+json" {
		t.Errorf("expected Content-Type=application/vnd.kafka.json.v2+json, got %q", receivedContentType)
	}
	if receivedPath != "/topics/tp_all_all_resource" {
		t.Errorf("expected path=/topics/tp_all_all_resource, got %s", receivedPath)
	}
}

// --- SendBatch aggregate tests ---

type mockTransport struct {
	mu           sync.Mutex
	deliverCalls int
	totalRecords int
}

func (m *mockTransport) Deliver(ctx context.Context, topic string, records []KafkaRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.deliverCalls++
	m.totalRecords += len(records)
	return nil
}

func (m *mockTransport) Close() error { return nil }

func TestKafkaSender_SendBatch_AggregatesRecords(t *testing.T) {
	mock := &mockTransport{}
	eqpInfo := &config.EqpInfoConfig{
		Process: "PROC", EqpModel: "MODEL", EqpID: "EQP",
		Line: "LINE", LineDesc: "Desc", Index: "1",
	}
	sender := NewKafkaSender(mock, "topic", eqpInfo, func() int64 { return 0 }, GrokRawFormatter{})

	batch := []*collector.MetricData{
		{Type: "CPU", Timestamp: time.Now(), Data: collector.CPUData{UsagePercent: 50, CoreCount: 4}},
		{Type: "CPU", Timestamp: time.Now(), Data: collector.CPUData{UsagePercent: 60, CoreCount: 4}},
		{Type: "CPU", Timestamp: time.Now(), Data: collector.CPUData{UsagePercent: 70, CoreCount: 4}},
	}

	err := sender.SendBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()

	if mock.deliverCalls != 1 {
		t.Errorf("expected 1 Deliver call (aggregated), got %d", mock.deliverCalls)
	}
	if mock.totalRecords != 3 {
		t.Errorf("expected 3 total records, got %d", mock.totalRecords)
	}
}

func TestKafkaSender_SendBatch_SkipsErrNoRows(t *testing.T) {
	mock := &mockTransport{}
	eqpInfo := &config.EqpInfoConfig{
		Process: "PROC", EqpModel: "MODEL", EqpID: "EQP",
		Line: "LINE", LineDesc: "Desc", Index: "1",
	}
	sender := NewKafkaSender(mock, "topic", eqpInfo, func() int64 { return 0 }, GrokRawFormatter{})

	batch := []*collector.MetricData{
		{Type: "CPU", Timestamp: time.Now(), Data: collector.CPUData{UsagePercent: 50, CoreCount: 4}},
		{Type: "Unknown", Timestamp: time.Now(), Data: nil},
		{Type: "CPU", Timestamp: time.Now(), Data: collector.CPUData{UsagePercent: 70, CoreCount: 4}},
	}

	err := sender.SendBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.deliverCalls != 1 {
		t.Errorf("expected 1 Deliver call, got %d", mock.deliverCalls)
	}
	if mock.totalRecords != 2 {
		t.Errorf("expected 2 records (skipped Unknown), got %d", mock.totalRecords)
	}
}

func TestKafkaSender_SendBatch_EmptyBatch(t *testing.T) {
	mock := &mockTransport{}
	eqpInfo := &config.EqpInfoConfig{
		Process: "PROC", EqpModel: "MODEL", EqpID: "EQP",
		Line: "LINE", LineDesc: "Desc", Index: "1",
	}
	sender := NewKafkaSender(mock, "topic", eqpInfo, func() int64 { return 0 }, GrokRawFormatter{})

	err := sender.SendBatch(context.Background(), []*collector.MetricData{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.deliverCalls != 0 {
		t.Errorf("expected 0 Deliver calls for empty batch, got %d", mock.deliverCalls)
	}
}

func TestKafkaSender_SendBatch_AllErrNoRows(t *testing.T) {
	mock := &mockTransport{}
	eqpInfo := &config.EqpInfoConfig{
		Process: "PROC", EqpModel: "MODEL", EqpID: "EQP",
		Line: "LINE", LineDesc: "Desc", Index: "1",
	}
	sender := NewKafkaSender(mock, "topic", eqpInfo, func() int64 { return 0 }, GrokRawFormatter{})

	batch := []*collector.MetricData{
		{Type: "Unknown", Timestamp: time.Now(), Data: nil},
		{Type: "Unknown", Timestamp: time.Now(), Data: nil},
	}

	err := sender.SendBatch(context.Background(), batch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mock.mu.Lock()
	defer mock.mu.Unlock()
	if mock.deliverCalls != 0 {
		t.Errorf("expected 0 Deliver calls (all skipped), got %d", mock.deliverCalls)
	}
}

// T10: 빈 버퍼 Close는 HTTP 요청 없이 완료
func TestBufferedHTTPTransport_EmptyClose(t *testing.T) {
	var httpCalls int32

	batchCfg := newTestBatchConfig()
	transport, server := newTestBufferedTransport(t, func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&httpCalls, 1)
		w.WriteHeader(http.StatusOK)
	}, batchCfg)
	defer server.Close()

	transport.Close()

	if atomic.LoadInt32(&httpCalls) != 0 {
		t.Errorf("expected 0 HTTP calls for empty Close, got %d", httpCalls)
	}
}
