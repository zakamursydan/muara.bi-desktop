//go:build cgo

package main

import (
	"context"
	"errors"
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

// BRD §5.5 promises that a bundle keeps working when it is carried to another
// machine. The only part of a file source that cannot travel is its path, so
// this is the acceptance test for that sentence: save what the picker gives,
// move the whole folder, and the same stored string must read the same rows.
//
// It also locks the two halves of the rule that make it true — the bundle folder
// is the anchor, and DuckDB is never handed the relative form, because a
// relative literal is re-resolved against the working directory on every read.
func TestBundleRelativeSourceSurvivesTheFolderMoving(t *testing.T) {
	fixture, err := filepath.Abs(filepath.Join("tests", "fixtures", "spike.csv"))
	if err != nil {
		t.Fatalf("absolute fixture: %v", err)
	}

	open := func(anchor, stored string) int64 {
		t.Helper()
		path, err := duckdb.ResolveFileSourcePath(stored, anchor)
		if err != nil {
			t.Fatalf("resolve %q against %s: %v", stored, anchor, err)
		}
		engine, err := duckdb.NewEngine(duckdb.DefaultGuardrailConfig())
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

		if err := session.RegisterFileSource(ctx, "orders", path); err != nil {
			t.Fatalf("register %s: %v", path, err)
		}
		res, err := session.Query(ctx, "SELECT count(*) AS n FROM orders", duckdb.QueryOptions{})
		if err != nil {
			t.Fatalf("query: %v", err)
		}
		return res.RowCount
	}

	place := func(dir string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(dir, "data"), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "data", "orders.csv"), readFixture(t, fixture), 0o600); err != nil {
			t.Fatalf("copy fixture into %s: %v", dir, err)
		}
	}

	// A folder as it would be on the analyst's first machine.
	first := filepath.Join(t.TempDir(), "sales")
	place(first)

	stored, err := duckdb.CanonicalFileSourcePath(filepath.Join(first, "data", "orders.csv"), first)
	if err != nil {
		t.Fatalf("canonicalise: %v", err)
	}
	if stored != "data/orders.csv" {
		t.Fatalf("the bundle would store %q, want the bundle-relative data/orders.csv", stored)
	}

	want := open(first, stored)
	if want == 0 {
		t.Fatal("the source read zero rows on the first machine")
	}

	// The same definition, in a different folder under a different temp root.
	second := filepath.Join(t.TempDir(), "carry", "sales")
	place(second)
	if got := open(second, stored); got != want {
		t.Errorf("after moving the bundle: %d rows, want %d", got, want)
	}

	// A path that climbs out of the bundle folder is refused even when the file
	// it points at really exists — DuckDB would happily read it, which is exactly
	// why the rule lives in the resolver and not in the reader.
	parent := filepath.Dir(first)
	if err := os.WriteFile(filepath.Join(parent, "orders.csv"), readFixture(t, fixture), 0o600); err != nil {
		t.Fatalf("write the sibling file: %v", err)
	}
	if _, err := duckdb.ResolveFileSourcePath("../orders.csv", first); !errors.Is(err, duckdb.ErrFileSourceOutsideAnchor) {
		t.Errorf("an escaping path returned %v, want ErrFileSourceOutsideAnchor", err)
	}
	// The same file one folder up is reachable through a re-pointed absolute path
	// (BRD §5.5: "path tidak ada, bisa ditunjuk ulang") — it just stops being
	// portable, and the definition then says so.
	if got := open(parent, "orders.csv"); got != want {
		t.Errorf("re-pointed sibling: %d rows, want %d", got, want)
	}
	// And an absolute path survives canonicalisation unchanged: outside files are
	// stored as they are, not rewritten into ../ hops that read something else
	// on the next machine.
	absolute, err := duckdb.CanonicalFileSourcePath(filepath.Join(parent, "orders.csv"), first)
	if err != nil {
		t.Fatalf("canonicalise an outside file: %v", err)
	}
	if absolute != filepath.ToSlash(filepath.Join(parent, "orders.csv")) {
		t.Errorf("outside file stored as %q, want its own absolute path", absolute)
	}
}

func readFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}
