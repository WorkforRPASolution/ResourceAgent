//go:build !windows
// +build !windows

package service

// ReportStartupError is a no-op on non-Windows platforms.
func ReportStartupError(serviceName string, err error) {
	// No-op: Event Log is a Windows-only concept.
}
