package sender

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"os"
	"strings"
	"sync"

	"github.com/IBM/sarama"
	"github.com/xdg-go/scram"
	"golang.org/x/net/proxy"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

var (
	// SHA256 hash generator for SCRAM-SHA-256
	SHA256 scram.HashGeneratorFcn = func() hash.Hash { return sha256.New() }
	// SHA512 hash generator for SCRAM-SHA-512
	SHA512 scram.HashGeneratorFcn = func() hash.Hash { return sha512.New() }
)

// XDGSCRAMClient implements sarama.SCRAMClient for SCRAM authentication.
type XDGSCRAMClient struct {
	*scram.Client
	*scram.ClientConversation
	HashGeneratorFcn scram.HashGeneratorFcn
}

// Begin starts the SCRAM authentication.
func (x *XDGSCRAMClient) Begin(userName, password, authzID string) (err error) {
	x.Client, err = x.HashGeneratorFcn.NewClient(userName, password, authzID)
	if err != nil {
		return err
	}
	x.ClientConversation = x.Client.NewConversation()
	return nil
}

// Step processes the server challenge.
func (x *XDGSCRAMClient) Step(challenge string) (response string, err error) {
	return x.ClientConversation.Step(challenge)
}

// Done returns true if the conversation is complete.
func (x *XDGSCRAMClient) Done() bool {
	return x.ClientConversation.Done()
}

// KafkaSender sends metrics to Kafka.
type KafkaSender struct {
	producer     sarama.AsyncProducer
	topic        string
	mu           sync.RWMutex
	closed       bool
	eqpInfo      *config.EqpInfoConfig // nil if Redis not configured
	timeDiffFunc func() int64
}

// NewKafkaSender creates a new Kafka sender with the given configuration.
func NewKafkaSender(cfg config.KafkaConfig, socksCfg config.SOCKSConfig, eqpInfo *config.EqpInfoConfig, timeDiffFunc func() int64) (*KafkaSender, error) {
	saramaConfig := sarama.NewConfig()

	// Producer settings
	saramaConfig.Producer.Return.Successes = false
	saramaConfig.Producer.Return.Errors = true
	saramaConfig.Producer.Retry.Max = cfg.MaxRetries
	saramaConfig.Producer.Retry.Backoff = cfg.RetryBackoff
	saramaConfig.Producer.Flush.Frequency = cfg.FlushFrequency
	saramaConfig.Producer.Flush.Messages = cfg.FlushMessages
	saramaConfig.Producer.Flush.MaxMessages = cfg.BatchSize

	// Compression
	switch strings.ToLower(cfg.Compression) {
	case "snappy":
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
	case "gzip":
		saramaConfig.Producer.Compression = sarama.CompressionGZIP
	case "lz4":
		saramaConfig.Producer.Compression = sarama.CompressionLZ4
	case "zstd":
		saramaConfig.Producer.Compression = sarama.CompressionZSTD
	default:
		saramaConfig.Producer.Compression = sarama.CompressionSnappy
	}

	// Required acks
	switch cfg.RequiredAcks {
	case 0:
		saramaConfig.Producer.RequiredAcks = sarama.NoResponse
	case 1:
		saramaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	case -1:
		saramaConfig.Producer.RequiredAcks = sarama.WaitForAll
	default:
		saramaConfig.Producer.RequiredAcks = sarama.WaitForLocal
	}

	// Timeout
	if cfg.Timeout > 0 {
		saramaConfig.Net.DialTimeout = cfg.Timeout
		saramaConfig.Net.ReadTimeout = cfg.Timeout
		saramaConfig.Net.WriteTimeout = cfg.Timeout
	}

	// TLS configuration
	if cfg.EnableTLS {
		tlsConfig, err := createTLSConfig(cfg.TLSCertFile, cfg.TLSKeyFile, cfg.TLSCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}
		saramaConfig.Net.TLS.Enable = true
		saramaConfig.Net.TLS.Config = tlsConfig
	}

	// SASL configuration
	if cfg.SASLEnabled {
		saramaConfig.Net.SASL.Enable = true
		saramaConfig.Net.SASL.User = cfg.SASLUser
		saramaConfig.Net.SASL.Password = cfg.SASLPassword

		switch strings.ToUpper(cfg.SASLMechanism) {
		case "PLAIN":
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		case "SCRAM-SHA-256":
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA256
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &XDGSCRAMClient{HashGeneratorFcn: SHA256}
			}
		case "SCRAM-SHA-512":
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypeSCRAMSHA512
			saramaConfig.Net.SASL.SCRAMClientGeneratorFunc = func() sarama.SCRAMClient {
				return &XDGSCRAMClient{HashGeneratorFcn: SHA512}
			}
		default:
			saramaConfig.Net.SASL.Mechanism = sarama.SASLTypePlaintext
		}
	}

	// SOCKS5 proxy support
	if socksCfg.Host != "" && socksCfg.Port > 0 {
		proxyAddr := fmt.Sprintf("%s:%d", socksCfg.Host, socksCfg.Port)
		socksDialer, proxyErr := proxy.SOCKS5("tcp", proxyAddr, nil, proxy.Direct)
		if proxyErr != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer for Kafka: %w", proxyErr)
		}
		saramaConfig.Net.Proxy.Enable = true
		saramaConfig.Net.Proxy.Dialer = socksDialer
	}

	producer, err := sarama.NewAsyncProducer(cfg.Brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	sender := &KafkaSender{
		producer:     producer,
		topic:        cfg.Topic,
		eqpInfo:      eqpInfo,
		timeDiffFunc: timeDiffFunc,
	}

	// Start error handler goroutine
	go sender.handleErrors()

	return sender, nil
}

// Send sends a single metric data to Kafka.
func (s *KafkaSender) Send(ctx context.Context, data *collector.MetricData) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("sender is closed")
	}
	s.mu.RUnlock()

	if s.eqpInfo != nil {
		// JSON mapper format: multiple KafkaValue messages per MetricData
		key, values, err := WrapMetricDataJSON(data, s.eqpInfo, s.timeDiffFunc())
		if err != nil {
			if errors.Is(err, ErrNoRows) {
				return nil // Skip: collector produced valid but empty data
			}
			return fmt.Errorf("failed to wrap metric data as JSON: %w", err)
		}
		for _, v := range values {
			msg := &sarama.ProducerMessage{
				Topic:     s.topic,
				Key:       sarama.StringEncoder(key),
				Value:     sarama.ByteEncoder(v),
				Timestamp: data.Timestamp,
			}
			select {
			case s.producer.Input() <- msg:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}

	// Legacy format: raw MetricData (no eqpInfo)
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal metric data: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic:     s.topic,
		Value:     sarama.ByteEncoder(jsonData),
		Timestamp: data.Timestamp,
	}
	if data.AgentID != "" {
		msg.Key = sarama.StringEncoder(data.AgentID)
	}

	select {
	case s.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// SendBatch sends multiple metric data items to Kafka.
func (s *KafkaSender) SendBatch(ctx context.Context, data []*collector.MetricData) error {
	for _, d := range data {
		if err := s.Send(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the Kafka producer.
func (s *KafkaSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.producer.Close()
}

func (s *KafkaSender) handleErrors() {
	log := logger.WithComponent("kafka-sender")
	for err := range s.producer.Errors() {
		log.Error().Err(err.Err).
			Str("topic", err.Msg.Topic).
			Interface("key", err.Msg.Key).
			Msg("Failed to send message to Kafka")
	}
}

func createTLSConfig(certFile, keyFile, caFile string) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA file: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	return tlsConfig, nil
}
