package logger

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// FixedFormatWriter wraps an io.Writer and converts zerolog JSON output
// into a fixed-width column format for better readability in log files.
//
// Output format:
//
//	2026-02-26 12:00:00.000 [INF] [main           ] Starting ResourceAgent version=dev
//	2026-02-26 12:00:01.200 [ERR] [windows-service] Service failed err="connection refused"
type FixedFormatWriter struct {
	w io.Writer
}

// NewFixedFormatWriter creates a new FixedFormatWriter that wraps the given writer.
func NewFixedFormatWriter(w io.Writer) *FixedFormatWriter {
	return &FixedFormatWriter{w: w}
}

var levelMap = map[string]string{
	"trace": "TRC",
	"debug": "DBG",
	"info":  "INF",
	"warn":  "WRN",
	"error": "ERR",
	"fatal": "FTL",
	"panic": "PNC",
}

const componentWidth = 15

func (f *FixedFormatWriter) Write(p []byte) (int, error) {
	var fields map[string]interface{}
	if err := json.Unmarshal(p, &fields); err != nil {
		// Not valid JSON — pass through as-is
		return f.w.Write(p)
	}

	// Extract known fields
	timestamp := extractString(fields, "time")
	level := extractString(fields, "level")
	component := extractString(fields, "component")
	message := extractString(fields, "message")

	// Remove known fields so remaining can be appended as key=value
	delete(fields, "time")
	delete(fields, "level")
	delete(fields, "component")
	delete(fields, "message")
	delete(fields, "caller") // Exclude caller for readability

	// Format timestamp: convert RFC3339 to "2006-01-02 15:04:05.000"
	ts := formatTimestamp(timestamp)

	// Format level
	lvl := levelMap[level]
	if lvl == "" {
		lvl = "???"
	}

	// Format component with fixed width
	comp := component
	if len(comp) > componentWidth {
		comp = comp[:componentWidth]
	}

	// Build extra fields string
	extra := formatExtra(fields)

	var line string
	if extra != "" {
		line = fmt.Sprintf("%s [%s] [%-*s] %s %s\n", ts, lvl, componentWidth, comp, message, extra)
	} else {
		line = fmt.Sprintf("%s [%s] [%-*s] %s\n", ts, lvl, componentWidth, comp, message)
	}

	_, err := f.w.Write([]byte(line))
	// Return original length to satisfy zerolog's expectation
	return len(p), err
}

func extractString(fields map[string]interface{}, key string) string {
	v, ok := fields[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// formatTimestamp converts an RFC3339 or similar timestamp to "2006-01-02 15:04:05.000" format.
func formatTimestamp(ts string) string {
	if len(ts) == 0 {
		return strings.Repeat(" ", 23) // 23-char placeholder
	}

	// Common zerolog formats:
	// "2006-01-02T15:04:05Z07:00" (RFC3339)
	// "2006-01-02T15:04:05.000Z07:00" (RFC3339Nano truncated)
	// Replace T with space, strip timezone
	result := ts

	// Replace 'T' separator with space
	result = strings.Replace(result, "T", " ", 1)

	// Strip timezone suffix (Z, +09:00, -05:00, etc.)
	if idx := strings.IndexAny(result[11:], "Z+-"); idx >= 0 {
		result = result[:11+idx]
	}

	// Ensure milliseconds are present
	dotIdx := strings.LastIndex(result, ".")
	if dotIdx == -1 {
		// No fractional seconds — append .000
		result += ".000"
	} else {
		frac := result[dotIdx+1:]
		switch {
		case len(frac) > 3:
			result = result[:dotIdx+4] // Truncate to 3 digits
		case len(frac) < 3:
			result += strings.Repeat("0", 3-len(frac)) // Pad to 3 digits
		}
	}

	// Pad or truncate to exactly 23 characters
	if len(result) < 23 {
		result += strings.Repeat(" ", 23-len(result))
	} else if len(result) > 23 {
		result = result[:23]
	}

	return result
}

// formatExtra builds a "key=value key2=value2" string from remaining fields.
func formatExtra(fields map[string]interface{}) string {
	if len(fields) == 0 {
		return ""
	}

	// Sort keys for consistent output
	keys := make([]string, 0, len(fields))
	for k := range fields {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		v := fields[k]
		s := fmt.Sprintf("%v", v)
		if strings.ContainsAny(s, " \t\n\"") {
			parts = append(parts, fmt.Sprintf("%s=%q", k, s))
		} else {
			parts = append(parts, fmt.Sprintf("%s=%s", k, s))
		}
	}

	return strings.Join(parts, " ")
}
