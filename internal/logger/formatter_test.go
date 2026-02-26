package logger

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestFixedFormatWriter_BasicMessage(t *testing.T) {
	var buf bytes.Buffer
	w := NewFixedFormatWriter(&buf)

	input := map[string]interface{}{
		"level":     "info",
		"time":      "2026-02-26T12:00:00+09:00",
		"component": "main",
		"message":   "Starting ResourceAgent",
		"version":   "dev",
	}
	data, _ := json.Marshal(input)

	n, err := w.Write(data)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, want %d", n, len(data))
	}

	line := buf.String()
	// Check timestamp format
	if !strings.HasPrefix(line, "2026-02-26 12:00:00.000") {
		t.Errorf("timestamp mismatch: got %q", line[:23])
	}
	// Check level
	if !strings.Contains(line, "[INF]") {
		t.Errorf("level not found: %q", line)
	}
	// Check component with padding
	if !strings.Contains(line, "[main           ]") {
		t.Errorf("component not found with padding: %q", line)
	}
	// Check message
	if !strings.Contains(line, "Starting ResourceAgent") {
		t.Errorf("message not found: %q", line)
	}
	// Check extra field
	if !strings.Contains(line, "version=dev") {
		t.Errorf("extra field not found: %q", line)
	}
	// Check newline
	if !strings.HasSuffix(line, "\n") {
		t.Errorf("missing trailing newline: %q", line)
	}
}

func TestFixedFormatWriter_ErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	w := NewFixedFormatWriter(&buf)

	input := map[string]interface{}{
		"level":     "error",
		"time":      "2026-02-26T12:00:01.200+09:00",
		"component": "windows-service",
		"message":   "Service failed",
		"caller":    "service/windows.go:42",
		"err":       "VirtualAddressList empty",
	}
	data, _ := json.Marshal(input)

	w.Write(data)
	line := buf.String()

	if !strings.Contains(line, "[ERR]") {
		t.Errorf("error level not found: %q", line)
	}
	if !strings.Contains(line, "[windows-service]") {
		t.Errorf("component not found: %q", line)
	}
	// caller should be excluded
	if strings.Contains(line, "caller=") {
		t.Errorf("caller should be excluded: %q", line)
	}
	if !strings.Contains(line, "err=") {
		t.Errorf("err field not found: %q", line)
	}
}

func TestFixedFormatWriter_NoExtraFields(t *testing.T) {
	var buf bytes.Buffer
	w := NewFixedFormatWriter(&buf)

	input := map[string]interface{}{
		"level":     "info",
		"time":      "2026-02-26T12:00:00Z",
		"component": "main",
		"message":   "Agent stopped",
	}
	data, _ := json.Marshal(input)

	w.Write(data)
	line := buf.String()

	// Should end with message + newline, no trailing space
	expected := "Agent stopped\n"
	if !strings.HasSuffix(line, expected) {
		t.Errorf("expected to end with %q, got %q", expected, line)
	}
}

func TestFixedFormatWriter_LongComponent(t *testing.T) {
	var buf bytes.Buffer
	w := NewFixedFormatWriter(&buf)

	input := map[string]interface{}{
		"level":     "warn",
		"time":      "2026-02-26T12:00:00Z",
		"component": "very-long-component-name",
		"message":   "truncated",
	}
	data, _ := json.Marshal(input)

	w.Write(data)
	line := buf.String()

	// Component should be truncated to 15 chars
	if !strings.Contains(line, "[very-long-compo]") {
		t.Errorf("component not truncated properly: %q", line)
	}
}

func TestFixedFormatWriter_InvalidJSON(t *testing.T) {
	var buf bytes.Buffer
	w := NewFixedFormatWriter(&buf)

	input := []byte("not json at all\n")
	n, err := w.Write(input)
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}
	if n != len(input) {
		t.Errorf("Write returned %d, want %d", n, len(input))
	}

	// Should pass through as-is
	if buf.String() != "not json at all\n" {
		t.Errorf("invalid JSON not passed through: %q", buf.String())
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"RFC3339 with timezone", "2026-02-26T12:00:00+09:00", "2026-02-26 12:00:00.000"},
		{"RFC3339 UTC", "2026-02-26T12:00:00Z", "2026-02-26 12:00:00.000"},
		{"With milliseconds", "2026-02-26T12:00:00.123+09:00", "2026-02-26 12:00:00.123"},
		{"With microseconds", "2026-02-26T12:00:00.123456+09:00", "2026-02-26 12:00:00.123"},
		{"With single digit frac", "2026-02-26T12:00:00.1Z", "2026-02-26 12:00:00.100"},
		{"Empty", "", "                       "},
		{"Negative timezone", "2026-02-26T12:00:00-05:00", "2026-02-26 12:00:00.000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTimestamp(tt.input)
			if got != tt.want {
				t.Errorf("formatTimestamp(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if len(got) != 23 {
				t.Errorf("formatTimestamp(%q) length = %d, want 23", tt.input, len(got))
			}
		})
	}
}

func TestFormatExtra_Sorted(t *testing.T) {
	fields := map[string]interface{}{
		"z_field": "last",
		"a_field": "first",
		"m_field": "middle",
	}
	got := formatExtra(fields)
	if got != "a_field=first m_field=middle z_field=last" {
		t.Errorf("formatExtra not sorted: %q", got)
	}
}

func TestFormatExtra_QuotedValues(t *testing.T) {
	fields := map[string]interface{}{
		"err": "connection refused by host",
	}
	got := formatExtra(fields)
	if got != `err="connection refused by host"` {
		t.Errorf("space-containing value not quoted: %q", got)
	}
}

func TestFormatExtra_Empty(t *testing.T) {
	got := formatExtra(map[string]interface{}{})
	if got != "" {
		t.Errorf("empty fields should return empty string: %q", got)
	}
}

func TestFixedFormatWriter_AllLevels(t *testing.T) {
	levels := map[string]string{
		"trace": "TRC",
		"debug": "DBG",
		"info":  "INF",
		"warn":  "WRN",
		"error": "ERR",
		"fatal": "FTL",
		"panic": "PNC",
	}
	for level, abbr := range levels {
		t.Run(level, func(t *testing.T) {
			var buf bytes.Buffer
			w := NewFixedFormatWriter(&buf)
			input := map[string]interface{}{
				"level":   level,
				"time":    "2026-02-26T12:00:00Z",
				"message": "test",
			}
			data, _ := json.Marshal(input)
			w.Write(data)
			if !strings.Contains(buf.String(), "["+abbr+"]") {
				t.Errorf("level %s: expected [%s] in %q", level, abbr, buf.String())
			}
		})
	}
}
