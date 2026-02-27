//go:build windows

package collector

import (
	"bufio"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

// buildFakeDaemon compiles the fake_daemon.go helper and returns its path.
func buildFakeDaemon(t *testing.T) string {
	t.Helper()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	out := filepath.Join(t.TempDir(), "fake_daemon"+ext)
	src := filepath.Join("testdata", "fake_daemon.go")
	cmd := exec.Command("go", "build", "-o", out, src)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build fake daemon: %v\n%s", err, output)
	}
	return out
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

	// Process will crash on second doRequest (fake daemon exits after responding once).
	// The crash daemon responds to the first stdin line (during startProcess initial request),
	// then the second stdin line triggers a response followed by exit.
	// Wait a moment for the crash to propagate.
	time.Sleep(100 * time.Millisecond)

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

	// Start process manually (startProcess does an initial request which will time out)
	cmd := exec.Command(fakePath, "--daemon")
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	stderr, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start slow daemon: %v", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = makeBufferedReader(stdout)
	p.stderr = stderr
	p.processErr = make(chan error, 1)

	go func() {
		err := cmd.Wait()
		p.processErr <- err
	}()

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

func makeBufferedReader(r interface{ Read([]byte) (int, error) }) *bufio.Reader {
	return bufio.NewReaderSize(r.(interface {
		Read([]byte) (int, error)
	}), 256*1024)
}
