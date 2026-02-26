package sender

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
)

// ErrNoRows is returned when a MetricData produces no EARS rows.
// This is not a failure — it means the collector had no data to report
// (e.g., no fan sensors detected). Callers should skip sending silently.
var ErrNoRows = errors.New("no EARS rows produced")

// KafkaValue is the value structure for Kafka direct messages (JSON mapper pipeline).
// Unlike KafkaValue2, it has no "process" field — EARS_PROCESS is in ParsedDataList.
type KafkaValue struct {
	Line  string `json:"line"`
	EqpID string `json:"eqpid"`
	Model string `json:"model"`
	Diff  int64  `json:"diff"`
	ESID  string `json:"esid"`
	Raw   string `json:"raw"` // ParsedDataList JSON string
}

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

// generateESID creates an ESID in the format: {Process}:{EqpID}-{metricType}-{timestamp_ms}-{counter}
func generateESID(process, eqpID, metricType string, timestampMs int64, counter int) string {
	return fmt.Sprintf("%s:%s-%s-%d-%d", process, eqpID, metricType, timestampMs, counter)
}

// WrapMetricDataLegacy creates KafkaMessageWrapper2 with plain text raw (for KafkaRest/Grok).
// Returns multiple records, one per EARS row.
func WrapMetricDataLegacy(data *collector.MetricData, eqpInfo *config.EqpInfoConfig, timeDiff int64) ([]byte, error) {
	rows := ConvertToEARSRows(data)
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: metric type %q", ErrNoRows, data.Type)
	}

	tsMs := data.Timestamp.UnixMilli()
	records := make([]KafkaMessage2, 0, len(rows))
	for i, row := range rows {
		records = append(records, KafkaMessage2{
			Key: eqpInfo.EqpID,
			Value: KafkaValue2{
				Process: eqpInfo.Process,
				Line:    eqpInfo.Line,
				EqpID:   eqpInfo.EqpID,
				Model:   eqpInfo.EqpModel,
				Diff:    timeDiff,
				ESID:    generateESID(eqpInfo.Process, eqpInfo.EqpID, data.Type, tsMs, i),
				Raw:     row.ToLegacyString(),
			},
		})
	}

	wrapper := KafkaMessageWrapper2{Records: records}
	return json.Marshal(wrapper)
}

// WrapMetricDataJSON creates KafkaValue messages with ParsedDataList raw (for Kafka direct/JSON mapper).
// Returns the key and multiple KafkaValue JSONs, one per EARS row.
func WrapMetricDataJSON(data *collector.MetricData, eqpInfo *config.EqpInfoConfig, timeDiff int64) (string, [][]byte, error) {
	rows := ConvertToEARSRows(data)
	if len(rows) == 0 {
		return "", nil, fmt.Errorf("%w: metric type %q", ErrNoRows, data.Type)
	}

	tsMs := data.Timestamp.UnixMilli()
	key := eqpInfo.EqpID
	values := make([][]byte, 0, len(rows))

	for i, row := range rows {
		pdl := row.ToParsedData(eqpInfo.Process)
		rawJSON, err := json.Marshal(pdl)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal ParsedDataList: %w", err)
		}

		kv := KafkaValue{
			Line:  eqpInfo.Line,
			EqpID: eqpInfo.EqpID,
			Model: eqpInfo.EqpModel,
			Diff:  timeDiff,
			ESID:  generateESID(eqpInfo.Process, eqpInfo.EqpID, data.Type, tsMs, i),
			Raw:   string(rawJSON),
		}
		b, err := json.Marshal(kv)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal KafkaValue: %w", err)
		}
		values = append(values, b)
	}

	return key, values, nil
}
