//go:build linux

package meminfo

import (
	"strings"
	"testing"
)

func TestParseMemTotal(t *testing.T) {
	got, err := parseMemTotal("MemTotal:       16384000 kB\nMemFree:          102400 kB\nMemAvailable:    8192000 kB\n")
	if err != nil {
		t.Fatalf("parseMemTotal: %v", err)
	}
	if want := uint64(16384000) * 1024; got != want {
		t.Fatalf("parseMemTotal = %d bytes, want %d (kB must be scaled by 1024)", got, want)
	}
}

func TestParseMemTotalFailsLoudly(t *testing.T) {
	for _, in := range []string{"", "MemFree: 1 kB\n", "MemTotal:\n", "MemTotal:   abc kB\n"} {
		if _, err := parseMemTotal(in); err == nil {
			t.Fatalf("parseMemTotal(%q) succeeded; a missing or unreadable MemTotal must not become a silent budget", in)
		} else if !strings.Contains(err.Error(), "MemTotal") {
			t.Fatalf("parseMemTotal(%q) error should name MemTotal, got %v", in, err)
		}
	}
}
