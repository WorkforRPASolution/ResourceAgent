//go:build windows
// +build windows

package service

import (
	"context"
	"sync"
	"time"

	"golang.org/x/sys/windows/svc"

	"resourceagent/internal/logger"
)

const serviceName = "ResourceAgent"

// WindowsService implements the Windows service interface.
type WindowsService struct {
	runFunc RunFunc
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	stopped bool
}

// NewService creates a new platform-specific service.
func NewService(runFunc RunFunc) Service {
	return &WindowsService{
		runFunc: runFunc,
	}
}

// Run starts the service.
func (s *WindowsService) Run(ctx context.Context) error {
	if !s.IsService() {
		// Running interactively
		return s.runFunc(ctx)
	}

	// Running as Windows service
	return svc.Run(serviceName, s)
}

// Stop requests the service to stop.
func (s *WindowsService) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil && !s.stopped {
		s.stopped = true
		s.cancel()
	}
	return nil
}

// IsService returns true if running as a Windows service.
func (s *WindowsService) IsService() bool {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return false
	}
	return isService
}

// Execute implements the svc.Handler interface.
func (s *WindowsService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	log := logger.WithComponent("windows-service")

	const acceptedCommands = svc.AcceptStop | svc.AcceptShutdown

	changes <- svc.Status{State: svc.StartPending}

	s.ctx, s.cancel = context.WithCancel(context.Background())

	// Start the main run function in a goroutine
	done := make(chan error, 1)
	go func() {
		done <- s.runFunc(s.ctx)
	}()

	changes <- svc.Status{State: svc.Running, Accepts: acceptedCommands}
	log.Info().Msg("Windows service started")

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
				// Respond twice as per documentation
				time.Sleep(100 * time.Millisecond)
				changes <- c.CurrentStatus

			case svc.Stop, svc.Shutdown:
				log.Info().Msg("Received stop signal from Windows service control")
				changes <- svc.Status{State: svc.StopPending}
				s.Stop()

				// Wait for runFunc to complete
				select {
				case <-done:
				case <-time.After(30 * time.Second):
					log.Warn().Msg("Timeout waiting for service to stop")
				}

				changes <- svc.Status{State: svc.Stopped}
				return false, 0

			default:
				log.Warn().Int("cmd", int(c.Cmd)).Msg("Unexpected service control command")
			}

		case err := <-done:
			if err != nil {
				log.Error().Err(err).Msg("Service run function exited with error")
				changes <- svc.Status{State: svc.Stopped}
				return true, 1
			}
			changes <- svc.Status{State: svc.Stopped}
			return false, 0
		}
	}
}
