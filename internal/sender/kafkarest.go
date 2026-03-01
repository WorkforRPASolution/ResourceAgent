package sender

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
	"resourceagent/internal/network"
)

const (
	kafkaRestContentType = "application/vnd.kafka.json.v2+json"
	maxRetries           = 2
	retryDelay           = 500 * time.Millisecond
)

// HTTPTransport implements KafkaTransport via the KafkaRest HTTP proxy.
type HTTPTransport struct {
	client  *http.Client
	baseURL string
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
		client:  client,
		baseURL: ensureHTTPScheme(kafkaRestAddr),
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

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(retryDelay):
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

	return fmt.Errorf("KafkaRest send failed after %d retries: %w", maxRetries, lastErr)
}

// Close is a no-op for HTTP transport.
func (t *HTTPTransport) Close() error {
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

func ensureHTTPScheme(addr string) string {
	if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
		return addr
	}
	return "http://" + addr
}
