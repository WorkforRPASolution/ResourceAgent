//go:build !windows && !linux

package collector

// macOS / BSD stub. SelfMetrics handle_count metric will report 0 on these
// platforms — they are dev/test environments, not production targets.
func (d *defaultRuntimeStats) ProcessHandleCount() (uint32, error) {
	return 0, nil
}
