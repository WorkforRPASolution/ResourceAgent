//go:build windows
// +build windows

package service

import (
	"fmt"

	"golang.org/x/sys/windows/svc/eventlog"
)

// ReportStartupError writes a startup error to the Windows Event Log.
// This ensures "net start" and Event Viewer show the actual error message
// even when the logger has not been initialized yet.
func ReportStartupError(serviceName string, err error) {
	// Ensure the event source is registered (idempotent if already exists)
	_ = eventlog.InstallAsEventCreate(serviceName, eventlog.Error|eventlog.Warning|eventlog.Info)

	elog, openErr := eventlog.Open(serviceName)
	if openErr != nil {
		// Cannot open event log — nothing more we can do
		return
	}
	defer elog.Close()

	elog.Error(1, fmt.Sprintf("Failed to start: %v", err))
}
