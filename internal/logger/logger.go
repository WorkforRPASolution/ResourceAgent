// Package logger provides structured logging with file rotation support.
package logger

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds the logger configuration.
type Config struct {
	Level      string `json:"Level"`
	FilePath   string `json:"FilePath"`
	MaxSizeMB  int    `json:"MaxSizeMB"`
	MaxBackups int    `json:"MaxBackups"`
	MaxAgeDays int    `json:"MaxAgeDays"`
	Compress   bool   `json:"Compress"`
	Console    bool   `json:"Console"`
}

// DefaultConfig returns sensible defaults for logging.
func DefaultConfig() Config {
	return Config{
		Level:      "info",
		FilePath:   "logs/agent.log",
		MaxSizeMB:  10,
		MaxBackups: 5,
		MaxAgeDays: 30,
		Compress:   true,
		Console:    false,
	}
}

var globalLogger zerolog.Logger

// Init initializes the global logger with the given configuration.
func Init(cfg Config) error {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339

	var writers []io.Writer

	// File output with rotation
	if cfg.FilePath != "" {
		dir := filepath.Dir(cfg.FilePath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}

		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.MaxSizeMB,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAgeDays,
			Compress:   cfg.Compress,
		}
		writers = append(writers, fileWriter)
	}

	// Console output
	if cfg.Console {
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		writers = append(writers, consoleWriter)
	}

	// Default to stdout if no writers configured
	if len(writers) == 0 {
		writers = append(writers, os.Stdout)
	}

	var output io.Writer
	if len(writers) == 1 {
		output = writers[0]
	} else {
		output = zerolog.MultiLevelWriter(writers...)
	}

	globalLogger = zerolog.New(output).With().Timestamp().Caller().Logger()
	return nil
}

// Logger returns the global logger instance.
func Logger() *zerolog.Logger {
	return &globalLogger
}

// Debug logs a debug message.
func Debug() *zerolog.Event {
	return globalLogger.Debug()
}

// Info logs an info message.
func Info() *zerolog.Event {
	return globalLogger.Info()
}

// Warn logs a warning message.
func Warn() *zerolog.Event {
	return globalLogger.Warn()
}

// Error logs an error message.
func Error() *zerolog.Event {
	return globalLogger.Error()
}

// Fatal logs a fatal message and exits.
func Fatal() *zerolog.Event {
	return globalLogger.Fatal()
}

// WithComponent returns a logger with component field.
func WithComponent(component string) zerolog.Logger {
	return globalLogger.With().Str("component", component).Logger()
}
