//go:build windows

package collector

import (
	"syscall"
	"unsafe"
)

// Windows kernel HANDLE count via GetProcessHandleCount (kernel32.dll).
// Includes file/socket/registry/mutex/event/thread/process/etc handles for
// the current process. Per-process handle limit is ~16M (16,777,216) but a
// healthy long-lived agent should stay well under 1,000.
//
// Win XP+ supported (Win7 OK). Lazy-load to avoid linker dependency on
// machines without kernel32 (which doesn't happen in practice on Windows).
var (
	modkernel32                  = syscall.NewLazyDLL("kernel32.dll")
	procGetProcessHandleCount    = modkernel32.NewProc("GetProcessHandleCount")
	procGetCurrentProcess        = modkernel32.NewProc("GetCurrentProcess")
)

func (d *defaultRuntimeStats) ProcessHandleCount() (uint32, error) {
	hProc, _, _ := procGetCurrentProcess.Call()
	var count uint32
	r1, _, e1 := procGetProcessHandleCount.Call(
		hProc,
		uintptr(unsafe.Pointer(&count)),
	)
	if r1 == 0 {
		// e1 is always non-nil (syscall.Errno). Non-zero means real error.
		if e1 != nil && e1.(syscall.Errno) != 0 {
			return 0, e1
		}
		return 0, nil
	}
	return count, nil
}
