package sender

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"

	"resourceagent/internal/collector"
	"resourceagent/internal/config"
	"resourceagent/internal/logger"
)

// FileSender writes metrics to a log file and optionally to console.
type FileSender struct {
	filePath    string
	writer      *lumberjack.Logger
	prettyPrint bool
	console     bool
	format      string
	mu          sync.Mutex
	closed      bool
}

// NewFileSender creates a new FileSender with the given configuration.
func NewFileSender(cfg config.FileConfig) (*FileSender, error) {
	log := logger.WithComponent("file-sender")

	// Default format is "legacy"
	format := cfg.Format
	if format == "" {
		format = "legacy"
	}
	if format != "json" && format != "legacy" {
		return nil, fmt.Errorf("unsupported file format %q: must be \"json\" or \"legacy\"", format)
	}

	// Ensure the directory exists
	dir := filepath.Dir(cfg.FilePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	// Set up lumberjack for log rotation
	writer := &lumberjack.Logger{
		Filename:   cfg.FilePath,
		MaxSize:    cfg.MaxSizeMB,
		MaxBackups: cfg.MaxBackups,
		Compress:   true,
	}

	log.Info().
		Str("file_path", cfg.FilePath).
		Str("format", format).
		Bool("console", cfg.Console).
		Bool("pretty", cfg.Pretty).
		Msg("FileSender initialized")

	return &FileSender{
		filePath:    cfg.FilePath,
		writer:      writer,
		prettyPrint: cfg.Pretty,
		console:     cfg.Console,
		format:      format,
	}, nil
}

// Send writes a single metric data to the file and optionally to console.
func (s *FileSender) Send(ctx context.Context, data *collector.MetricData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("sender is closed")
	}

	if s.format == "legacy" {
		return s.sendLegacy(data)
	}
	return s.sendJSON(data)
}

// sendJSON writes metric data as JSON (original behavior).
func (s *FileSender) sendJSON(data *collector.MetricData) error {
	var jsonData []byte
	var err error

	if s.prettyPrint {
		jsonData, err = json.MarshalIndent(data, "", "  ")
	} else {
		jsonData, err = json.Marshal(data)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal metric data: %w", err)
	}

	if _, err := s.writer.Write(append(jsonData, '\n')); err != nil {
		return fmt.Errorf("failed to write to file: %w", err)
	}

	if s.console {
		fmt.Println(string(jsonData))
	}

	return nil
}

// sendLegacy writes metric data in Grok-compatible legacy text format.
func (s *FileSender) sendLegacy(data *collector.MetricData) error {
	rows := ConvertToEARSRows(data)
	for _, row := range rows {
		line := row.ToLegacyString()
		if _, err := s.writer.Write(append([]byte(line), '\n')); err != nil {
			return fmt.Errorf("failed to write to file: %w", err)
		}
		if s.console {
			fmt.Println(line)
		}
	}
	return nil
}

// SendBatch writes multiple metric data items.
func (s *FileSender) SendBatch(ctx context.Context, data []*collector.MetricData) error {
	for _, d := range data {
		if err := s.Send(ctx, d); err != nil {
			return err
		}
	}
	return nil
}

// Close releases resources held by the FileSender.
func (s *FileSender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true
	return s.writer.Close()
}
