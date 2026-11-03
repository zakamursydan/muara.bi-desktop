//go:build cgo

package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muara.bi/desktop/internal/bundle"
)

// These tests are the live half of FR-11's pre-flight: a bundle whose file
// sources mount in a scratch session, then every View's SQL is DESCRIBEd and
// marked merah/kuning/hijau. The static half (alias conflicts, dedupe) is
// covered in internal/bundle; the open document's session must survive this
// check untouched.

// checkBundle writes a bundle with every fate a View's schema can meet: a
// clean resolve, a missing table, a missing column, a type mismatch, a SYNCED
// View this build cannot compile, a server source this build cannot attach, a
// SQLite source, a file that is not there, and a broken statement.
func checkBundle(t *testing.T, folder string) string {
	t.Helper()
	copyFixture(t, "spike.csv", filepath.Join(folder, "data", "orders.csv"))
	copyFixture(t, "spike.json", filepath.Join(folder, "data", "events.json"))
	copyFixture(t, "spike.sqlite", filepath.Join(folder, "data", "warehouse.sqlite"))

	path := filepath.Join(folder, "check.muara")
	b := &bundle.Bundle{
		Connections: []bundle.Connection{
			{ID: "c-orders", Alias: "orders", Driver: bundle.DriverFile,
				File: &bundle.FileSource{Path: filepath.Join(folder, "data", "orders.csv")}},
			{ID: "c-events", Alias: "events", Driver: bundle.DriverFile,
				File: &bundle.FileSource{Path: filepath.Join(folder, "data", "events.json")}},
			{ID: "c-wh", Alias: "wh", Driver: bundle.DriverSQLite, Database: "warehouse",
				File: &bundle.FileSource{Path: filepath.Join(folder, "data", "warehouse.sqlite")}},
			{ID: "c-gone", Alias: "gone", Driver: bundle.DriverFile,
				File: &bundle.FileSource{Path: filepath.Join(folder, "data", "gone.csv")}},
			{ID: "c-sales", Alias: "sales", Driver: bundle.DriverPostgres, Database: "dwh",
				Address: &bundle.Address{Host: "db.internal", Port: 5432, User: "analyst"}},
		},
		SemanticModels: []bundle.SemanticModel{
			// Listed before its dependency on purpose: the retry pass has to
			// turn this ordering into a green line, not a false merah.
			{ID: "sm-rev", Name: "revenue_by_channel", RawSQL: "SELECT channel, SUM(total) AS revenue FROM order_lines GROUP BY 1",
				Aliases: []string{"orders"}},
			{ID: "sm-lines", Name: "order_lines", RawSQL: "SELECT id, channel, total FROM orders",
				Aliases: []string{"orders"}},
			{ID: "sm-bad", Name: "bad_model", RawSQL: "SELECT * FROM ghost_table",
				Aliases: []string{"orders"}},
		},
		Views: []bundle.View{
			{ID: "v-good", Name: "Revenue", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL:    "SELECT channel, SUM(total) AS revenue FROM order_lines GROUP BY 1",
				Aliases:   []string{"orders"},
				ModelRefs: []string{"sm-lines"}},
			{ID: "v-table", Name: "Ghost table", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT * FROM ghost_table", Aliases: []string{"orders"}},
			{ID: "v-column", Name: "Ghost column", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT nope FROM order_lines", Aliases: []string{"orders"}},
			{ID: "v-type", Name: "Wrong type", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT SUM(channel) AS x FROM order_lines", Aliases: []string{"orders"}},
			{ID: "v-synced", Name: "Synced", Kind: bundle.KindTable, QueryMode: bundle.QuerySynced,
				BuilderState: []byte(`{"table":"orders"}`), Aliases: []string{"orders"}},
			{ID: "v-server", Name: "Server", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT COUNT(*) FROM sales.public.orders", Aliases: []string{"sales"}},
			{ID: "v-sqlite", Name: "SQLite", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT COUNT(*) AS n FROM wh.orders", Aliases: []string{"wh"}},
			{ID: "v-gone", Name: "Gone file", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT * FROM gone", Aliases: []string{"gone"}},
			{ID: "v-broken", Name: "Broken", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT FROM", Aliases: []string{"orders"}},
		},
	}
	if err := bundle.Save(path, b); err != nil {
		t.Fatalf("save check bundle: %v", err)
	}
	return path
}

func lineBy(lines []ImportCheckLine, kind, name string) (ImportCheckLine, bool) {
	for _, l := range lines {
		if l.Kind == kind && l.Name == name {
			return l, true
		}
	}
	return ImportCheckLine{}, false
}

func wantLine(t *testing.T, lines []ImportCheckLine, kind, name, level, wantReason string) {
	t.Helper()
	line, ok := lineBy(lines, kind, name)
	if !ok {
		t.Fatalf("no %s line for %q in %v", kind, name, lines)
	}
	if line.Level != level {
		t.Errorf("%s %q level = %q, want %q (reason: %s)", kind, name, line.Level, level, line.Reason)
	}
	if wantReason != "" && !strings.Contains(line.Reason, wantReason) {
		t.Errorf("%s %q reason = %q, want it to contain %q", kind, name, line.Reason, wantReason)
	}
}

func TestLiveCheckMarksEveryViewRedYellowOrGreen(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	lines, err := service.ImportLiveCheck(checkBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("live check: %v", err)
	}
	if len(lines) != 12 {
		t.Fatalf("lines = %d, want 3 models + 9 views", len(lines))
	}

	// Models: the reversed listing order must come out green thanks to the
	// retry pass, and a model reading a missing table is merah.
	wantLine(t, lines, "semantic_model", "revenue_by_channel", "hijau", "")
	wantLine(t, lines, "semantic_model", "order_lines", "hijau", "")
	wantLine(t, lines, "semantic_model", "bad_model", "merah", "ghost_table")

	// Views, one per fate.
	wantLine(t, lines, "view", "Revenue", "hijau", "2 kolom")
	wantLine(t, lines, "view", "Ghost table", "merah", "ghost_table")
	wantLine(t, lines, "view", "Ghost column", "merah", "nope")
	wantLine(t, lines, "view", "Wrong type", "kuning", "No function matches")
	wantLine(t, lines, "view", "Synced", "kuning", "FR-4")
	wantLine(t, lines, "view", "Server", "kuning", "sales")
	wantLine(t, lines, "view", "SQLite", "hijau", "1 kolom")
	wantLine(t, lines, "view", "Gone file", "merah", "gone")
	wantLine(t, lines, "view", "Broken", "merah", "Parser Error")
}

// The check must not touch the open document's session: the sources it mounts
// live in a scratch session, so a query that used to answer still answers and
// the document is still the same file.
func TestLiveCheckLeavesTheOpenDocumentAlone(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	doc := workBundle(t, t.TempDir())
	if _, err := service.Open(doc); err != nil {
		t.Fatalf("open: %v", err)
	}
	before, err := service.Run("SELECT COUNT(*) AS n FROM order_lines")
	if err != nil {
		t.Fatalf("query before: %v", err)
	}

	if _, err := service.ImportLiveCheck(checkBundle(t, t.TempDir())); err != nil {
		t.Fatalf("live check: %v", err)
	}

	info, err := service.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if !info.Open || info.Path != doc {
		t.Fatalf("document = %+v, want %s still open", info, doc)
	}
	after, err := service.Run("SELECT COUNT(*) AS n FROM order_lines")
	if err != nil {
		t.Fatalf("query after: %v", err)
	}
	if asText(after.Rows[0]["n"]) != asText(before.Rows[0]["n"]) {
		t.Errorf("count = %v after the check, want %v", after.Rows[0]["n"], before.Rows[0]["n"])
	}
	// The scratch session's own sources must not leak into the document's
	// session either.
	if _, err := service.Run("SELECT COUNT(*) FROM gone"); err == nil {
		t.Error("the scratch session's alias answers in the document's session")
	}
}

func TestLiveCheckRefusesSomethingThatIsNotABundle(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	bad := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(bad, []byte("bukan bundle"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := service.ImportLiveCheck(bad); !errors.Is(err, bundle.ErrNotABundle) {
		t.Errorf("live check of a text file = %v, want ErrNotABundle", err)
	}
}
