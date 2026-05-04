//go:build windows

package collector

import (
	"errors"
	"os"
	"testing"
	"time"
)

// TestPipeDeadlineProbe is a micro-diagnostic that verifies whether the
// Windows Go runtime honours SetReadDeadline / SetWriteDeadline on
// anonymous pipes created via os.Pipe().
//
// This is the prerequisite assumption for Phase 1-1 option A. If this test
// fails on the target Windows host the option-A code path is silently
// degenerate there and we must fall back to option B (Process.Kill on
// timeout) only.
//
// Run explicitly:
//
//	go test -v -run TestPipeDeadlineProbe ./internal/collector/...
//
// Result interpretation:
//   - PASS in <500ms: anonymous pipe deadline is honoured → option A viable.
//   - PASS in much longer than the deadline OR FAIL: deadline is ignored →
//     option A must be removed.
func TestPipeDeadlineProbe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	defer r.Close()
	defer w.Close()

	deadline := time.Now().Add(200 * time.Millisecond)
	if err := r.SetReadDeadline(deadline); err != nil {
		t.Fatalf("SetReadDeadline returned error (deadline unsupported): %v", err)
	}

	buf := make([]byte, 64)
	start := time.Now()
	n, readErr := r.Read(buf)
	elapsed := time.Since(start)

	t.Logf("Read returned after %v: n=%d err=%v", elapsed, n, readErr)

	// We expect the read to fail with a timeout error within ~500ms of
	// the configured deadline. Anything longer means the runtime did not
	// honour the deadline.
	if elapsed > 1500*time.Millisecond {
		t.Fatalf("anonymous pipe ignored SetReadDeadline: elapsed=%v (expected ~200ms)", elapsed)
	}
	if readErr == nil {
		t.Fatalf("expected timeout error but read succeeded with n=%d", n)
	}
	if !os.IsTimeout(readErr) && !errors.Is(readErr, os.ErrDeadlineExceeded) {
		t.Fatalf("expected timeout error, got: %v", readErr)
	}
}

// TestPipeWriteDeadlineProbe mirrors the read probe for write deadlines.
// The hang in TestDaemonTimeout could equally be in stdin.Write if the
// child process never drains its stdin (which is exactly the "slow" mode).
func TestPipeWriteDeadlineProbe(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe failed: %v", err)
	}
	defer r.Close()
	defer w.Close()

	// Fill the pipe buffer first so the next write blocks. Default Windows
	// anonymous pipe buffer is 4096 bytes; write 64KB to be sure.
	junk := make([]byte, 64*1024)
	deadline := time.Now().Add(200 * time.Millisecond)
	if err := w.SetWriteDeadline(deadline); err != nil {
		t.Fatalf("SetWriteDeadline returned error: %v", err)
	}

	start := time.Now()
	n, writeErr := w.Write(junk)
	elapsed := time.Since(start)
	t.Logf("Write returned after %v: n=%d err=%v", elapsed, n, writeErr)

	if elapsed > 1500*time.Millisecond {
		t.Fatalf("anonymous pipe ignored SetWriteDeadline: elapsed=%v (expected ~200ms)", elapsed)
	}
	// Write may or may not error depending on whether the buffer fills before
	// the deadline. We only enforce that it returned in time.
}
