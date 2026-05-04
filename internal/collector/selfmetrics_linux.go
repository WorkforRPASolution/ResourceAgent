//go:build linux

package collector

import "os"

// Linux fd count via /proc/self/fd entry count. Cheap (one Readdirnames syscall).
// Per-process fd limit is typically 1024 (soft) / 4096+ (hard).
func (d *defaultRuntimeStats) ProcessHandleCount() (uint32, error) {
	dir, err := os.Open("/proc/self/fd")
	if err != nil {
		return 0, err
	}
	defer dir.Close()
	names, err := dir.Readdirnames(-1)
	if err != nil {
		return 0, err
	}
	// /proc/self/fd contains an extra entry for the open dir handle itself.
	if n := len(names); n > 0 {
		return uint32(n - 1), nil
	}
	return 0, nil
}
