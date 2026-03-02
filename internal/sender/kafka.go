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
	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/logger"
	"resourceagent/internal/network"
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

// KafkaTransport delivers records to Kafka.
type KafkaTransport interface {
	Deliver(ctx context.Context, topic string, records []KafkaRecord) error
	Close() error
}

// SaramaTransport implements KafkaTransport using the sarama async producer.
type SaramaTransport struct {
	producer sarama.AsyncProducer
}

// NewSaramaTransport creates a new sarama-based Kafka transport.
func NewSaramaTransport(brokers []string, cfg config.KafkaConfig, batchCfg config.BatchConfig, socksCfg config.SOCKSConfig) (*SaramaTransport, error) {
	saramaConfig := sarama.NewConfig()

	// Producer settings
	saramaConfig.Producer.Return.Successes = false
	saramaConfig.Producer.Return.Errors = true
	saramaConfig.Producer.Retry.Max = batchCfg.MaxRetries
	saramaConfig.Producer.Retry.Backoff = batchCfg.RetryBackoff
	saramaConfig.Producer.Flush.Frequency = batchCfg.FlushFrequency
	saramaConfig.Producer.Flush.Messages = batchCfg.FlushMessages
	saramaConfig.Producer.Flush.MaxMessages = batchCfg.MaxBatchSize

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
		socksDialer, proxyErr := network.NewSOCKS5Dialer(socksCfg.Host, socksCfg.Port)
		if proxyErr != nil {
			return nil, fmt.Errorf("failed to create SOCKS5 dialer for Kafka: %w", proxyErr)
		}
		saramaConfig.Net.Proxy.Enable = true
		saramaConfig.Net.Proxy.Dialer = socksDialer
	}

	producer, err := sarama.NewAsyncProducer(brokers, saramaConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka producer: %w", err)
	}

	t := &SaramaTransport{producer: producer}
	go t.handleErrors()

	return t, nil
}

// Deliver sends records to Kafka via the sarama async producer.
func (t *SaramaTransport) Deliver(ctx context.Context, topic string, records []KafkaRecord) error {
	for _, rec := range records {
		valueBytes, err := json.Marshal(rec.Value)
		if err != nil {
			return fmt.Errorf("failed to marshal KafkaValue: %w", err)
		}
		msg := &sarama.ProducerMessage{
			Topic:     topic,
			Key:       sarama.StringEncoder(rec.Key),
			Value:     sarama.ByteEncoder(valueBytes),
			Timestamp: rec.Timestamp,
		}
		select {
		case t.producer.Input() <- msg:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}

// Close shuts down the sarama producer.
func (t *SaramaTransport) Close() error {
	return t.producer.Close()
}

func (t *SaramaTransport) handleErrors() {
	log := logger.WithComponent("kafka-transport")
	for err := range t.producer.Errors() {
		log.Error().Err(err.Err).
			Str("topic", err.Msg.Topic).
			Interface("key", err.Msg.Key).
			Msg("Failed to send message to Kafka")
	}
}

// KafkaSender sends metrics to Kafka via a pluggable transport and formatter.
type KafkaSender struct {
	transport    KafkaTransport
	topic        string
	eqpInfo      *config.EqpInfoConfig
	timeDiffFunc func() int64
	formatter    RawFormatter
	mu           sync.RWMutex
	closed       bool
}

// NewKafkaSender creates a unified Kafka sender with the given transport and formatter.
func NewKafkaSender(transport KafkaTransport, topic string, eqpInfo *config.EqpInfoConfig, timeDiffFunc func() int64, formatter RawFormatter) *KafkaSender {
	return &KafkaSender{
		transport:    transport,
		topic:        topic,
		eqpInfo:      eqpInfo,
		timeDiffFunc: timeDiffFunc,
		formatter:    formatter,
	}
}

// Send sends a single metric data to Kafka.
func (s *KafkaSender) Send(ctx context.Context, data *collector.MetricData) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("sender is closed")
	}
	s.mu.RUnlock()

	records, err := PrepareRecords(data, s.eqpInfo, s.timeDiffFunc(), s.formatter)
	if err != nil {
		if errors.Is(err, ErrNoRows) {
			log := logger.WithComponent("kafka-sender")
			log.Debug().Str("type", data.Type).Msg("Skipping empty metric data (no rows)")
			return nil
		}
		return fmt.Errorf("failed to prepare records: %w", err)
	}

	return s.transport.Deliver(ctx, s.topic, records)
}

// SendBatch sends multiple metric data items to Kafka in a single aggregated Deliver call.
func (s *KafkaSender) SendBatch(ctx context.Context, data []*collector.MetricData) error {
	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return fmt.Errorf("sender is closed")
	}
	s.mu.RUnlock()

	var allRecords []KafkaRecord
	for _, d := range data {
		records, err := PrepareRecords(d, s.eqpInfo, s.timeDiffFunc(), s.formatter)
		if err != nil {
			if errors.Is(err, ErrNoRows) {
				continue
			}
			return fmt.Errorf("failed to prepare records: %w", err)
		}
		allRecords = append(allRecords, records...)
	}
	if len(allRecords) == 0 {
		return nil
	}
	return s.transport.Deliver(ctx, s.topic, allRecords)
}

// Close closes the Kafka sender and its transport.
func (s *KafkaSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.transport.Close()
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
