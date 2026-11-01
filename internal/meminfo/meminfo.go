// Package meminfo reads the amount of physical memory a machine has, so the
// desktop can size its DuckDB budget from the real host instead of a guess.
//
// It is deliberately free of CGO and of the engine: the three per-OS readers
// below are the part of M0 that CI can cross-compile and unit-test on every
// platform, while the engine stays the only place that knows how to apply a
// budget.
package meminfo

import "strconv"

// gib and mib are the binary units DuckDB itself is asked to use. Decimal units
// are not used anywhere here on purpose: "4GB" is 7% less than the label says,
// and the whole point of this package is that the number on screen means the
// bytes it spends.
const (
	mib uint64 = 1 << 20
	gib uint64 = 1 << 30
)

// maxBudget is BRD §15's ceiling: a desktop that allocates more than 4 GiB to
// one in-process database starts competing with the machine it lives on.
const maxBudget uint64 = 4 * gib

// minBudget keeps a degenerate reading (a 512 MB VM, or a mock in a test) from
// producing a budget that means nothing. Zero is what such a reading would round
// down to, and the engine rejects it outright — so this floor is the product's own
// promise, not a restatement of a DuckDB rule.
const minBudget uint64 = 256 * mib

// Total returns the physical memory of this machine in bytes.
func Total() (uint64, error) { return totalRAM() }

// Budget converts a byte count into the memory-limit string the engine sends to
// DuckDB: min(4GiB, RAM/4), rounded down, and never below minBudget.
//
// Rounding down is the safe direction — a budget is a ceiling, and DuckDB spills
// to temp_directory when it runs out rather than returning fewer rows. The value
// is reported in GiB when it is a whole number of GiB and in MiB otherwise, so
// the string stays inside the engine's NormalizeMaxMemory grammar and matches
// the units the settings UI labels its slider with.
func Budget(total uint64) string {
	b := total / 4
	if b > maxBudget {
		b = maxBudget
	}
	if b < minBudget {
		b = minBudget
	}
	if b%gib == 0 {
		return strconv.FormatUint(b/gib, 10) + "GiB"
	}
	return strconv.FormatUint(b/mib, 10) + "MiB"
}
