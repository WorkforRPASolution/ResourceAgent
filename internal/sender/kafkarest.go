package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
	"resourceagent/internal/network"
)

const (
	kafkaRestContentType = "application/vnd.kafka.json.v2+json"
)

// HTTPTransport implements KafkaTransport via the KafkaRest HTTP proxy.
type HTTPTransport struct {
	client    *http.Client
	transport *http.Transport
	baseURL   string
	closeOnce sync.Once
}

// NewHTTPTransport creates a new HTTP-based Kafka REST transport.
func NewHTTPTransport(kafkaRestAddr string, socksCfg config.SOCKSConfig) (*HTTPTransport, error) {
	transport, err := network.NewHTTPTransport(socksCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP transport for KafkaRest: %w", err)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	return &HTTPTransport{
		client:    client,
		transport: transport,
		baseURL:   ensureHTTPScheme(kafkaRestAddr),
	}, nil
}

// Deliver sends records to the KafkaRest proxy as a KafkaMessageWrapper2 JSON POST.
func (t *HTTPTransport) Deliver(ctx context.Context, topic string, records []KafkaRecord) error {
	messages := make([]KafkaMessage2, len(records))
	for i, rec := range records {
		messages[i] = KafkaMessage2{
			Key:   rec.Key,
			Value: rec.Value,
		}
	}

	wrapper := KafkaMessageWrapper2{Records: messages}
	body, err := json.Marshal(wrapper)
	if err != nil {
		return fmt.Errorf("failed to marshal KafkaMessageWrapper2: %w", err)
	}

	url := t.baseURL + "/topics/" + topic

	const defaultRetries = 2
	const defaultRetryDelay = 500 * time.Millisecond

	var lastErr error
	for attempt := 0; attempt <= defaultRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(defaultRetryDelay):
			}
		}

		lastErr = t.doPost(ctx, url, body)
		if lastErr == nil {
			return nil
		}

		log := logger.WithComponent("kafkarest-transport")
		log.Warn().
			Err(lastErr).
			Int("attempt", attempt+1).
			Msg("KafkaRest send failed, retrying")
	}

	return fmt.Errorf("KafkaRest send failed after %d retries: %w", defaultRetries, lastErr)
}

// Close releases idle TCP connections in the transport pool.
// Safe to call multiple times.
func (t *HTTPTransport) Close() error {
	t.closeOnce.Do(func() {
		t.transport.CloseIdleConnections()
	})
	return nil
}

func (t *HTTPTransport) doPost(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", kafkaRestContentType)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("KafkaRest returned HTTP %d", resp.StatusCode)
	}

	return nil
}

// bufferedEntry holds records along with their target topic.
type bufferedEntry struct {
	topic   string
	records []KafkaRecord
}

// BufferedHTTPTransport implements KafkaTransport with buffered batch delivery via HTTP.
type BufferedHTTPTransport struct {
	client    *http.Client
	transport *http.Transport
	baseURL   string
	batchCfg  config.BatchConfig

	mu     sync.Mutex
	buffer []bufferedEntry
	topic  string // current topic (assumes single topic per sender)

	flushCh   chan struct{} // signal to flush immediately
	stopCh    chan struct{}
	doneCh    chan struct{}
	closeOnce sync.Once
}

// NewBufferedHTTPTransport creates a new buffered HTTP transport with batch delivery.
func NewBufferedHTTPTransport(kafkaRestAddr string, socksCfg config.SOCKSConfig, batchCfg config.BatchConfig) (*BufferedHTTPTransport, error) {
	transport, err := network.NewHTTPTransport(socksCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP transport for KafkaRest: %w", err)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	t := &BufferedHTTPTransport{
		client:    client,
		transport: transport,
		baseURL:   ensureHTTPScheme(kafkaRestAddr),
		batchCfg:  batchCfg,
		flushCh:   make(chan struct{}, 1),
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}

	go t.flushLoop()
	return t, nil
}

// Deliver buffers records for later batch delivery. Returns nil immediately.
func (t *BufferedHTTPTransport) Deliver(_ context.Context, topic string, records []KafkaRecord) error {
	t.mu.Lock()
	t.buffer = append(t.buffer, bufferedEntry{topic: topic, records: records})
	t.topic = topic
	count := t.bufferRecordCount()
	t.mu.Unlock()

	if count >= t.batchCfg.FlushMessages {
		select {
		case t.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// Close stops the flush loop, flushes remaining records, and releases idle TCP connections.
// Safe to call multiple times.
func (t *BufferedHTTPTransport) Close() error {
	t.closeOnce.Do(func() {
		close(t.stopCh)
		<-t.doneCh
		t.transport.CloseIdleConnections()
	})
	return nil
}

func (t *BufferedHTTPTransport) bufferRecordCount() int {
	count := 0
	for _, entry := range t.buffer {
		count += len(entry.records)
	}
	return count
}

func (t *BufferedHTTPTransport) flushLoop() {
	defer close(t.doneCh)

	ticker := time.NewTicker(t.batchCfg.FlushFrequency)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			t.flush("close")
			return
		case <-ticker.C:
			t.flush("timer")
		case <-t.flushCh:
			t.flush("count")
		}
	}
}

func (t *BufferedHTTPTransport) flush(trigger string) {
	t.mu.Lock()
	if len(t.buffer) == 0 {
		t.mu.Unlock()
		return
	}
	entries := t.buffer
	t.buffer = nil
	t.mu.Unlock()

	// Aggregate all records by topic
	topicRecords := make(map[string][]KafkaRecord)
	totalRecords := 0
	for _, entry := range entries {
		topicRecords[entry.topic] = append(topicRecords[entry.topic], entry.records...)
		totalRecords += len(entry.records)
	}

	log := logger.WithComponent("buffered-kafkarest")
	log.Debug().
		Str("trigger", trigger).
		Int("records", totalRecords).
		Int("topics", len(topicRecords)).
		Msg("Flushing buffered records")

	for topic, records := range topicRecords {
		t.sendBatchWithSplit(topic, records)
	}
}

func (t *BufferedHTTPTransport) sendBatchWithSplit(topic string, records []KafkaRecord) {
	maxSize := t.batchCfg.MaxBatchSize
	if maxSize <= 0 {
		maxSize = len(records)
	}

	for i := 0; i < len(records); i += maxSize {
		end := i + maxSize
		if end > len(records) {
			end = len(records)
		}
		t.sendBatch(topic, records[i:end])
	}
}

func (t *BufferedHTTPTransport) sendBatch(topic string, records []KafkaRecord) {
	messages := make([]KafkaMessage2, len(records))
	for i, rec := range records {
		messages[i] = KafkaMessage2{
			Key:   rec.Key,
			Value: rec.Value,
		}
	}

	wrapper := KafkaMessageWrapper2{Records: messages}
	body, err := json.Marshal(wrapper)
	if err != nil {
		log := logger.WithComponent("buffered-kafkarest")
		log.Error().Err(err).Msg("Failed to marshal batch")
		return
	}

	url := t.baseURL + "/topics/" + topic
	log := logger.WithComponent("buffered-kafkarest")

	var lastErr error
	for attempt := 0; attempt <= t.batchCfg.MaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(t.batchCfg.RetryBackoff)
		}

		lastErr = t.doPost(url, body)
		if lastErr == nil {
			log.Debug().
				Int("records", len(records)).
				Str("topic", topic).
				Msg("Batch sent successfully")
			return
		}

		log.Warn().
			Err(lastErr).
			Int("attempt", attempt+1).
			Int("records", len(records)).
			Msg("Buffered KafkaRest send failed, retrying")
	}

	log.Error().
		Err(lastErr).
		Int("records", len(records)).
		Msg("Buffered KafkaRest send failed after all retries, dropping batch")
}

func (t *BufferedHTTPTransport) doPost(url string, body []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", kafkaRestContentType)

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("KafkaRest returned HTTP %d", resp.StatusCode)
	}

	return nil
}

func ensureHTTPScheme(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}
