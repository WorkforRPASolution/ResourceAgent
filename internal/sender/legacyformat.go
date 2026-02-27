package sender

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"resourceagent/internal/collector"
)

var unsafeCharsRe = regexp.MustCompile(`[^a-zA-Z0-9_:.@\-]`)
var multiUnderscoreRe = regexp.MustCompile(`_{2,}`)

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
	var b strings.Builder
	b.Grow(23) // "2006-01-02 15:04:05,000"
	b.WriteString(t.Format(legacyTimeFmt))
	b.WriteByte(',')
	writeMillis(&b, t)
	return b.String()
}

// FormatJSONTimestamp formats to "2006-01-02T15:04:05.000" (JSON mapper compatible).
func FormatJSONTimestamp(t time.Time) string {
	var b strings.Builder
	b.Grow(23) // "2006-01-02T15:04:05.000"
	b.WriteString(t.Format(jsonTimeFmt))
	b.WriteByte('.')
	writeMillis(&b, t)
	return b.String()
}

// writeMillis writes zero-padded milliseconds (000-999) to the builder.
func writeMillis(b *strings.Builder, t time.Time) {
	ms := t.Nanosecond() / 1e6
	b.WriteByte(byte('0' + ms/100))
	b.WriteByte(byte('0' + (ms/10)%10))
	b.WriteByte(byte('0' + ms%10))
}

// sanitizeName replaces special characters in field values
// to ensure safe downstream processing (ES insertion).
// Parentheses are removed (common in hardware names like "Intel(R)").
// Other unsafe chars → '_', then collapse consecutive '_'.
// Keeps: [a-zA-Z0-9_:.@-]
func sanitizeName(s string) string {
	s = strings.NewReplacer("(", "", ")", "").Replace(s)
	s = unsafeCharsRe.ReplaceAllString(s, "_")
	s = multiUnderscoreRe.ReplaceAllString(s, "_")
	return s
}

// formatValue formats a float64 without scientific notation.
func formatValue(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// ToLegacyString returns the ARSAgent-compatible plain text format for Grok parsing.
func (r EARSRow) ToLegacyString() string {
	var b strings.Builder
	b.Grow(128)
	b.WriteString(FormatLegacyTimestamp(r.Timestamp))
	b.WriteString(" category:")
	b.WriteString(r.Category)
	b.WriteString(",pid:")
	b.WriteString(strconv.Itoa(r.PID))
	b.WriteString(",proc:")
	b.WriteString(sanitizeName(r.ProcName))
	b.WriteString(",metric:")
	b.WriteString(sanitizeName(r.Metric))
	b.WriteString(",value:")
	b.WriteString(formatValue(r.Value))
	return b.String()
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
	case "CPU":
		return convertCPU(data)
	case "Memory":
		return convertMemory(data)
	case "Disk":
		return convertDisk(data)
	case "Network":
		return convertNetwork(data)
	case "CPUProcess":
		return convertCPUProcess(data)
	case "MemoryProcess":
		return convertMemoryProcess(data)
	case "Temperature":
		return convertTemperature(data)
	case "GPU":
		return convertGPU(data)
	case "Fan":
		return convertFan(data)
	case "Voltage":
		return convertVoltage(data)
	case "MotherboardTemp":
		return convertMotherboardTemp(data)
	case "StorageSmart":
		return convertStorageSmart(data)
	case "Uptime":
		return convertUptime(data)
	case "ProcessWatch":
		return convertProcessWatch(data)
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
		systemRow(data.Timestamp, "memory", "total_used_size", float64(d.UsedBytes)),
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
	rows := []EARSRow{
		systemRow(data.Timestamp, "network", "all_inbound", float64(d.TCPInboundCount)),
		systemRow(data.Timestamp, "network", "all_outbound", float64(d.TCPOutboundCount)),
	}
	for _, iface := range d.Interfaces {
		rows = append(rows, EARSRow{
			Timestamp: data.Timestamp,
			Category:  "network",
			PID:       0,
			ProcName:  iface.Name,
			Metric:    "recv_rate",
			Value:     iface.BytesRecvRate,
		})
		rows = append(rows, EARSRow{
			Timestamp: data.Timestamp,
			Category:  "network",
			PID:       0,
			ProcName:  iface.Name,
			Metric:    "sent_rate",
			Value:     iface.BytesSentRate,
		})
	}
	return rows
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
		if g.FanSpeed != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_fan_speed", *g.FanSpeed))
		}
		if g.Power != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_power", *g.Power))
		}
		if g.CoreClock != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_core_clock", *g.CoreClock))
		}
		if g.MemoryClock != nil {
			rows = append(rows, systemRow(data.Timestamp, "gpu", g.Name+"_memory_clock", *g.MemoryClock))
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
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_temperature", *s.Temperature))
		}
		if s.RemainingLife != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_remaining_life", *s.RemainingLife))
		}
		if s.MediaErrors != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_media_errors", float64(*s.MediaErrors)))
		}
		if s.PowerCycles != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_power_cycles", float64(*s.PowerCycles)))
		}
		if s.UnsafeShutdowns != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_unsafe_shutdowns", float64(*s.UnsafeShutdowns)))
		}
		if s.PowerOnHours != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_power_on_hours", float64(*s.PowerOnHours)))
		}
		if s.TotalBytesWritten != nil {
			rows = append(rows, systemRow(data.Timestamp, "storage_smart", s.Name+"_total_bytes_written", float64(*s.TotalBytesWritten)))
		}
	}
	return rows
}

func convertProcessWatch(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.ProcessWatchData](data.Data)
	if !ok {
		return nil
	}
	rows := make([]EARSRow, 0, len(d.Statuses))
	for _, s := range d.Statuses {
		value := 0.0
		if s.Running {
			value = 1.0
		}
		metric := processWatchMetric(s.Type, s.Running)
		rows = append(rows, EARSRow{
			Timestamp: data.Timestamp,
			Category:  "process_watch",
			PID:       int(s.PID),
			ProcName:  s.Name,
			Metric:    metric,
			Value:     value,
		})
	}
	return rows
}

// processWatchMetric returns the EARS metric name with _alert suffix for anomalous states.
//   - required + NOT running → required_alert
//   - forbidden + running   → forbidden_alert
func processWatchMetric(typ string, running bool) string {
	isAlert := (typ == "required" && !running) || (typ == "forbidden" && running)
	if isAlert {
		return typ + "_alert"
	}
	return typ
}

func convertUptime(data *collector.MetricData) []EARSRow {
	d, ok := unmarshalData[collector.UptimeData](data.Data)
	if !ok {
		return nil
	}
	return []EARSRow{
		systemRow(data.Timestamp, "uptime", "boot_time_unix", float64(d.BootTimeUnix)),
		systemRow(data.Timestamp, "uptime", "uptime_minutes", d.UptimeMinutes),
	}
}
