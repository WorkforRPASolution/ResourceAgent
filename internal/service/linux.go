//go:build !windows
// +build !windows

package service

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"resourceagent/internal/logger"
)

// LinuxService implements the service interface for Linux/Unix systems.
type LinuxService struct {
	runFunc RunFunc
	cancel  context.CancelFunc
	mu      sync.Mutex
	stopped bool
}

// NewService creates a new platform-specific service.
func NewService(runFunc RunFunc) Service {
	return &LinuxService{
		runFunc: runFunc,
	}
}

// Run starts the service and handles signals for graceful shutdown.
func (s *LinuxService) Run(ctx context.Context) error {
	log := logger.WithComponent("linux-service")

	ctx, s.cancel = context.WithCancel(ctx)

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the main run function in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.runFunc(ctx)
	}()

	log.Info().Msg("Service started")

	select {
	case sig := <-sigChan:
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		s.Stop()

		// Wait for runFunc to complete
		select {
		case err := <-done:
			return err
		case sig := <-sigChan:
			log.Warn().Str("signal", sig.String()).Msg("Received second signal, forcing exit")
			return nil
		}

	case err := <-done:
		return err
	}
}

// Stop requests the service to stop.
func (s *LinuxService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil && !s.stopped {
		s.stopped = true
		s.cancel()
	}
	return nil
}

// IsService returns true if running as a system service (always false on Linux as detection is not straightforward).
func (s *LinuxService) IsService() bool {
	// On Linux, we can check if stdin is a terminal to determine if running interactively
	// For systemd services, stdin is typically not a terminal
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) == 0
}
