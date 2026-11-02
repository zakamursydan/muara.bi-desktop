//go:build cgo

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"muara.bi/engine/duckdb"
)

// The three file fixtures are the part of M2 that every platform can answer
// without a server, credentials or a connector bundle, so they are read here on
// whatever OS runs the tests. The engine already proves the mapping; what this
// adds is that the committed fixtures themselves parse, which is the claim the
// Windows question is actually built on.
func TestSpikeFileFixturesReadWithNoBundledConnector(t *testing.T) {
	bundle := t.TempDir()
	engine, err := duckdb.NewEngine(duckdb.GuardrailConfig{
		MaxRows:            20,
		ExtensionDirectory: bundle,
	})
	if err != nil {
		t.Fatalf("new engine: %v", err)
	}
	defer engine.Close()

	ctx := context.Background()
	session, err := engine.NewSession(ctx)
	if err != nil {
		t.Fatalf("new session: %v", err)
	}
	defer session.Close()

	for _, name := range []string{"spike.csv", "spike.json", "spike.parquet"} {
		t.Run(name, func(t *testing.T) {
			path, err := filepath.Abs(filepath.Join("tests", "fixtures", name))
			if err != nil {
				t.Fatalf("resolve fixture: %v", err)
			}
			if err := session.RegisterFileSource(ctx, "orders", path); err != nil {
				t.Fatalf("register %s: %v", name, err)
			}
			res, err := session.Query(ctx,
				`SELECT channel, COUNT(*) AS orders, SUM(total) AS revenue
				 FROM orders GROUP BY 1 ORDER BY revenue DESC`, duckdb.QueryOptions{})
			if err != nil {
				t.Fatalf("query %s: %v", name, err)
			}
			if res.RowCount != 3 || res.Truncated {
				t.Errorf("%s = %d rows, truncated=%v, want 3 whole rows", name, res.RowCount, res.Truncated)
			}
			if got := asText(res.Rows[0]["channel"]); got != "wholesale" {
				t.Errorf("%s top channel = %q, want wholesale", name, got)
			}

			// Parquet carries its schema, CSV and JSON have it inferred — all
			// three must still hand the grid typed columns.
			schema, err := session.Query(ctx, `DESCRIBE SELECT * FROM orders`, duckdb.QueryOptions{})
			if err != nil {
				t.Fatalf("describe %s: %v", name, err)
			}
			want := map[string]string{"id": "BIGINT", "channel": "VARCHAR", "total": "DOUBLE"}
			if len(schema.Rows) != len(want) {
				t.Fatalf("%s inferred %d columns, want %d", name, len(schema.Rows), len(want))
			}
			for _, row := range schema.Rows {
				column := asText(row["column_name"])
				if got, expected := asText(row["column_type"]), want[column]; got != expected {
					t.Errorf("%s column %s is %s, want %s", name, column, got, expected)
				}
			}
		})
	}

	// Nothing may arrive in the bundle folder: a reader that quietly downloads
	// would turn an offline promise into a network dependency.
	entries, err := os.ReadDir(bundle)
	if err != nil {
		t.Fatalf("read bundle dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("bundle dir gained %d entries while reading files", len(entries))
	}
}
