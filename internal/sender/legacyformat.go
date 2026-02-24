package sender

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"resourceagent/internal/collector"
)

// EARSRow represents one metric line with EARS fields.
type EARSRow struct {
	Timestamp time.Time
	Category  string
	PID       int
	ProcName  string
	Metric    string
	Value     float64
}

const (
	legacyTimeFmt = "2006-01-02 15:04:05"
	jsonTimeFmt   = "2006-01-02T15:04:05"
)

// FormatLegacyTimestamp formats to "2006-01-02 15:04:05,000" (Grok TIMESTAMP_ISO8601 compatible).
func FormatLegacyTimestamp(t time.Time) string {
	return fmt.Sprintf("%s,%03d", t.Format(legacyTimeFmt), t.Nanosecond()/1e6)
}

// FormatJSONTimestamp formats to "2006-01-02T15:04:05.000" (JSON mapper compatible).
func FormatJSONTimestamp(t time.Time) string {
	return fmt.Sprintf("%s.%03d", t.Format(jsonTimeFmt), t.Nanosecond()/1e6)
}

// formatValue formats a float64 without scientific notation.
func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ToLegacyString returns the ARSAgent-compatible plain text format for Grok parsing.
func (r EARSRow) ToLegacyString() string {
	return fmt.Sprintf("%s category:%s,pid:%d,proc:%s,metric:%s,value:%s",
		FormatLegacyTimestamp(r.Timestamp), r.Category, r.PID, r.ProcName, r.Metric, formatValue(r.Value))
}

// ParsedData represents a typed key-value pair for the JSON mapper pipeline.
type ParsedData struct {
	Name  string `json:"name"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

// ParsedDataList is the container for ParsedData entries with a timestamp.
type ParsedDataList struct {
	Timestamp string       `json:"timestamp"`
	Data      []ParsedData `json:"data"`
}

// ToParsedData returns a ParsedDataList for the JSON mapper format.
func (r EARSRow) ToParsedData(process string) ParsedDataList {
	return ParsedDataList{
		Timestamp: FormatJSONTimestamp(r.Timestamp),
		Data: []ParsedData{
			{Name: "EARS_PROCESS", Value: process, Type: "String"},
			{Name: "EARS_CATEGORY", Value: r.Category, Type: "String"},
			{Name: "EARS_PID", Value: strconv.Itoa(r.PID), Type: "Int"},
			{Name: "EARS_PROCNAME", Value: r.ProcName, Type: "String"},
			{Name: "EARS_METRIC", Value: r.Metric, Type: "String"},
			{Name: "EARS_VALUE", Value: formatValue(r.Value), Type: "Double"},
		},
	}
}

// ConvertToEARSRows converts MetricData to EARS rows based on the metric type.
func ConvertToEARSRows(data *collector.MetricData) []EARSRow {
	switch data.Type {
	case "cpu":
		return convertCPU(data)
	case "memory":
		return convertMemory(data)
	case "disk":
		return convertDisk(data)
	case "network":
		return convertNetwork(data)
	case "cpu_process":
		return convertCPUProcess(data)
	case "memory_process":
		return convertMemoryProcess(data)
	case "temperature":
		return convertTemperature(data)
	case "gpu":
		return convertGPU(data)
	case "fan":
		return convertFan(data)
	case "voltage":
		return convertVoltage(data)
	case "motherboard_temp":
		return convertMotherboardTemp(data)
	case "storage_smart":
		return convertStorageSmart(data)
	default:
		return nil
	}
}

// unmarshalData converts interface{} data to a specific type via type assertion or JSON fallback.
func unmarshalData[T any](data interface{}) (*T, bool) {
	if v, ok := data.(*T); ok {
		return v, true
	}
	if v, ok := data.(T); ok {
		return &v, true
	}
	b, err := json.Marshal(data)
	if err != nil {
		return nil, false
	}
	var result T
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, false
	}
	return &result, true
}

func systemRow(ts time.Time, category, metric string, value float64) EARSRow {
	return EARSRow{
		Timestamp: ts,
		Category:  category,
		PID:       0,
		ProcName:  "@system",
		Metric:    metric,
		Value:     value,
	}
}

func convertCPU(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.CPUData](data.Data)
	if !ok {
		return nil
	}
	return []EARSRow{systemRow(data.Timestamp, "cpu", "total_used_pct", d.UsagePercent)}
}

func convertMemory(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.MemoryData](data.Data)
	if !ok {
		return nil
	}
	return []EARSRow{
		systemRow(data.Timestamp, "memory", "total_used_pct", d.UsagePercent),
		systemRow(data.Timestamp, "memory", "total_free_pct", 100-d.UsagePercent),
		systemRow(data.Timestamp, "memory", "total_used_size", float64(d.TotalBytes)),
	}
}

func convertDisk(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.DiskData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Partitions))
	for _, p := range d.Partitions {
		rows = append(rows, systemRow(data.Timestamp, "disk", p.Mountpoint, p.UsagePercent))
	}
	return rows
}

func convertNetwork(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.NetworkData](data.Data)
	if !ok {
		return nil
	}
	var totalRecv, totalSent uint64
	for _, iface := range d.Interfaces {
		totalRecv += iface.BytesRecv
		totalSent += iface.BytesSent
	}
	return []EARSRow{
		systemRow(data.Timestamp, "network", "all_inbound", float64(totalRecv)),
		systemRow(data.Timestamp, "network", "all_outbound", float64(totalSent)),
	}
}

func convertCPUProcess(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.ProcessCPUData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Processes))
	for _, p := range d.Processes {
		rows = append(rows, EARSRow{
			Timestamp: data.Timestamp,
			Category:  "cpu",
			PID:       int(p.PID),
			ProcName:  p.Name,
			Metric:    "used_pct",
			Value:     p.CPUPercent,
		})
	}
	return rows
}

func convertMemoryProcess(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.ProcessMemoryData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Processes))
	for _, p := range d.Processes {
		rows = append(rows, EARSRow{
			Timestamp: data.Timestamp,
			Category:  "memory",
			PID:       int(p.PID),
			ProcName:  p.Name,
			Metric:    "used",
			Value:     float64(p.RSS),
		})
	}
	return rows
}

func convertTemperature(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.TemperatureData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Sensors))
	for _, s := range d.Sensors {
		rows = append(rows, systemRow(data.Timestamp, "temperature", s.Name, s.Temperature))
	}
	return rows
}

func convertGPU(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.GpuData](data.Data)
	if !ok {
		return nil
	}
	var rows []EARSRow
	for _, g := range d.Gpus {
		if g.Temperature != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_temperature", *g.Temperature))
		}
		if g.CoreLoad != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_core_load", *g.CoreLoad))
		}
		if g.MemoryLoad != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_memory_load", *g.MemoryLoad))
		}
	}
	return rows
}

func convertFan(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.FanData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Sensors))
	for _, s := range d.Sensors {
		rows = append(rows, systemRow(data.Timestamp, "fan", s.Name, s.RPM))
	}
	return rows
}

func convertVoltage(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.VoltageData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Sensors))
	for _, s := range d.Sensors {
		rows = append(rows, systemRow(data.Timestamp, "voltage", s.Name, s.Voltage))
	}
	return rows
}

func convertMotherboardTemp(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.MotherboardTempData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Sensors))
	for _, s := range d.Sensors {
		rows = append(rows, systemRow(data.Timestamp, "motherboard_temp", s.Name, s.Temperature))
	}
	return rows
}

func convertStorageSmart(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.StorageSmartData](data.Data)
	if !ok {
		return nil
	}
	var rows []EARSRow
	for _, s := range d.Storages {
		if s.Temperature != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name, *s.Temperature))
		}
	}
	return rows
}
