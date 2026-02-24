package sender

import (
	"encoding/json"
	"fmt"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

// KafkaValue2 is the value structure for the production Kafka message format.
type KafkaValue2 struct {
	Process string `json:"process"`
	Line    string `json:"line"`
	EqpID   string `json:"eqpid"`
	Model   string `json:"model"`
	Diff    int64  `json:"diff"`
	ESID    string `json:"esid"`
	Raw     string `json:"raw"` // MetricData JSON string
}

// KafkaMessage2 is a single record in the production Kafka message format.
type KafkaMessage2 struct {
	Key   string      `json:"key"`
	Value KafkaValue2 `json:"value"`
}

// KafkaMessageWrapper2 is the production-compatible Kafka message wrapper.
type KafkaMessageWrapper2 struct {
	Records []KafkaMessage2 `json:"records"`
}

// WrapMetricData wraps a MetricData into KafkaMessageWrapper2 format.
// The raw MetricData is JSON-serialized and placed in the "raw" field.
// Returns the wrapper JSON bytes.
func WrapMetricData(data *collector.MetricData, eqpInfo *config.EqpInfoConfig) ([]byte, error) {
	// Serialize the original metric data as the "raw" field
	rawJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal metric data for wrapping: %w", err)
	}

	wrapper := KafkaMessageWrapper2{
		Records: []KafkaMessage2{
			{
				Key: eqpInfo.EqpID,
				Value: KafkaValue2{
					Process: eqpInfo.Process,
					Line:    eqpInfo.Line,
					EqpID:   eqpInfo.EqpID,
					Model:   eqpInfo.EqpModel,
					Diff:    time.Now().UnixMilli() - data.Timestamp.UnixMilli(),
					ESID:    fmt.Sprintf("%s_%s_%s", eqpInfo.EqpID, data.Type, data.Timestamp.Format("20060102150405")),
					Raw:     string(rawJSON),
				},
			},
		},
	}

	return json.Marshal(wrapper)
}
