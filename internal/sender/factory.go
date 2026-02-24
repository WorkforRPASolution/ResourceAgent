package sender

import (
	"fmt"
	"strings"

	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// NewSender creates a Sender based on the configuration.
// It returns a KafkaSender by default, or a FileSender if sender_type is "file".
func NewSender(cfg *config.Config) (Sender, error) {
	log := logger.WithComponent("sender-factory")

	senderType := strings.ToLower(cfg.SenderType)
	if senderType == "" {
		senderType = "kafka" // default for backward compatibility
	}

	log.Info().
		Str("sender_type", senderType).
		Msg("Creating sender")

	switch senderType {
	case "kafka":
		return NewKafkaSender(cfg.Kafka, cfg.SOCKSProxy, cfg.EqpInfo)
	case "file":
		return NewFileSender(cfg.File)
	default:
		return nil, fmt.Errorf("unknown sender type: %s (supported: kafka, file)", senderType)
	}
}
