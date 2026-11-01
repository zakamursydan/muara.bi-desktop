//go:build windows

package meminfo

import (
	"fmt"
	"syscall"
	"unsafe"
)

// memoryStatusEx mirrors MEMORYSTATUSEX. The two uint32 fields come first so the
// uint64 that follows lands at offset 8, exactly like the C layout; dwLength
// tells the API which version of the struct it is being handed.
type memoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

// totalRAM calls kernel32!GlobalMemoryStatusEx directly.
//
// golang.org/x/sys/windows does not export it, and the alternatives do not hold
// up: `wmic` is deprecated and removed in recent Windows 11 builds, and WMI is a
// service that can be unavailable on the very machines this product must install
// on. A LazyDLL proc needs no dependency, no CGO and no admin rights.
func totalRAM() (uint64, error) {
	proc := syscall.NewLazyDLL("kernel32.dll").NewProc("GlobalMemoryStatusEx")
	var status memoryStatusEx
	status.length = uint32(unsafe.Sizeof(status))
	ok, _, err := proc.Call(uintptr(unsafe.Pointer(&status)))
	if ok == 0 {
		return 0, fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}
	return status.totalPhys, nil
}
