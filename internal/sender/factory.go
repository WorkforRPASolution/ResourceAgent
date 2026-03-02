package sender

import (
	"fmt"
	"strings"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

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
		log.Info().
			Strs("brokers", cfg.Kafka.Brokers).
			Str("topic", topic).
			Msg("Creating Kafka sender")
		transport, err := NewSaramaTransport(cfg.Kafka, cfg.Batch, cfg.SOCKSProxy)
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
