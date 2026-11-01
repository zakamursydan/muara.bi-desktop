//go:build cgo

package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"muara.bi/desktop/internal/meminfo"
	"muara.bi/engine/duckdb"
)

const (
	mib uint64 = 1 << 20
	gib uint64 = 1 << 30
)

// TestRAMBudgetReachesDuckDB closes the chain the memory profile depends on:
// derive min(4GiB, RAM/4), hand the string to the engine, and check the bytes
// DuckDB actually stored. Bytes, because "2.0 GiB" and "2.0 GB" look the same on
// screen and differ by 7% in reality.
//
// The RAM sizes are synthetic on purpose: asserting against this machine's real
// total would make CI fail on a runner with an awkward size, where DuckDB's
// one-decimal echo cannot round-trip exactly.
func TestRAMBudgetReachesDuckDB(t *testing.T) {
	cases := []struct {
		ramBytes  uint64
		wantLimit uint64
	}{
		{8 * gib, 2 * gib},
		{6 * gib, 1536 * mib},
		{1 * gib, 256 * mib},
	}

	for _, tc := range cases {
		budget := meminfo.Budget(tc.ramBytes)
		engine, err := duckdb.NewEngine(duckdb.GuardrailConfig{
			MaxRows:          duckdb.DefaultMaxRows,
			StatementTimeout: 10 * time.Second,
			MaxMemory:        budget,
		})
		if err != nil {
			t.Fatalf("engine with budget %q: %v", budget, err)
		}

		ctx := context.Background()
		session, err := engine.NewSession(ctx)
		if err != nil {
			engine.Close()
			t.Fatalf("new session: %v", err)
		}

		result, err := session.Query(ctx, "SELECT current_setting('max_memory') AS budget", duckdb.QueryOptions{})
		if err != nil {
			session.Close()
			engine.Close()
			t.Fatalf("read setting for budget %q: %v", budget, err)
		}
		reported, parseErr := duckBytes(asText(result.Rows[0]["budget"]))
		_ = session.Close()
		_ = engine.Close()
		if parseErr != nil {
			t.Fatalf("budget %q: %v", budget, parseErr)
		}
		if reported != tc.wantLimit {
			t.Fatalf("RAM %d -> %q landed as %d bytes, want %d", tc.ramBytes, budget, reported, tc.wantLimit)
		}
	}

	if total, err := meminfo.Total(); err != nil {
		t.Errorf("meminfo.Total on the host we ship to: %v", err)
	} else {
		t.Logf("this machine: RAM %d bytes -> budget %s", total, meminfo.Budget(total))
	}
}

// duckBytes parses a size DuckDB echoes back, which is always a binary unit.
func duckBytes(s string) (uint64, error) {
	fields := strings.Fields(s)
	if len(fields) != 2 {
		return 0, fmt.Errorf("unexpected duckdb size %q", s)
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("unexpected duckdb size %q: %w", s, err)
	}
	multipliers := map[string]uint64{"KiB": 1 << 10, "MiB": mib, "GiB": gib, "TiB": 1 << 40}
	m, ok := multipliers[fields[1]]
	if !ok {
		return 0, fmt.Errorf("unknown duckdb size unit %q", fields[1])
	}
	return uint64(value * float64(m)), nil
}
