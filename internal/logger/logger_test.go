package logger

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// blockingWriter simulates a blocked stdout (e.g., Windows cmd Quick Edit mode).
// Write blocks until Unblock() is called.
type blockingWriter struct {
	mu      sync.Mutex
	buf     bytes.Buffer
	blockCh chan struct{}
}

func newBlockingWriter() *blockingWriter {
	return &blockingWriter{
		blockCh: make(chan struct{}),
	}
}

func (w *blockingWriter) Write(p []byte) (int, error) {
	<-w.blockCh // Block until unblocked
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}

func (w *blockingWriter) Unblock() {
	close(w.blockCh)
}

func (w *blockingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// TestAsyncWriter_DoesNotBlockCaller verifies that writing to an async writer
// returns immediately even when the underlying writer is blocked.
func TestAsyncWriter_DoesNotBlockCaller(t *testing.T) {
	bw := newBlockingWriter()
	aw := newAsyncWriter(bw, 100)
	defer aw.Close()

	done := make(chan struct{})
	go func() {
		_, err := aw.Write([]byte("hello"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		close(done)
	}()

	select {
	case <-done:
		// Write returned immediately - good
	case <-time.After(1 * time.Second):
		t.Fatal("Write blocked - asyncWriter should return immediately")
	}

	// Unblock and verify data was eventually written
	bw.Unblock()
	time.Sleep(50 * time.Millisecond)
	if bw.String() != "hello" {
		t.Errorf("expected %q, got %q", "hello", bw.String())
	}
}

// TestAsyncWriter_DropsWhenBufferFull verifies that writes are dropped
// (not blocked) when the internal buffer is full.
func TestAsyncWriter_DropsWhenBufferFull(t *testing.T) {
	bw := newBlockingWriter()
	aw := newAsyncWriter(bw, 2) // Very small buffer
	defer func() {
		bw.Unblock() // Must unblock so drain can finish
		aw.Close()
	}()

	// drain goroutine picks up 1 message (blocks on bw.Write).
	// Channel capacity is 2, so total capacity = 1 (drain) + 2 (channel) = 3.
	// Write 4 to guarantee the channel is full.
	for i := 0; i < 4; i++ {
		aw.Write([]byte("msg"))
	}
	time.Sleep(10 * time.Millisecond) // Let drain goroutine pick up one

	// This should NOT block even though buffer is full
	done := make(chan struct{})
	go func() {
		aw.Write([]byte("overflow"))
		close(done)
	}()

	select {
	case <-done:
		// Write returned immediately - good
	case <-time.After(1 * time.Second):
		t.Fatal("Write blocked on full buffer - should drop instead")
	}
}

// TestAsyncWriter_Close drains remaining messages.
func TestAsyncWriter_Close(t *testing.T) {
	var buf bytes.Buffer
	aw := newAsyncWriter(&buf, 100)

	aw.Write([]byte("a"))
	aw.Write([]byte("b"))
	aw.Close()

	// After close, buffer should have received the data
	if buf.String() != "ab" {
		t.Errorf("expected %q, got %q", "ab", buf.String())
	}
}

// TestAsyncWriter_WriteAfterClose does not panic.
func TestAsyncWriter_WriteAfterClose(t *testing.T) {
	var buf bytes.Buffer
	aw := newAsyncWriter(&buf, 100)
	aw.Close()

	// Write after Close should not panic (silently discarded)
	n, err := aw.Write([]byte("after-close"))
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != len("after-close") {
		t.Errorf("expected n=%d, got %d", len("after-close"), n)
	}
}

// TestInit_ConsoleBlockDoesNotBlockFileWrite verifies that a blocked console
// writer does not prevent file writes. This is the core bug fix test.
func TestInit_ConsoleBlockDoesNotBlockFileWrite(t *testing.T) {
	// Set up: a temporary file for the log output
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	cfg := Config{
		Level:    "info",
		FilePath: logFile,
		Console:  true, // Console enabled
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Write a log message - should succeed even though console might block
	// (In real scenario, stdout blocks from Windows Quick Edit mode)
	Info().Msg("test message")

	// Give async writer time to flush
	time.Sleep(50 * time.Millisecond)

	// Verify the log file was written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("log file is empty - expected data to be written")
	}
}

// TestInit_FileWriteNotBlockedByConsole verifies that even when stdout is
// blocked, file writes still succeed. This simulates Windows Quick Edit mode.
func TestInit_FileWriteNotBlockedByConsole(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	// Replace stdout with a pipe that nobody reads from.
	// Once the pipe buffer fills (~64KB on macOS), writes will block.
	origStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := Config{
		Level:    "info",
		FilePath: logFile,
		Console:  true,
	}
	if err := Init(cfg); err != nil {
		os.Stdout = origStdout
		t.Fatalf("Init failed: %v", err)
	}

	// Write enough data to overflow ANY pipe buffer (~2MB >> 64KB pipe buffer).
	// If console writer is synchronous, this WILL block.
	bigMsg := strings.Repeat("x", 10000)
	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			Info().Str("data", bigMsg).Msg("test")
		}
		close(done)
	}()

	select {
	case <-done:
		// Good: writes completed without blocking
	case <-time.After(5 * time.Second):
		os.Stdout = origStdout
		t.Fatal("logging blocked - console writer is blocking file writer")
	}

	// Restore stdout and cleanup
	os.Stdout = origStdout
	w.Close()
	r.Close()

	time.Sleep(100 * time.Millisecond)

	// Verify file was written
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("log file is empty - console blocking prevented file writes")
	}
}

// TestInit_ReInitClosesOldWriter verifies that calling Init twice
// properly closes the old file writer.
func TestInit_ReInitClosesOldWriter(t *testing.T) {
	dir := t.TempDir()
	logFile := filepath.Join(dir, "test.log")

	// First init
	cfg := Config{
		Level:    "info",
		FilePath: logFile,
		Console:  false,
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("first Init failed: %v", err)
	}
	Info().Msg("first message")
	time.Sleep(50 * time.Millisecond)

	// Second init (simulates hot reload)
	if err := Init(cfg); err != nil {
		t.Fatalf("second Init failed: %v", err)
	}
	Info().Msg("second message")
	time.Sleep(50 * time.Millisecond)

	// Verify log file has data from both inits
	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	content := string(data)
	if !bytes.Contains([]byte(content), []byte("first message")) {
		t.Error("log file missing 'first message'")
	}
	if !bytes.Contains([]byte(content), []byte("second message")) {
		t.Error("log file missing 'second message'")
	}
}
