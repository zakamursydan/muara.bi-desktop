package meminfo

import (
	"testing"

	"muara.bi/engine/duckdb"
)

func TestBudgetIsQuarterOfRAMCappedAtFourGiB(t *testing.T) {
	cases := []struct {
		name  string
		total uint64
		want  string
	}{
		{"8 GiB laptop", 8 * gib, "2GiB"},
		{"16 GiB", 16 * gib, "4GiB"},
		{"64 GiB workstation stays capped", 64 * gib, "4GiB"},
		{"4 GiB", 4 * gib, "1GiB"},
		{"12 GiB", 12 * gib, "3GiB"},
		{"rounds down, never up", 8*gib + 1, "2GiB"},
		{"odd size lands on whole MiB", 6 * gib, "1536MiB"},
		{"3 GiB", 3 * gib, "768MiB"},
		{"tiny VM hits the floor", 512 * mib, "256MiB"},
		{"zero reading still yields a valid limit", 0, "256MiB"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Budget(tc.total)
			if got != tc.want {
				t.Fatalf("Budget(%d) = %q, want %q", tc.total, got, tc.want)
			}
			// The string is handed to the engine unchanged, so it has to survive
			// the engine's own grammar check. This is the cross-module contract.
			if _, err := duckdb.NormalizeMaxMemory(got); err != nil {
				t.Fatalf("engine rejects the budget this package produces: %v", err)
			}
		})
	}
}

func TestTotalReportsASanePhysicalSize(t *testing.T) {
	total, err := Total()
	if err != nil {
		t.Fatalf("Total() on the host we ship to: %v", err)
	}
	if total < gib {
		t.Fatalf("Total() = %d bytes; under 1 GiB is a misreading, not a real machine", total)
	}
	if total > 1<<50 {
		t.Fatalf("Total() = %d bytes; more than 1 TiB means a unit was misread", total)
	}
	t.Logf("host has %d bytes; budget %s", total, Budget(total))
}
