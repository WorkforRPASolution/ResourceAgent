package sender

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/logger"
	"resourceagent/internal/network"
)

const (
	kafkaRestContentType = "application/vnd.kafka.json.v2+json"
	maxRetries           = 2
	retryDelay           = 500 * time.Millisecond
)

// KafkaRestSender sends metrics to Kafka via the KafkaRest HTTP proxy.
type KafkaRestSender struct {
	client       *http.Client
	url          string // cached: baseURL + "/topics/" + topic
	eqpInfo      *config.EqpInfoConfig
	timeDiffFunc func() int64
	mu           sync.RWMutex
	closed       bool
}

// NewKafkaRestSender creates a new KafkaRest HTTP sender.
func NewKafkaRestSender(kafkaRestAddr, topic string, eqpInfo *config.EqpInfoConfig,
	socksCfg config.SOCKSConfig, timeDiffFunc func() int64) (*KafkaRestSender, error) {

	transport, err := network.NewHTTPTransport(socksCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP transport for KafkaRest: %w", err)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}

	baseURL := ensureHTTPScheme(kafkaRestAddr)

	return &KafkaRestSender{
		client:       client,
		url:          baseURL + "/topics/" + topic,
		eqpInfo:      eqpInfo,
		timeDiffFunc: timeDiffFunc,
	}, nil
}

// Send transmits a single metric to KafkaRest.
func (s *KafkaRestSender) Send(ctx context.Context, data *collector.MetricData) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("sender is closed")
	}
	s.mu.RUnlock()

	body, err := WrapMetricDataLegacy(data, s.eqpInfo, s.timeDiffFunc())
	if err != nil {
		if errors.Is(err, ErrNoRows) {
			return nil // Skip: collector produced valid but empty data
		}
		return fmt.Errorf("failed to wrap metric data: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
			}
		}

		lastErr = s.doPost(ctx, s.url, body)
		if lastErr == nil {
			return nil
		}

		log := logger.WithComponent("kafkarest-sender")
		log.Warn().
			Err(lastErr).
			Int("attempt", attempt+1).
			Msg("KafkaRest send failed, retrying")
	}

	return fmt.Errorf("KafkaRest send failed after %d retries: %w", maxRetries, lastErr)
}

// SendBatch transmits multiple metric data items.
func (s *KafkaRestSender) SendBatch(ctx context.Context, data []*collector.MetricData) error {
	for _, d := range data {
		if err := s.Send(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// Close releases resources.
func (s *KafkaRestSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	return nil
}

func (s *KafkaRestSender) doPost(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", kafkaRestContentType)

	resp, err := s.client.Do(req)
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
