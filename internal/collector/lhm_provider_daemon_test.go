//go:build windows

package collector

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// fake_daemon binary is built once per test process and shared across
// all tests. Per-test rebuilds caused intermittent hangs on Windows when
// AV/EDR scanned each freshly written .exe, holding the file handle open
// long enough that `go build` itself never returned.
var (
	fakeDaemonOnce sync.Once
	fakeDaemonPath string
	fakeDaemonErr  error
	fakeDaemonDir  string
)

// buildFakeDaemon compiles the fake_daemon.go helper once per process and
// returns the cached path on subsequent calls.
func buildFakeDaemon(t *testing.T) string {
	t.Helper()
	fakeDaemonOnce.Do(func() {
		ext := ""
		if runtime.GOOS == "windows" {
			ext = ".exe"
		}
		// Use a process-lifetime directory so the binary stays alive across
		// tests. We deliberately don't t.TempDir() — that's per-test and
		// disappears as soon as the first test ends.
		dir, err := os.MkdirTemp("", "ra-fake-daemon-*")
		if err != nil {
			fakeDaemonErr = fmt.Errorf("mkdtemp: %w", err)
			return
		}
		fakeDaemonDir = dir
		out := filepath.Join(dir, "fake_daemon"+ext)
		src := filepath.Join("testdata", "fake_daemon.go")

		// Cap the build to 60s so a wedged AV scan surfaces as a clear test
		// failure rather than the global testing timeout panic.
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-o", out, src)
		if output, err := cmd.CombinedOutput(); err != nil {
			fakeDaemonErr = fmt.Errorf("go build: %w\n%s", err, output)
			return
		}
		fakeDaemonPath = out
	})
	if fakeDaemonErr != nil {
		t.Fatalf("failed to build fake daemon: %v", fakeDaemonErr)
	}
	return fakeDaemonPath
}

// newTestProvider creates a fresh LhmProvider for testing (not the singleton).
func newTestProvider(helperPath string) *LhmProvider {
	return &LhmProvider{
		cacheTTL:       5 * time.Second,
		requestTimeout: 3 * time.Second,
		helperPath:     helperPath,
		helperFound:    true,
	}
}

// wireSlowDaemonForTest spawns the fake daemon via os.Pipe() and mounts the
// pipes onto p, mirroring what startProcess does. Tests use this when they
// need the "slow" mode (which causes startProcess's initial doRequest to
// time out and roll back), so direct wiring is required.
func wireSlowDaemonForTest(t *testing.T, p *LhmProvider, fakePath string) {
	t.Helper()
	cmd := exec.Command(fakePath, "--daemon")

	childStdinR, parentStdinW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdin: %v", err)
	}
	parentStdoutR, childStdoutW, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe stdout: %v", err)
	}
	cmd.Stdin = childStdinR
	cmd.Stdout = childStdoutW
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("StderrPipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start slow daemon: %v", err)
	}
	// Drop our copies of the child ends.
	childStdinR.Close()
	childStdoutW.Close()

	p.cmd = cmd
	p.stdinFile = parentStdinW
	p.stdoutFile = parentStdoutR
	p.stdoutReader = bufio.NewReaderSize(parentStdoutR, 256*1024)
	p.stderr = stderr
	p.processExit = make(chan struct{})
	go func() {
		cmd.Wait()
		close(p.processExit)
	}()
}

func TestDaemonProtocol(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	// Set env for normal mode
	t.Setenv("FAKE_DAEMON_MODE", "normal")

	if err := p.startProcess(); err != nil {
		t.Fatalf("startProcess failed: %v", err)
	}
	defer p.stopProcess()

	// Initial data should be cached from startProcess
	if p.data == nil {
		t.Fatal("expected cached data after startProcess")
	}
	if len(p.data.Sensors) != 1 {
		t.Errorf("expected 1 sensor, got %d", len(p.data.Sensors))
	}
	if p.data.Sensors[0].Name != "CPU Package" {
		t.Errorf("expected sensor name 'CPU Package', got %q", p.data.Sensors[0].Name)
	}
	if len(p.data.Fans) != 1 {
		t.Errorf("expected 1 fan, got %d", len(p.data.Fans))
	}

	// Second request
	data, err := p.doRequest()
	if err != nil {
		t.Fatalf("doRequest failed: %v", err)
	}
	if len(data.Sensors) != 1 {
		t.Errorf("expected 1 sensor on second request, got %d", len(data.Sensors))
	}
}

func TestDaemonCacheHit(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	t.Setenv("FAKE_DAEMON_MODE", "normal")

	if err := p.startProcess(); err != nil {
		t.Fatalf("startProcess failed: %v", err)
	}
	defer p.stopProcess()

	// GetData should return cached data (within TTL)
	data1, err := p.GetData(ctx)
	if err != nil {
		t.Fatalf("GetData failed: %v", err)
	}

	// Immediately call again — should get same pointer (cache hit)
	data2, err := p.GetData(ctx)
	if err != nil {
		t.Fatalf("second GetData failed: %v", err)
	}

	if data1 != data2 {
		t.Error("expected cache hit to return same pointer")
	}
}

func TestDaemonProcessCrash(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	// "crash" mode: respond once then exit on second request
	t.Setenv("FAKE_DAEMON_MODE", "crash")

	if err := p.startProcess(); err != nil {
		t.Fatalf("startProcess failed: %v", err)
	}

	// First GetData uses cached data from startProcess — should succeed
	data, err := p.GetData(ctx)
	if err != nil {
		t.Fatalf("first GetData failed: %v", err)
	}
	if len(data.Sensors) != 1 {
		t.Errorf("expected 1 sensor, got %d", len(data.Sensors))
	}

	// Expire cache to force new request
	p.mu.Lock()
	p.lastUpdate = time.Time{}
	p.mu.Unlock()

	// Crash daemon exits immediately after first response (during startProcess).
	// Wait for the process to actually exit before calling GetData.
	select {
	case <-p.processExit:
	case <-time.After(5 * time.Second):
		t.Fatal("crash daemon did not exit in time")
	}

	// Now switch env to normal so restart succeeds
	t.Setenv("FAKE_DAEMON_MODE", "normal")

	// This GetData should detect dead process, restart, and succeed
	data, err = p.GetData(ctx)
	if err != nil {
		t.Fatalf("GetData after crash should restart and succeed: %v", err)
	}
	if len(data.Sensors) != 1 {
		t.Errorf("expected 1 sensor after restart, got %d", len(data.Sensors))
	}

	// Cleanup: stop restarted daemon so Windows can release fake_daemon.exe handle
	// before t.TempDir cleanup (otherwise "Access is denied" on Windows).
	p.stopProcess()
}

func TestDaemonBackoff(t *testing.T) {
	// Use a non-existent path to trigger start failures
	p := newTestProvider("/nonexistent/fake_daemon.exe")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	// First failure
	p.consecutiveFailures = 0
	p.lastStartAttempt = time.Now().Add(-10 * time.Second) // Allow immediate start

	err := p.restartWithBackoff()
	if err == nil {
		t.Fatal("expected error from restart with bad path")
	}
	if p.consecutiveFailures != 1 {
		t.Errorf("expected consecutiveFailures=1, got %d", p.consecutiveFailures)
	}

	// Second failure — backoff should be 2s but we won't wait that long.
	// Just verify the counter increments.
	p.lastStartAttempt = time.Now().Add(-10 * time.Second)
	err = p.restartWithBackoff()
	if err == nil {
		t.Fatal("expected error from restart")
	}
	if p.consecutiveFailures != 2 {
		t.Errorf("expected consecutiveFailures=2, got %d", p.consecutiveFailures)
	}
}

func TestDaemonGracefulShutdown(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	t.Setenv("FAKE_DAEMON_MODE", "normal")

	if err := p.startProcess(); err != nil {
		t.Fatalf("startProcess failed: %v", err)
	}

	// Verify process is alive
	if !p.isProcessAlive() {
		t.Fatal("expected process to be alive")
	}

	// Stop should close stdin and wait for exit
	p.stopProcess()

	if p.isProcessAlive() {
		t.Error("expected process to be dead after stop")
	}
	if p.cmd != nil {
		t.Error("expected cmd to be nil after stop")
	}
}

func TestDaemonTimeout(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)
	p.requestTimeout = 500 * time.Millisecond // Short timeout for test

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	// "slow" mode: reads stdin but never responds
	t.Setenv("FAKE_DAEMON_MODE", "slow")
	wireSlowDaemonForTest(t, p, fakePath)

	// doRequestWithTimeout should fail
	_, err := p.doRequestWithTimeout(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}

	// Clean up
	p.stopProcess()
}

func TestDaemonConcurrentGetData(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	p.ctx, p.cancel = ctx, cancel
	p.started = true

	t.Setenv("FAKE_DAEMON_MODE", "normal")

	if err := p.startProcess(); err != nil {
		t.Fatalf("startProcess failed: %v", err)
	}
	defer p.stopProcess()

	// Run 10 concurrent GetData calls (simulates 6 collectors)
	var wg sync.WaitGroup
	errors := make(chan error, 10)

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			data, err := p.GetData(ctx)
			if err != nil {
				errors <- err
				return
			}
			if len(data.Sensors) != 1 {
				errors <- fmt.Errorf("expected 1 sensor, got %d", len(data.Sensors))
			}
		}()
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Errorf("concurrent GetData error: %v", err)
	}
}

// TestDoRequestTimeout_NoGoroutineLeak verifies that a request timeout does
// not leak the worker goroutine spawned by doRequestWithTimeout. The first
// request times out, kills the daemon, and drains the worker; subsequent
// requests find broken pipes and return immediately without spawning new
// long-running goroutines.
func TestDoRequestTimeout_NoGoroutineLeak(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)
	p.requestTimeout = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.ctx, p.cancel = ctx, cancel
	p.started = true

	// "slow" mode: reads stdin but never responds → guaranteed timeout.
	t.Setenv("FAKE_DAEMON_MODE", "slow")
	wireSlowDaemonForTest(t, p, fakePath)
	defer p.stopProcess()

	before := runtime.NumGoroutine()

	// Trigger the timeout path. killAndDrain must reap the worker
	// synchronously — by the time this call returns there should be no
	// extra long-running goroutine attributable to LhmProvider.
	if _, err := p.doRequestWithTimeout(ctx); err == nil {
		t.Fatal("expected timeout error under slow mode")
	}

	// A handful of follow-up calls hit a broken pipe (LhmHelper killed) and
	// must short-circuit without spawning workers that linger.
	for i := 0; i < 20; i++ {
		_, _ = p.doRequestWithTimeout(ctx)
	}

	// Allow OS resources / worker exits a brief window to settle.
	time.Sleep(200 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Tolerate small scheduler jitter. The previous goroutine-per-request
	// design would leak ~21 here.
	if delta := after - before; delta > 5 {
		t.Errorf("goroutine leak detected: before=%d after=%d delta=%d",
			before, after, delta)
	}
}

// TestDoRequestTimeout_KillsDaemonOnTimeout verifies that a request timeout
// triggers Process.Kill so the next GetData is forced to spawn a fresh
// daemon. This is the entire option-B mechanism: timeout → kill → restart.
func TestDoRequestTimeout_KillsDaemonOnTimeout(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)
	p.requestTimeout = 200 * time.Millisecond

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.ctx, p.cancel = ctx, cancel
	p.started = true

	t.Setenv("FAKE_DAEMON_MODE", "slow")
	wireSlowDaemonForTest(t, p, fakePath)

	if _, err := p.doRequestWithTimeout(ctx); err == nil {
		t.Fatal("expected timeout error under slow mode")
	}

	// processExit should have been closed by killAndDrain → cmd.Wait observer.
	select {
	case <-p.processExit:
		// expected
	case <-time.After(3 * time.Second):
		t.Fatal("expected LhmHelper process to be killed after timeout")
	}

	// Each timeout increments the diagnostic counter (used by operators to
	// detect "kill alone won't help" sustained-hang scenarios).
	if p.consecutiveTimeouts == 0 {
		t.Error("expected consecutiveTimeouts to be incremented on timeout")
	}
}

// TestDoRequestTimeout_RecoveryResetsCounter verifies the diagnostic counter
// is cleared when a normal response arrives after one or more timeouts.
func TestDoRequestTimeout_RecoveryResetsCounter(t *testing.T) {
	fakePath := buildFakeDaemon(t)
	p := newTestProvider(fakePath)
	p.requestTimeout = 2 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.ctx, p.cancel = ctx, cancel
	p.started = true

	// Start in normal mode for a successful baseline call.
	t.Setenv("FAKE_DAEMON_MODE", "normal")
	if err := p.startProcess(); err != nil {
		t.Fatalf("startProcess failed: %v", err)
	}
	defer p.stopProcess()

	// Force the counter as if prior timeouts had occurred.
	p.consecutiveTimeouts = 2

	// A successful request should reset the counter back to 0.
	if _, err := p.doRequestWithTimeout(ctx); err != nil {
		t.Fatalf("doRequestWithTimeout failed under normal mode: %v", err)
	}
	if p.consecutiveTimeouts != 0 {
		t.Errorf("expected consecutiveTimeouts=0 after success, got %d", p.consecutiveTimeouts)
	}
}
