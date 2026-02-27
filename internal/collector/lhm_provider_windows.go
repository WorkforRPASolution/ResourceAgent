//go:build windows

package collector

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"resourceagent/internal/logger"
)

// LhmData represents the complete JSON output from LhmHelper.exe.
// All LHM-based collectors share this cached data.
type LhmData struct {
	Sensors          []LhmSensor          `json:"Sensors"`
	Fans             []LhmFan             `json:"Fans"`
	Gpus             []LhmGpu             `json:"Gpus"`
	Storages         []LhmStorage         `json:"Storages"`
	Voltages         []LhmVoltage         `json:"Voltages"`
	MotherboardTemps []LhmMotherboardTemp `json:"MotherboardTemps"`
	Error            string               `json:"error,omitempty"`
}

// LhmSensor represents CPU temperature sensor data.
type LhmSensor struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
	High        float64 `json:"High"`
	Critical    float64 `json:"Critical"`
}

// LhmFan represents fan sensor data.
type LhmFan struct {
	Name string  `json:"Name"`
	RPM  float64 `json:"RPM"`
}

// LhmGpu represents GPU sensor data.
type LhmGpu struct {
	Name        string   `json:"Name"`
	Temperature *float64 `json:"Temperature"`
	CoreLoad    *float64 `json:"CoreLoad"`
	MemoryLoad  *float64 `json:"MemoryLoad"`
	FanSpeed    *float64 `json:"FanSpeed"`
	Power       *float64 `json:"Power"`
	CoreClock   *float64 `json:"CoreClock"`
	MemoryClock *float64 `json:"MemoryClock"`
}

// LhmStorage represents S.M.A.R.T storage data.
type LhmStorage struct {
	Name              string   `json:"Name"`
	Type              string   `json:"Type"`
	Temperature       *float64 `json:"Temperature"`
	RemainingLife     *float64 `json:"RemainingLife"`
	MediaErrors       *int64   `json:"MediaErrors"`
	PowerCycles       *int64   `json:"PowerCycles"`
	UnsafeShutdowns   *int64   `json:"UnsafeShutdowns"`
	PowerOnHours      *int64   `json:"PowerOnHours"`
	TotalBytesWritten *int64   `json:"TotalBytesWritten"`
}

// LhmVoltage represents voltage sensor data.
type LhmVoltage struct {
	Name    string  `json:"Name"`
	Voltage float64 `json:"Voltage"`
}

// LhmMotherboardTemp represents motherboard temperature sensor data.
type LhmMotherboardTemp struct {
	Name        string  `json:"Name"`
	Temperature float64 `json:"Temperature"`
}

// LhmProvider provides cached access to LhmHelper.exe output.
// Thread-safe singleton that all LHM-based collectors share.
// Manages a long-running LhmHelper daemon process via stdin/stdout pipes.
type LhmProvider struct {
	mu          sync.Mutex
	data        *LhmData
	lastUpdate  time.Time
	cacheTTL    time.Duration
	helperPath  string
	helperFound bool

	// Daemon process state
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	// Process exit signaling — channel is CLOSED (not drained) on exit,
	// making it safe for multiple reads.
	processExit chan struct{}

	// Restart backoff
	consecutiveFailures int
	lastStartAttempt    time.Time
	requestTimeout      time.Duration

	// Lifecycle
	ctx     context.Context
	cancel  context.CancelFunc
	started bool
}

var (
	lhmProviderInstance *LhmProvider
	lhmProviderOnce     sync.Once
)

// GetLhmProvider returns the singleton LhmProvider instance.
func GetLhmProvider() *LhmProvider {
	lhmProviderOnce.Do(func() {
		lhmProviderInstance = &LhmProvider{
			cacheTTL:       5 * time.Second,
			requestTimeout: 10 * time.Second,
		}
	})
	return lhmProviderInstance
}

// SetCacheTTL sets the cache time-to-live duration.
func (p *LhmProvider) SetCacheTTL(ttl time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cacheTTL = ttl
}

// Start initializes and starts the LhmHelper daemon process.
// Must be called before GetData. Non-fatal: callers should log and continue
// if this fails (collectors will return empty data).
func (p *LhmProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.started {
		return nil
	}

	// Find helper if not already found
	if !p.helperFound {
		path, err := p.findLhmHelper()
		if err != nil {
			return err
		}
		p.helperPath = path
		p.helperFound = true
	}

	p.ctx, p.cancel = context.WithCancel(ctx)
	p.started = true

	if err := p.startProcess(); err != nil {
		p.started = false
		p.cancel()
		return fmt.Errorf("failed to start LhmHelper daemon: %w", err)
	}

	return nil
}

// Stop shuts down the LhmHelper daemon process gracefully.
func (p *LhmProvider) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return
	}

	p.started = false
	p.stopProcess()
	p.cancel()
}

// GetData returns cached LhmHelper data, refreshing if stale.
func (p *LhmProvider) GetData(ctx context.Context) (*LhmData, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return &LhmData{}, nil
	}

	// Cache hit
	if p.data != nil && time.Since(p.lastUpdate) < p.cacheTTL {
		return p.data, nil
	}

	log := logger.WithComponent("lhm-provider")

	// Check if process is alive; restart if dead
	if !p.isProcessAlive() {
		log.Warn().Msg("LhmHelper process is dead, attempting restart")
		if err := p.restartWithBackoff(); err != nil {
			return nil, err
		}
	}

	// Request data from daemon
	data, err := p.doRequestWithTimeout(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("LhmHelper request failed, stopping process for restart on next call")
		p.consecutiveFailures++
		p.stopProcess()
		return nil, fmt.Errorf("LhmHelper request failed: %w", err)
	}

	if data.Error != "" {
		log.Warn().Str("error", data.Error).Msg("LhmHelper returned error in data")
		return nil, fmt.Errorf("LhmHelper error: %s", data.Error)
	}

	// Success: update cache and reset backoff
	p.data = data
	p.lastUpdate = time.Now()
	p.consecutiveFailures = 0

	log.Debug().
		Int("sensors", len(data.Sensors)).
		Int("fans", len(data.Fans)).
		Int("gpus", len(data.Gpus)).
		Int("storages", len(data.Storages)).
		Msg("LhmHelper data refreshed")

	return data, nil
}

// Invalidate clears the cache, forcing a refresh on next GetData call.
func (p *LhmProvider) Invalidate() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = nil
	p.lastUpdate = time.Time{}
}

// startProcess launches LhmHelper.exe in daemon mode and validates with an initial request.
func (p *LhmProvider) startProcess() error {
	log := logger.WithComponent("lhm-provider")

	cmd := exec.Command(p.helperPath, "--daemon")

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		stdin.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		stdin.Close()
		stdout.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start LhmHelper: %w", err)
	}

	p.cmd = cmd
	p.stdin = stdin
	p.stdout = bufio.NewReaderSize(stdout, 256*1024) // 256KB buffer for large JSON
	p.stderr = stderr
	p.processExit = make(chan struct{})

	// Monitor process exit in background.
	// Capture exitCh by value so a restart creating a new channel won't
	// cause this goroutine to close the wrong (new) channel.
	exitCh := p.processExit
	go func() {
		cmd.Wait()
		close(exitCh)
	}()

	// Forward stderr to Go logger in background
	go p.drainStderr(stderr)

	log.Info().
		Int("pid", cmd.Process.Pid).
		Str("path", p.helperPath).
		Msg("LhmHelper daemon started")

	// Validate with an initial request
	initialData, err := p.doRequest()
	if err != nil {
		log.Error().Err(err).Msg("LhmHelper initial collection failed")
		p.stopProcess()
		return fmt.Errorf("LhmHelper initial collection failed: %w", err)
	}

	if initialData.Error != "" {
		log.Error().Str("error", initialData.Error).Msg("LhmHelper initialization error")
		p.stopProcess()
		return fmt.Errorf("LhmHelper initialization error: %s", initialData.Error)
	}

	// Cache the initial data
	p.data = initialData
	p.lastUpdate = time.Now()
	p.consecutiveFailures = 0

	log.Info().
		Int("sensors", len(initialData.Sensors)).
		Int("fans", len(initialData.Fans)).
		Int("gpus", len(initialData.Gpus)).
		Int("storages", len(initialData.Storages)).
		Int("voltages", len(initialData.Voltages)).
		Int("mb_temps", len(initialData.MotherboardTemps)).
		Msg("LhmHelper daemon ready")

	return nil
}

// stopProcess closes stdin to signal C# to exit, then waits or kills.
func (p *LhmProvider) stopProcess() {
	log := logger.WithComponent("lhm-provider")

	if p.cmd == nil || p.cmd.Process == nil {
		return
	}

	pid := p.cmd.Process.Pid

	// Close stdin to signal graceful exit
	if p.stdin != nil {
		p.stdin.Close()
		p.stdin = nil
	}

	// Close stderr to unblock drainStderr goroutine before waiting on process exit.
	// cmd.Wait() closes pipe read ends, but closing early is safe and prevents
	// potential deadlock if drainStderr is blocked.
	if p.stderr != nil {
		p.stderr.Close()
		p.stderr = nil
	}

	// Wait for process to exit with timeout.
	// processExit is a closed channel after process exits, safe for multiple reads.
	select {
	case <-p.processExit:
		log.Info().Int("pid", pid).Msg("LhmHelper daemon stopped")
	case <-time.After(5 * time.Second):
		log.Warn().Int("pid", pid).Msg("LhmHelper daemon did not exit in time, killing")
		p.cmd.Process.Kill()
		<-p.processExit
	}

	p.cmd = nil
	p.stdout = nil
}

// drainStderr reads LhmHelper stderr and forwards to Go logger.
func (p *LhmProvider) drainStderr(r io.Reader) {
	log := logger.WithComponent("lhm-helper")
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Info().Str("stderr", scanner.Text()).Msg("LhmHelper")
	}
}

// doRequest writes a request to stdin and reads a JSON response line from stdout.
// Must be called while p.mu is held (or during startup before any concurrent access).
func (p *LhmProvider) doRequest() (*LhmData, error) {
	if p.stdin == nil {
		return nil, fmt.Errorf("LhmHelper stdin is closed")
	}
	if p.stdout == nil {
		return nil, fmt.Errorf("LhmHelper stdout is closed")
	}

	// Send request
	if _, err := p.stdin.Write([]byte("collect\n")); err != nil {
		return nil, fmt.Errorf("failed to write to LhmHelper stdin: %w", err)
	}

	// Read response line
	line, err := p.stdout.ReadBytes('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read from LhmHelper stdout: %w", err)
	}

	var data LhmData
	if err := json.Unmarshal(line, &data); err != nil {
		return nil, fmt.Errorf("failed to parse LhmHelper response (%d bytes): %w", len(line), err)
	}

	return &data, nil
}

// doRequestWithTimeout wraps pipe I/O with a deadline.
// Captures pipe references to avoid data races if stopProcess runs after timeout.
func (p *LhmProvider) doRequestWithTimeout(ctx context.Context) (*LhmData, error) {
	// Capture pipe references while lock is held (called from GetData).
	// This prevents data race if timeout fires and stopProcess sets p.stdin/p.stdout = nil.
	stdin := p.stdin
	stdout := p.stdout
	if stdin == nil || stdout == nil {
		return nil, fmt.Errorf("LhmHelper pipes not available")
	}

	type result struct {
		data *LhmData
		err  error
	}

	ch := make(chan result, 1)
	go func() {
		// Use captured references, not p.stdin/p.stdout
		if _, err := stdin.Write([]byte("collect\n")); err != nil {
			ch <- result{nil, fmt.Errorf("failed to write to LhmHelper stdin: %w", err)}
			return
		}

		line, err := stdout.ReadBytes('\n')
		if err != nil {
			ch <- result{nil, fmt.Errorf("failed to read from LhmHelper stdout: %w", err)}
			return
		}

		var data LhmData
		if err := json.Unmarshal(line, &data); err != nil {
			ch <- result{nil, fmt.Errorf("failed to parse LhmHelper response (%d bytes): %w", len(line), err)}
			return
		}

		ch <- result{&data, nil}
	}()

	select {
	case r := <-ch:
		return r.data, r.err
	case <-ctx.Done():
		return nil, fmt.Errorf("LhmHelper request cancelled: %w", ctx.Err())
	case <-time.After(p.requestTimeout):
		return nil, fmt.Errorf("LhmHelper request timed out (%v)", p.requestTimeout)
	}
}

// isProcessAlive checks if the daemon process is still running.
// Uses a closed channel (processExit) so multiple calls return the same result.
func (p *LhmProvider) isProcessAlive() bool {
	if p.cmd == nil || p.cmd.Process == nil {
		return false
	}

	select {
	case <-p.processExit:
		return false
	default:
		return true
	}
}

// restartWithBackoff restarts the daemon with exponential backoff.
// Backoff: 1s, 2s, 4s, 8s, 16s, 32s, 60s (max). Resets on success.
func (p *LhmProvider) restartWithBackoff() error {
	log := logger.WithComponent("lhm-provider")

	// Calculate backoff delay
	backoffSeconds := 1 << p.consecutiveFailures
	if backoffSeconds > 60 {
		backoffSeconds = 60
	}
	backoff := time.Duration(backoffSeconds) * time.Second

	// Wait if not enough time has passed.
	// Release mutex during sleep to allow Stop() to proceed.
	elapsed := time.Since(p.lastStartAttempt)
	if elapsed < backoff {
		wait := backoff - elapsed
		log.Warn().
			Int("consecutive_failures", p.consecutiveFailures).
			Dur("backoff_wait", wait).
			Msg("LhmHelper restart backoff")

		p.mu.Unlock()
		select {
		case <-time.After(wait):
		case <-p.ctx.Done():
			p.mu.Lock()
			return fmt.Errorf("LhmHelper restart cancelled: %w", p.ctx.Err())
		}
		p.mu.Lock()

		// Re-check state after re-acquiring lock (Stop may have been called)
		if !p.started {
			return fmt.Errorf("LhmHelper stopped during restart backoff")
		}
	}

	p.lastStartAttempt = time.Now()

	log.Info().
		Int("consecutive_failures", p.consecutiveFailures).
		Msg("Restarting LhmHelper daemon")

	// Clean up old process
	p.stopProcess()

	// Start new process
	if err := p.startProcess(); err != nil {
		p.consecutiveFailures++
		return fmt.Errorf("LhmHelper restart failed (attempt %d): %w", p.consecutiveFailures, err)
	}

	return nil
}

// findLhmHelper searches for LhmHelper.exe in common locations.
func (p *LhmProvider) findLhmHelper() (string, error) {
	candidates := []string{
		"LhmHelper.exe",
		"./LhmHelper.exe",
		filepath.Join(".", "utils", "LhmHelper.exe"),
		filepath.Join(".", "utils", "lhm-helper", "LhmHelper.exe"),
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "LhmHelper.exe"),
			filepath.Join(exeDir, "utils", "LhmHelper.exe"),
			filepath.Join(exeDir, "utils", "lhm-helper", "LhmHelper.exe"),
			filepath.Join(exeDir, "..", "..", "utils", "lhm-helper", "LhmHelper.exe"),
		)
	}

	candidates = append(candidates,
		`C:\Program Files\ResourceAgent\LhmHelper.exe`,
		`C:\Program Files\ResourceAgent\utils\LhmHelper.exe`,
		`C:\Program Files\ResourceAgent\utils\lhm-helper\LhmHelper.exe`,
	)

	for _, path := range candidates {
		if fullPath, err := exec.LookPath(path); err == nil {
			return fullPath, nil
		}
	}

	return "", fmt.Errorf("LhmHelper.exe not found in any expected location")
}
