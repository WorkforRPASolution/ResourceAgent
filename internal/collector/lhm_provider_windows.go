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
	"strings"
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

	// Daemon process state.
	// stdinFile / stdoutFile are *os.File (anonymous pipes obtained via
	// os.Pipe) so we can call SetWriteDeadline / SetReadDeadline on them.
	// stdoutReader wraps stdoutFile for line-buffered reads.
	cmd          *exec.Cmd
	stdinFile    *os.File
	stdoutFile   *os.File
	stdoutReader *bufio.Reader
	stderr       io.ReadCloser

	// Process exit signaling — channel is CLOSED (not drained) on exit,
	// making it safe for multiple reads.
	processExit chan struct{}

	// Restart backoff
	consecutiveFailures int
	lastStartAttempt    time.Time
	requestTimeout      time.Duration

	// Timeout tracking. Each request timeout (LHM_TIMEOUT_KILL) triggers a
	// daemon kill so blocked stdin/stdout I/O unwinds — see option B note in
	// doRequestWithTimeout. consecutiveTimeouts is retained as an operator
	// signal: a sustained streak across multiple restarts implies LhmHelper
	// itself is unhealthy (bad sensor backend, .NET runtime issue, etc.) and
	// a kill alone won't help.
	consecutiveTimeouts int

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
		return fmt.Errorf("failed to start LhmHelper daemon: %w%s", err, diagnoseStartupFailure(err))
	}

	return nil
}

// diagnoseStartupFailure returns a user-friendly hint appended to startup errors.
// The most common LhmHelper startup failure on Windows 7 is a missing or
// outdated .NET Framework: the process exits before Go can write to stdin,
// surfacing as "pipe being closed" / "broken pipe" / "file already closed".
// LhmHelper targets .NET Framework 4.7 (net47), which is already installed on
// most factory Windows 7 PCs.
func diagnoseStartupFailure(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	isPipeError := strings.Contains(msg, "pipe is being closed") ||
		strings.Contains(msg, "broken pipe") ||
		strings.Contains(msg, "file already closed") ||
		strings.Contains(msg, "initial collection failed")
	if !isPipeError {
		return ""
	}
	return "\n\nHINT: LhmHelper.exe exited before responding. Most common causes:\n" +
		"  1. .NET Framework 4.7 (or later) not installed on this PC.\n" +
		"     Check: reg query \"HKLM\\SOFTWARE\\Microsoft\\NET Framework Setup\\NDP\\v4\\Full\" /v Release\n" +
		"     Required: 460798 or higher (.NET Framework 4.7).\n" +
		"     Most Windows 7 factory PCs already ship with 4.7. Contact administrator if upgrade is needed.\n" +
		"  2. Missing dependency DLLs next to LhmHelper.exe (LibreHardwareMonitorLib.dll, System.Text.Json.dll, etc).\n" +
		"  3. Antivirus quarantined LhmHelper.exe.\n" +
		"Run LhmHelper.exe manually from a command prompt to see the exact error."
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
//
// We use os.Pipe() (not cmd.StdinPipe / cmd.StdoutPipe) so the parent ends
// remain typed as *os.File. That lets doRequestWithTimeout call
// SetWriteDeadline / SetReadDeadline on them, which is the core mechanism
// keeping the request goroutine from leaking when LhmHelper hangs.
func (p *LhmProvider) startProcess() error {
	log := logger.WithComponent("lhm-provider")

	cmd := exec.Command(p.helperPath, "--daemon")

	// Anonymous pipes for stdin/stdout. childStdinR / childStdoutW are the
	// child's ends — we hand them to exec and close our copies after Start so
	// that EOF/SIGPIPE propagate cleanly when the child exits.
	childStdinR, parentStdinW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}
	parentStdoutR, childStdoutW, err := os.Pipe()
	if err != nil {
		childStdinR.Close()
		parentStdinW.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	cmd.Stdin = childStdinR
	cmd.Stdout = childStdoutW

	stderr, err := cmd.StderrPipe()
	if err != nil {
		childStdinR.Close()
		parentStdinW.Close()
		parentStdoutR.Close()
		childStdoutW.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		childStdinR.Close()
		parentStdinW.Close()
		parentStdoutR.Close()
		childStdoutW.Close()
		return fmt.Errorf("failed to start LhmHelper: %w", err)
	}

	// After Start, the child has duplicated handles; close the parent copies
	// of the child ends so EOF flows through when the child exits.
	childStdinR.Close()
	childStdoutW.Close()

	p.cmd = cmd
	p.stdinFile = parentStdinW
	p.stdoutFile = parentStdoutR
	p.stdoutReader = bufio.NewReaderSize(parentStdoutR, 256*1024) // 256KB buffer for large JSON
	p.stderr = stderr
	p.processExit = make(chan struct{})
	p.consecutiveTimeouts = 0

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

	// Close stdin to signal graceful exit. Clear any pending write deadline
	// first so Close itself isn't unwound through a deadline error path.
	if p.stdinFile != nil {
		p.stdinFile.SetWriteDeadline(time.Time{})
		p.stdinFile.Close()
		p.stdinFile = nil
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

	// Close the stdout side last; cmd.Wait already drained it but releasing
	// the parent fd avoids handle leaks across restarts.
	if p.stdoutFile != nil {
		p.stdoutFile.SetReadDeadline(time.Time{})
		p.stdoutFile.Close()
		p.stdoutFile = nil
	}

	p.cmd = nil
	p.stdoutReader = nil
}

// drainStderr reads LhmHelper stderr and forwards to Go logger.
// defer recover 로 reader/logger panic 을 잡아 LhmProvider 전체 프로세스가 죽지 않도록 한다.
// scanner.Err() 가 비-nil 인 경우 (e.g. bufio.ErrTooLong) 로깅으로 디버깅 신호를 남긴다.
func (p *LhmProvider) drainStderr(r io.Reader) {
	log := logger.WithComponent("lhm-helper")
	defer func() {
		if rec := recover(); rec != nil {
			log.Error().Interface("panic", rec).Msg("drainStderr panic recovered")
		}
	}()

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		log.Info().Str("stderr", scanner.Text()).Msg("LhmHelper")
	}
	if err := scanner.Err(); err != nil {
		log.Warn().Err(err).Msg("drainStderr scanner error")
	}
}

// lhmRequestResult is the message a worker goroutine in doRequestWithTimeout
// posts back to its caller. Defined at package scope so killAndDrain can
// share the channel type.
type lhmRequestResult struct {
	data *LhmData
	err  error
}

// doRequest issues a single collect with the configured request timeout.
// Kept as a thin wrapper for callers (notably startProcess) that don't have a
// caller-supplied context. Must be called while p.mu is held (or during
// startup before any concurrent access).
func (p *LhmProvider) doRequest() (*LhmData, error) {
	return p.doRequestWithTimeout(p.ctx)
}

// doRequestWithTimeout sends "collect\n" and reads one JSON response line.
//
// Strategy: option B (Kill-on-timeout) only.
//
// Background: An earlier hybrid design tried option A (pipe deadlines) as a
// first defence, but Win10 / Win11 testing surfaced the Go runtime error
// "file type does not support deadline" — anonymous pipes from os.Pipe() do
// not honour SetReadDeadline / SetWriteDeadline on Windows. The hybrid plan
// was abandoned in favour of option B alone, see plan v2.4.2 note.
//
// Mechanism:
//  1. Spawn a worker goroutine that performs the blocking stdin.Write +
//     stdout.ReadBytes. Any result (success, parse error, pipe error) is
//     reported on the result channel.
//  2. Race the channel against p.requestTimeout.
//  3. On timeout, call cmd.Process.Kill() so the blocked I/O syscalls return
//     errors and the worker goroutine unwinds. Synchronously drain the
//     channel with a hard upper bound so the worker is observably done
//     before we return — this guarantees no goroutine leak.
//
// All abnormal paths emit a structured log line with a unique LHM_* prefix
// (see docs/runbooks/lhm-provider-timeout-monitoring.md) so deployments
// can be diagnosed from logs without a debugger.
func (p *LhmProvider) doRequestWithTimeout(ctx context.Context) (*LhmData, error) {
	log := logger.WithComponent("lhm-provider")

	// Capture pipe references while p.mu is held. Prevents data races if
	// stopProcess runs after a timeout and clears the fields.
	stdinFile := p.stdinFile
	stdoutReader := p.stdoutReader
	if stdinFile == nil || stdoutReader == nil {
		return nil, fmt.Errorf("LhmHelper pipes not available")
	}

	ch := make(chan lhmRequestResult, 1)
	go func() {
		if _, err := stdinFile.Write([]byte("collect\n")); err != nil {
			ch <- lhmRequestResult{nil, fmt.Errorf("stdin write: %w", err)}
			return
		}
		line, err := stdoutReader.ReadBytes('\n')
		if err != nil {
			ch <- lhmRequestResult{nil, fmt.Errorf("stdout read: %w", err)}
			return
		}
		var data LhmData
		if err := json.Unmarshal(line, &data); err != nil {
			ch <- lhmRequestResult{nil, fmt.Errorf("parse response (%d bytes): %w", len(line), err)}
			return
		}
		ch <- lhmRequestResult{&data, nil}
	}()

	select {
	case r := <-ch:
		if r.err != nil {
			// Non-timeout I/O error (broken pipe, EOF, parse failure). The
			// worker already returned, so no goroutine leak. GetData/restart
			// logic will recreate the daemon if the pipe is broken.
			log.Warn().Err(r.err).Msg("LHM_IO_ERROR pipe I/O or parse failed")
			return nil, fmt.Errorf("LhmHelper request failed: %w", r.err)
		}
		// Success: clear any prior timeout streak.
		if p.consecutiveTimeouts > 0 {
			log.Info().
				Int("prior_timeouts", p.consecutiveTimeouts).
				Msg("LHM_TIMEOUT_RECOVERED daemon responded after prior timeout streak")
			p.consecutiveTimeouts = 0
		}
		return r.data, nil

	case <-ctx.Done():
		// Caller cancelled. Kill the daemon so the blocked worker unwinds.
		log.Warn().Err(ctx.Err()).Msg("LHM_CTX_CANCELLED killing LhmHelper to release blocked I/O")
		p.killAndDrain(ch, "ctx_cancelled")
		return nil, fmt.Errorf("LhmHelper request cancelled: %w", ctx.Err())

	case <-time.After(p.requestTimeout):
		p.consecutiveTimeouts++
		log.Error().
			Dur("timeout", p.requestTimeout).
			Int("consecutive_timeouts", p.consecutiveTimeouts).
			Msg("LHM_TIMEOUT_KILL request timed out; killing LhmHelper to release blocked goroutine")
		p.killAndDrain(ch, "timeout")
		return nil, fmt.Errorf("LhmHelper request timed out after %v", p.requestTimeout)
	}
}

// killAndDrain kills the LhmHelper daemon (so the blocked worker's pipe I/O
// returns an error) and waits for the worker to actually finish. The drain
// step is what makes the goroutine accounting tight: by the time this
// function returns, the worker has provably exited.
//
// A hard upper bound on the drain protects against the (theoretical) case
// where Process.Kill itself fails or the OS does not unblock the I/O — in
// which case we surface LHM_DRAIN_TIMEOUT so the operator can see that one
// goroutine is unaccountable until the next process restart.
func (p *LhmProvider) killAndDrain(ch <-chan lhmRequestResult, reason string) {
	log := logger.WithComponent("lhm-provider")

	if p.cmd != nil && p.cmd.Process != nil {
		if err := p.cmd.Process.Kill(); err != nil {
			log.Error().
				Err(err).
				Str("reason", reason).
				Msg("LHM_KILL_FAILED Process.Kill failed")
		}
	}

	select {
	case <-ch:
		// Worker observed the killed pipe and exited — no leak.
	case <-time.After(2 * time.Second):
		log.Error().
			Str("reason", reason).
			Msg("LHM_DRAIN_TIMEOUT worker goroutine did not unwind within 2s; one goroutine leaked until next process restart")
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
