// Package logger provides structured logging with file rotation support.
package logger

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rs/zerolog"
	"gopkg.in/natefinch/lumberjack.v2"
)

// asyncWriter wraps an io.Writer to make writes non-blocking.
// If the underlying writer blocks (e.g., Windows cmd Quick Edit mode),
// the caller's Write returns immediately. Messages are buffered and
// delivered by a background goroutine. If the buffer is full, messages are dropped.
type asyncWriter struct {
	ch     chan []byte
	w      io.Writer
	done   chan struct{}
	once   sync.Once
	mu     sync.RWMutex
	closed bool
}

func newAsyncWriter(w io.Writer, bufSize int) *asyncWriter {
	aw := &asyncWriter{
		ch:   make(chan []byte, bufSize),
		w:    w,
		done: make(chan struct{}),
	}
	go aw.drain()
	return aw
}

func (aw *asyncWriter) Write(p []byte) (int, error) {
	aw.mu.RLock()
	if aw.closed {
		aw.mu.RUnlock()
		return len(p), nil // Silently discard after Close
	}
	cp := make([]byte, len(p))
	copy(cp, p)
	select {
	case aw.ch <- cp:
	default:
		// Drop if buffer full - prevents blocking the caller
	}
	aw.mu.RUnlock()
	return len(p), nil
}

func (aw *asyncWriter) drain() {
	defer close(aw.done)
	for p := range aw.ch {
		aw.w.Write(p)
	}
}

func (aw *asyncWriter) Close() {
	aw.once.Do(func() {
		aw.mu.Lock()
		aw.closed = true
		aw.mu.Unlock()
		close(aw.ch)
		<-aw.done // Wait for drain to finish
	})
}

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

var (
	globalLogger     zerolog.Logger
	prevFileWriter   io.Closer     // Previous file writer to close on re-init
	prevConsoleAsync *asyncWriter  // Previous async console writer to close on re-init
)

// Init initializes the global logger with the given configuration.
func Init(cfg Config) error {
	level, err := zerolog.ParseLevel(cfg.Level)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)
	zerolog.TimeFieldFormat = time.RFC3339

	// Close previous writers from prior Init call (hot reload)
	if prevFileWriter != nil {
		prevFileWriter.Close()
		prevFileWriter = nil
	}
	if prevConsoleAsync != nil {
		prevConsoleAsync.Close()
		prevConsoleAsync = nil
	}

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
		prevFileWriter = fileWriter
		writers = append(writers, fileWriter)
	}

	// Console output (async to prevent stdout blocking from cascading to file writes)
	if cfg.Console {
		consoleWriter := zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: time.RFC3339,
		}
		aw := newAsyncWriter(consoleWriter, 1000)
		prevConsoleAsync = aw
		writers = append(writers, aw)
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
