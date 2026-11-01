// Command spike is the M0 platform probe, not a product entry point: it answers
// whether this repository can consume the shared engine at all — compiling the
// CGO DuckDB path from a second Go module, applying the configured memory
// budget, and reading a real Postgres through ATTACH.
//
// M0 replaces it with the Wails binding that calls the same engine calls.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"muara.bi/engine/duckdb"
	"muara.bi/engine/sqlident"
)

func main() {
	dsn := flag.String("pg-dsn", os.Getenv("MUARA_SPIKE_PG_DSN"),
		"Postgres conninfo to ATTACH; empty skips the federated step")
	alias := flag.String("alias", "sample", "DuckDB catalogue name for the attached source")
	table := flag.String("table", "", "fixture table to count rows in, after the source is attached")
	extDir := flag.String("extension-directory", os.Getenv("DUCKDB_EXTENSION_DIRECTORY"),
		"directory holding pre-bundled .duckdb_extension files; empty uses DuckDB's default")
	timeout := flag.Duration("timeout", 15*time.Second, "per-statement deadline")
	flag.Parse()

	// Deliberately leaves MaxMemory empty: the point is to prove the shipped
	// default reaches DuckDB from a module that is not the web one.
	engine, err := duckdb.NewEngine(duckdb.GuardrailConfig{
		MaxRows:            20,
		StatementTimeout:   *timeout,
		ExtensionDirectory: *extDir,
	})
	if err != nil {
		fail("new engine", err)
	}
	defer engine.Close()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout*4)
	defer cancel()

	session, err := engine.NewSession(ctx)
	if err != nil {
		fail("new session", err)
	}
	defer session.Close()

	report(ctx, session, "SELECT 1 AS one, current_setting('max_memory') AS budget, current_setting('threads') AS threads")

	if *dsn == "" {
		fmt.Println("SKIP federated step: no -pg-dsn given (engine-only probe)")
		return
	}
	// The alias becomes an identifier in the statement below, so it is checked
	// with the engine's own rule instead of being interpolated blindly.
	if !sqlident.IsPlain(*alias) {
		fail("validate alias", fmt.Errorf("%q is not a plain identifier", *alias))
	}
	if err := session.AttachPostgres(ctx, *alias, *dsn); err != nil {
		fail("attach postgres", err)
	}
	// Counts metadata rather than naming tables, so no identifier from the
	// remote catalogue has to be interpolated into this statement.
	report(ctx, session, fmt.Sprintf(
		"SELECT count(*) AS tables FROM %s.information_schema.tables WHERE table_schema = 'public'", *alias))

	// -table reads only a row count from a named fixture table, which is enough
	// to prove data flows through the scanner without pulling real rows.
	if *table != "" {
		if !sqlident.IsPlain(*table) {
			fail("validate table", fmt.Errorf("%q is not a plain identifier", *table))
		}
		report(ctx, session, fmt.Sprintf("SELECT count(*) AS rows_in_%s FROM %s.%s",
			*table, *alias, *table))
	}
}

func report(ctx context.Context, session duckdb.Session, query string) {
	result, err := session.Query(ctx, query, duckdb.QueryOptions{})
	if err != nil {
		fail("query", err)
	}
	for _, row := range result.Rows {
		fmt.Println("row:", row)
	}
	fmt.Printf("ok: %d row(s), truncated=%v by=%q\n", result.RowCount, result.Truncated, result.TruncatedBy)
}

func fail(step string, err error) {
	fmt.Fprintf(os.Stderr, "%s failed: %v\n", step, err)
	os.Exit(1)
}
