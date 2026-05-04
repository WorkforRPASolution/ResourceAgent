package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog"

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
//
// Memory safety: Deliver enforces an upper bound (BatchConfig.MaxBufferedRecords)
// on the in-memory record count. When exceeded, oldest entries are dropped
// (FIFO) to keep RSS bounded if KafkaRest becomes unreachable. See
// docs/runbooks/buffered-http-transport-monitoring.md for diagnosis.
type BufferedHTTPTransport struct {
	client    *http.Client
	transport *http.Transport
	baseURL   string
	batchCfg  config.BatchConfig

	mu          sync.Mutex
	buffer      []bufferedEntry
	bufferCount int    // mu-protected source of truth for record count enforcement
	topic       string // current topic (assumes single topic per sender)
	closed      bool

	// Lock-free observability fields. Writes happen inside mu (so they are
	// consistent with bufferCount); reads from external SelfMetrics may
	// occur without the lock — eventual-consistency is acceptable.
	bufferCountObs      atomic.Int64
	droppedTotal        atomic.Int64
	bufferHighWaterMark atomic.Int64

	dropLogger zerolog.Logger // BasicSampler{N:10} sampled logger for drop bursts

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

	base := logger.WithComponent("kafkarest-buffer")
	sampled := base.Sample(&zerolog.BasicSampler{N: 10})

	t := &BufferedHTTPTransport{
		client:     client,
		transport:  transport,
		baseURL:    ensureHTTPScheme(kafkaRestAddr),
		batchCfg:   batchCfg,
		dropLogger: sampled,
		flushCh:    make(chan struct{}, 1),
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}

	go t.flushLoop()
	return t, nil
}

// Deliver buffers records for later batch delivery. Returns nil immediately.
//
// If MaxBufferedRecords is set (>0) and the buffer would exceed it after
// appending, oldest entries are dropped (FIFO) inside the same critical
// section that performs the append. Drops are accounted for via atomic
// counters and a sampled ERROR log so log volume stays bounded even if
// the drop streak is long.
func (t *BufferedHTTPTransport) Deliver(_ context.Context, topic string, records []KafkaRecord) error {
	if len(records) == 0 {
		return nil
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return errors.New("transport closed")
	}

	t.buffer = append(t.buffer, bufferedEntry{topic: topic, records: records})
	t.bufferCount += len(records)
	t.topic = topic

	var droppedCount int
	cap := t.batchCfg.MaxBufferedRecords
	if cap > 0 {
		for t.bufferCount > cap && len(t.buffer) > 0 {
			droppedCount += len(t.buffer[0].records)
			t.bufferCount -= len(t.buffer[0].records)
			t.buffer = t.buffer[1:]
		}
	}

	cur := int64(t.bufferCount)
	t.bufferCountObs.Store(cur)
	if cur > t.bufferHighWaterMark.Load() {
		t.bufferHighWaterMark.Store(cur)
	}

	flushNeeded := t.bufferCount >= t.batchCfg.FlushMessages
	t.mu.Unlock()

	if droppedCount > 0 {
		newTotal := t.droppedTotal.Add(int64(droppedCount))
		t.dropLogger.Error().
			Int("dropped_records", droppedCount).
			Int64("buffer_count", cur).
			Int("max_buffered_records", cap).
			Int64("dropped_total", newTotal).
			Msg("BUFFER_DROP_OLDEST oldest records dropped due to buffer cap (sampled 1/10)")
	}

	if flushNeeded {
		select {
		case t.flushCh <- struct{}{}:
		default:
		}
	}

	return nil
}

// BufferStats returns lock-free observability snapshots for the buffer.
// Intended for SelfMetrics / debugging.
//
//   count   — current buffered record count
//   dropped — cumulative dropped records since process start
//   hwm     — high-water-mark observed buffer count
func (t *BufferedHTTPTransport) BufferStats() (count, dropped, hwm int64) {
	return t.bufferCountObs.Load(), t.droppedTotal.Load(), t.bufferHighWaterMark.Load()
}

// Close stops the flush loop, flushes remaining records, and releases idle TCP connections.
// Safe to call multiple times.
func (t *BufferedHTTPTransport) Close() error {
	t.closeOnce.Do(func() {
		t.mu.Lock()
		t.closed = true
		t.mu.Unlock()
		close(t.stopCh)
		<-t.doneCh
		t.transport.CloseIdleConnections()
	})
	return nil
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
	t.bufferCount = 0
	t.bufferCountObs.Store(0)
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
