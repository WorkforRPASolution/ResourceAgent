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

// RawFormatter formats an EARSRow into the raw string for a KafkaValue.
type RawFormatter interface {
	FormatRaw(row EARSRow, process string) (string, error)
}

// GrokRawFormatter produces plain text for the KafkaRest/Grok pipeline.
type GrokRawFormatter struct{}

func (f GrokRawFormatter) FormatRaw(row EARSRow, _ string) (string, error) {
	return row.ToGrokString(), nil
}

// JSONRawFormatter produces ParsedDataList JSON for the Kafka direct/JSON mapper pipeline.
type JSONRawFormatter struct{}

func (f JSONRawFormatter) FormatRaw(row EARSRow, process string) (string, error) {
	pdl := row.ToParsedData(process)
	b, err := json.Marshal(pdl)
	if err != nil {
		return "", fmt.Errorf("failed to marshal ParsedDataList: %w", err)
	}
	return string(b), nil
}

// KafkaValue is the value structure for Kafka messages.
// Both the KafkaRest (Grok) and Kafka direct (JSON mapper) pipelines use this struct.
// The "process" field is safe to include for Kafka direct consumers because
// json4s (KafkaToElastic) ignores extra fields.
type KafkaValue struct {
	Process string `json:"process"`
	Line    string `json:"line"`
	EqpID   string `json:"eqpid"`
	Model   string `json:"model"`
	Diff    int64  `json:"diff"`
	ESID    string `json:"esid"`
	Raw     string `json:"raw"`
}

// KafkaMessage2 is a single record in the Kafka REST message format.
type KafkaMessage2 struct {
	Key   string     `json:"key"`
	Value KafkaValue `json:"value"`
}

// KafkaMessageWrapper2 is the Kafka REST message wrapper for HTTP transport.
type KafkaMessageWrapper2 struct {
	Records []KafkaMessage2 `json:"records"`
}

// KafkaRecord is an intermediate record produced by PrepareRecords.
type KafkaRecord struct {
	Key       string
	Value     KafkaValue
	Timestamp time.Time
}

// generateESID creates an ESID in the format: {Process}:{EqpID}-{metricType}-{timestamp_ms}-{counter}
func generateESID(process, eqpID, metricType string, timestampMs int64, counter int) string {
	return fmt.Sprintf("%s:%s-%s-%d-%d", process, eqpID, metricType, timestampMs, counter)
}

// PrepareRecords converts MetricData into KafkaRecords using the given formatter.
func PrepareRecords(data *collector.MetricData, eqpInfo *config.EqpInfoConfig, timeDiff int64, formatter RawFormatter) ([]KafkaRecord, error) {
	rows := ConvertToEARSRows(data)
	if len(rows) == 0 {
		return nil, fmt.Errorf("%w: metric type %q", ErrNoRows, data.Type)
	}

	tsMs := data.Timestamp.UnixMilli()
	records := make([]KafkaRecord, 0, len(rows))

	for i, row := range rows {
		raw, err := formatter.FormatRaw(row, eqpInfo.Process)
		if err != nil {
			return nil, fmt.Errorf("failed to format raw for row %d: %w", i, err)
		}

		records = append(records, KafkaRecord{
			Key: eqpInfo.EqpID,
			Value: KafkaValue{
				Process: eqpInfo.Process,
				Line:    eqpInfo.Line,
				EqpID:   eqpInfo.EqpID,
				Model:   eqpInfo.EqpModel,
				Diff:    timeDiff,
				ESID:    generateESID(eqpInfo.Process, eqpInfo.EqpID, data.Type, tsMs, i),
				Raw:     raw,
			},
			Timestamp: data.Timestamp,
		})
	}

	return records, nil
}
