package sender

import (
	"fmt"
	"net"
	"strings"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// extractHost strips scheme and port from an address, returning just the hostname/IP.
func extractHost(addr string) string {
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// resolveBrokerAddr derives a Kafka broker address from a KafkaRest address and broker port.
// KafkaRest proxy and Kafka broker run on the same k8s node (DaemonSet),
// so the KafkaRest host is also the broker host.
func resolveBrokerAddr(kafkaRestAddr string, brokerPort int) (string, error) {
	host := extractHost(kafkaRestAddr)
	if host == "" {
		return "", fmt.Errorf("cannot resolve Kafka broker: KafkaRestAddress is empty (ServiceDiscovery may have failed)")
	}
	if brokerPort <= 0 {
		brokerPort = 9092
	}
	return fmt.Sprintf("%s:%d", host, brokerPort), nil
}

// NewSender creates a Sender based on the configuration.
func NewSender(cfg *config.Config, timeDiffFunc func() int64) (Sender, error) {
	log := logger.WithComponent("sender-factory")

	senderType := strings.ToLower(cfg.SenderType)
	if senderType == "" {
		senderType = "kafka" // default for backward compatibility
	}

	log.Info().
		Str("sender_type", senderType).
		Msg("Creating sender")

	switch senderType {
	case "kafkarest":
		topic := config.ResolveTopic(cfg.ResourceMonitorTopic, cfg.EqpInfo)
		log.Info().
			Str("kafkarest_addr", cfg.KafkaRestAddress).
			Str("topic", topic).
			Msg("Creating KafkaRest sender")
		transport, err := NewBufferedHTTPTransport(cfg.KafkaRestAddress, cfg.SOCKSProxy, cfg.Batch)
		if err != nil {
			return nil, err
		}
		return NewKafkaSender(transport, topic, cfg.EqpInfo, timeDiffFunc, GrokRawFormatter{}), nil
	case "kafka":
		topic := config.ResolveTopic(cfg.ResourceMonitorTopic, cfg.EqpInfo)
		brokerAddr, addrErr := resolveBrokerAddr(cfg.KafkaRestAddress, cfg.Kafka.BrokerPort)
		if addrErr != nil {
			return nil, addrErr
		}
		log.Info().
			Str("broker", brokerAddr).
			Str("topic", topic).
			Msg("Creating Kafka sender")
		transport, err := NewSaramaTransport([]string{brokerAddr}, cfg.Kafka, cfg.Batch, cfg.SOCKSProxy)
		if err != nil {
			return nil, err
		}
		return NewKafkaSender(transport, topic, cfg.EqpInfo, timeDiffFunc, JSONRawFormatter{}), nil
	case "file":
		return NewFileSender(cfg.File)
	default:
		return nil, fmt.Errorf("unknown sender type: %s (supported: kafkarest, kafka, file)", senderType)
	}
}
