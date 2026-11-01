//go:build linux

package meminfo

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// totalRAM reads MemTotal from /proc/meminfo. The value is in kB and is already
// the raw physical size, so multiplying by 1024 is exact rather than rounding.
//
// sysconf(_SC_PHYS_PAGES) is the other option and is not used: it needs CGO or a
// hand-rolled syscall, and /proc/meminfo is readable in a plain Go build, which
// is what lets CI compile this package for linux with CGO_ENABLED=0.
func totalRAM() (uint64, error) {
	raw, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, fmt.Errorf("read MemTotal: %w", err)
	}
	return parseMemTotal(string(raw))
}

func parseMemTotal(procFile string) (uint64, error) {
	for _, line := range strings.Split(procFile, "\n") {
		kb, ok := strings.CutPrefix(line, "MemTotal:")
		if !ok {
			continue
		}
		fields := strings.Fields(kb)
		if len(fields) == 0 {
			break
		}
		kilobytes, err := strconv.ParseUint(fields[0], 10, 64)
		if err != nil {
			return 0, fmt.Errorf("parse MemTotal %q: %w", fields[0], err)
		}
		return kilobytes * 1024, nil
	}
	return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
}
