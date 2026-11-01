//go:build darwin

package meminfo

import "golang.org/x/sys/unix"

// totalRAM reads hw.memsize, the value System Information shows as "Memory".
// It is a sysctl, not a file, so it costs one syscall and cannot be confused
// with memory that is merely available right now.
func totalRAM() (uint64, error) {
	return unix.SysctlUint64("hw.memsize")
}
