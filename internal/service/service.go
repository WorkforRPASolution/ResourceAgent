// Package service provides platform-specific service integration.
package service

import "context"

// Service defines the interface for platform-specific service management.
type Service interface {
	// Run starts the service. It blocks until the service is stopped.
	Run(ctx context.Context) error

	// Stop requests the service to stop.
	Stop() error

	// IsService returns true if running as a system service.
	IsService() bool
}

// RunFunc is the main function that runs the agent logic.
type RunFunc func(ctx context.Context) error
