//go:build cgo

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"muara.bi/desktop/internal/bundle"
	"muara.bi/desktop/internal/profile"
)

// These tests are the part of BRD §5.1 that does not need a mouse: a bundle on
// disk becomes the one document the app works on, its file sources answer real
// queries, and everything the machine cannot resolve comes back as a line in the
// mount report instead of a lost bundle.

func copyFixture(t *testing.T, name, dest string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("tests", "fixtures", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatalf("create folder for %s: %v", dest, err)
	}
	if err := os.WriteFile(dest, raw, 0o644); err != nil {
		t.Fatalf("write %s: %v", dest, err)
	}
	return dest
}

// workBundle writes a bundle whose sources are all readable on this machine, plus
// one server source this machine has no credential for. It is the shape a real
// analyst's file would have: a CSV, a SQLite database, and a warehouse they reach
// over a VPN.
func workBundle(t *testing.T, folder string) string {
	t.Helper()
	csv := copyFixture(t, "spike.csv", filepath.Join(folder, "data", "orders.csv"))
	sqlite := copyFixture(t, "spike.sqlite", filepath.Join(folder, "data", "warehouse.sqlite"))

	path := filepath.Join(folder, "work.muara")
	b := &bundle.Bundle{
		Manifest: bundle.Manifest{
			CreatedAt:  time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC),
			ExportedBy: "zakmursydan",
		},
		Connections: []bundle.Connection{
			{ID: "c-1", Alias: "orders", Driver: bundle.DriverFile,
				File: &bundle.FileSource{Path: csv}},
			{ID: "c-2", Alias: "wh", Driver: bundle.DriverSQLite, Database: "warehouse",
				File: &bundle.FileSource{Path: sqlite}},
			{ID: "c-3", Alias: "sales", Driver: bundle.DriverPostgres, Database: "dwh",
				SchemaFilter: []string{"public"},
				Address:      &bundle.Address{Host: "db.internal", Port: 5432, User: "analyst", SSLMode: "require"}},
		},
		SemanticModels: []bundle.SemanticModel{{
			ID: "sm-1", Name: "order_lines", RawSQL: "SELECT id, channel, total FROM orders",
			Aliases: []string{"orders"},
		}},
		Views: []bundle.View{{
			ID: "v-1", Name: "Revenue per channel", Kind: bundle.KindTable,
			QueryMode: bundle.QueryCustomSQL,
			RawSQL:    "SELECT channel, SUM(total) AS revenue FROM order_lines GROUP BY 1",
			Aliases:   []string{"orders"},
		}},
	}
	if err := bundle.Save(path, b); err != nil {
		t.Fatalf("save work bundle: %v", err)
	}
	return path
}

func mountBy(reports []MountReport, name string) (MountReport, bool) {
	for _, r := range reports {
		if r.Name == name {
			return r, true
		}
	}
	return MountReport{}, false
}

func TestOpeningABundleMakesItsSourcesAnswerQueries(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	path := workBundle(t, t.TempDir())
	info, err := service.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if !info.Open || info.Path != path || info.Name != "work.muara" {
		t.Fatalf("document = %+v, want work.muara open at %s", info, path)
	}
	if info.Connections != 3 || info.Views != 1 || info.SemanticModels != 1 {
		t.Errorf("counts = connections %d, views %d, models %d, want 3, 1, 1",
			info.Connections, info.Views, info.SemanticModels)
	}

	// A CSV registered as a view and a SQLite database attached as a catalog are
	// two different engine moves; both have to come out of one Open.
	file, ok := mountBy(info.Mounts, "orders")
	if !ok || !file.Mounted {
		t.Errorf("orders mount = %+v, want mounted", file)
	}
	sqlite, ok := mountBy(info.Mounts, "wh")
	if !ok || !sqlite.Mounted {
		t.Errorf("wh mount = %+v, want mounted", sqlite)
	}
	// The stored path was relative to the bundle, so what the app resolved to must
	// be inside that same folder — not the folder of the process.
	if !strings.HasPrefix(sqlite.Target, filepath.Dir(path)) {
		t.Errorf("wh resolved to %q, want it under %s", sqlite.Target, filepath.Dir(path))
	}

	res, err := service.Run("SELECT channel, SUM(total) AS revenue FROM order_lines GROUP BY 1 ORDER BY 2 DESC")
	if err != nil {
		t.Fatalf("run the view's SQL: %v", err)
	}
	if res.RowCount != 3 || res.Truncated {
		t.Errorf("view query = %d rows, truncated=%v, want 3 whole rows", res.RowCount, res.Truncated)
	}
	if got := asText(res.Rows[0]["channel"]); got != "wholesale" {
		t.Errorf("top channel = %q, want wholesale", got)
	}

	attached, err := service.Run("SELECT COUNT(*) AS n FROM wh.orders")
	if err != nil {
		t.Fatalf("query the attached catalog: %v", err)
	}
	if attached.RowCount != 1 || asText(attached.Rows[0]["n"]) != "6" {
		t.Errorf("wh.orders count = %v, want 6", attached.Rows)
	}
}

// The session is replaced with the document on purpose: DuckDB keeps attached
// catalogs and temp views for the life of a session, so reusing it would let a
// query in file B read a server file A attached — quietly, and wrong.
func TestOpeningASecondBundleLosesTheFirstOnesAliases(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	first := workBundle(t, filepath.Join(t.TempDir(), "a"))
	second := filepath.Join(t.TempDir(), "second.muara")
	if err := bundle.Save(second, &bundle.Bundle{
		Connections: []bundle.Connection{{
			ID: "c-1", Alias: "lines", Driver: bundle.DriverFile,
			File: &bundle.FileSource{Path: copyFixture(t, "spike.json", filepath.Join(filepath.Dir(second), "lines.json"))},
		}},
	}); err != nil {
		t.Fatalf("save second: %v", err)
	}

	if _, err := service.Open(first); err != nil {
		t.Fatalf("open first: %v", err)
	}
	if _, err := service.Run("SELECT COUNT(*) FROM order_lines"); err != nil {
		t.Fatalf("first bundle's model should answer: %v", err)
	}

	info, err := service.Open(second)
	if err != nil {
		t.Fatalf("open second: %v", err)
	}
	if info.Path != second {
		t.Errorf("document path = %q, want %q", info.Path, second)
	}
	if _, err := service.Run("SELECT COUNT(*) FROM order_lines"); err == nil {
		t.Error("the first bundle's temp view is still readable after opening another file")
	}
	// The new file's own source answers, so the failure above is the session
	// starting clean, not a broken mount.
	res, err := service.Run("SELECT COUNT(*) AS n FROM lines")
	if err != nil {
		t.Fatalf("second bundle's source should answer: %v", err)
	}
	if asText(res.Rows[0]["n"]) != "6" {
		t.Errorf("lines count = %v, want 6 rows", res.Rows)
	}
}

// A bundle may legitimately name many file sources, and every one of them has
// to come out of a single Open with its own mount line — the count limit M4
// picks will be enforced on top of this, not by it silently dropping sources.
// The measured cost of this shape is in PROGRESS §3: 500 sources mount in
// ~0.8 s on this machine, so the test stays at 200 and asserts no timing.
func TestOpeningABundleWithManySourcesMountsEveryOne(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	const sources = 200
	dir := t.TempDir()
	data := filepath.Join(dir, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatalf("data folder: %v", err)
	}
	b := &bundle.Bundle{}
	for i := 0; i < sources; i++ {
		name := fmt.Sprintf("src_%03d.csv", i)
		if err := os.WriteFile(filepath.Join(data, name), []byte("id\n1\n"), 0o644); err != nil {
			t.Fatalf("fixture %s: %v", name, err)
		}
		b.Connections = append(b.Connections, bundle.Connection{
			ID:     fmt.Sprintf("c-%03d", i),
			Alias:  fmt.Sprintf("src_%03d", i),
			Driver: bundle.DriverFile,
			File:   &bundle.FileSource{Path: filepath.Join(data, name)},
		})
	}
	path := filepath.Join(dir, "many.muara")
	if err := bundle.Save(path, b); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := service.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(info.Mounts) != sources {
		t.Fatalf("mounts = %d lines, want one per source (%d)", len(info.Mounts), sources)
	}
	for _, m := range info.Mounts {
		if !m.Mounted {
			t.Errorf("source %s not mounted: %s", m.Name, m.Reason)
		}
	}
	res, err := service.Run("SELECT COUNT(*) AS n FROM src_199")
	if err != nil {
		t.Fatalf("the last source should answer: %v", err)
	}
	if asText(res.Rows[0]["n"]) != "1" {
		t.Errorf("src_199 count = %v, want 1", res.Rows)
	}
}

func TestAMachineWithoutTheProfileReportsTheAddressItWouldUse(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()
	service.profile = &profile.Profile{
		Machine: "laptop-uni",
		Targets: []profile.Target{{
			Alias: "sales", Host: "10.20.3.4", Port: 5432, User: "analyst",
			Secret: &profile.SecretRef{Kind: profile.SecretKeychain, Label: "muara.sales"},
		}},
	}

	info, err := service.Open(workBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	server, ok := mountBy(info.Mounts, "sales")
	if !ok {
		t.Fatal("no mount line for the postgres source")
	}
	if server.Mounted {
		t.Error("a server source mounted without a keychain reader — it must refuse instead")
	}
	// §5.2: the profile wins over the address the exporter wrote into the bundle,
	// because the bundle carries someone else's server.
	if !strings.Contains(server.Target, "10.20.3.4:5432") {
		t.Errorf("target = %q, want this machine's 10.20.3.4:5432 rather than the bundle's db.internal", server.Target)
	}
	if strings.Contains(server.Target, "db.internal") {
		t.Errorf("target = %q, want the bundle's host replaced by the profile's", server.Target)
	}
	if !strings.Contains(server.Reason, "keychain") || !strings.Contains(server.Reason, "muara.sales") {
		t.Errorf("reason = %q, want it to name the credential source that is missing", server.Reason)
	}
	if !strings.Contains(server.Reason, "profile") {
		t.Errorf("reason = %q, want it to say where the address came from", server.Reason)
	}

	// A machine that knows nothing about the alias still reads the address the
	// exporter left in the bundle, and says which half is missing here: the
	// credential, which is the user's to fill in.
	service.profile = &profile.Profile{}
	info, err = service.Open(workBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("reopen without a profile: %v", err)
	}
	server, _ = mountBy(info.Mounts, "sales")
	if !strings.Contains(server.Target, "db.internal") {
		t.Errorf("target = %q, want the bundle's address when this machine names none", server.Target)
	}
	if !strings.Contains(server.Reason, "credential") {
		t.Errorf("reason = %q, want it to name the missing credential", server.Reason)
	}

	// The other side of that line: a profile that supplies both a host and a
	// readable credential is no longer a setup problem, so the report has to blame
	// the code this build does not have rather than the user's settings.
	t.Setenv("MUARA_TEST_SALES_PW", "hunter2")
	service.profile = &profile.Profile{Targets: []profile.Target{{
		Alias: "sales", Host: "10.20.3.4", Port: 5432,
		Secret: &profile.SecretRef{Kind: profile.SecretEnv, Label: "MUARA_TEST_SALES_PW"},
	}}}
	info, err = service.Open(workBundle(t, t.TempDir()))
	if err != nil {
		t.Fatalf("reopen with a full profile: %v", err)
	}
	server, _ = mountBy(info.Mounts, "sales")
	if !strings.Contains(server.Reason, "conninfo") || server.Mounted {
		t.Errorf("reason = %q, want it to name the builder this build lacks, and still not mount", server.Reason)
	}
	// Everything readable still is.
	file, _ := mountBy(info.Mounts, "orders")
	if !file.Mounted {
		t.Errorf("orders mount = %+v, want mounted even next to an unusable server source", file)
	}
}

func TestADataFileThatMovedIsReportedNotFatal(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	folder := t.TempDir()
	path := workBundle(t, folder)
	if err := os.Remove(filepath.Join(folder, "data", "warehouse.sqlite")); err != nil {
		t.Fatalf("move the data away: %v", err)
	}

	info, err := service.Open(path)
	if err != nil {
		t.Fatalf("a bundle whose data moved must still open: %v", err)
	}
	lost, ok := mountBy(info.Mounts, "wh")
	if !ok {
		t.Fatal("no mount line for the missing SQLite file")
	}
	if lost.Mounted {
		t.Error("a missing file reported as mounted")
	}
	if !strings.Contains(lost.Reason, "warehouse.sqlite") {
		t.Errorf("reason = %q, want it to name the file that is not there", lost.Reason)
	}
	if !strings.Contains(lost.Target, filepath.Dir(path)) {
		t.Errorf("target = %q, want the path the bundle pointed at, so the user can point elsewhere", lost.Target)
	}
	// The rest of the file still works, which is the whole reason this is a report
	// and not an error.
	kept, _ := mountBy(info.Mounts, "orders")
	if !kept.Mounted {
		t.Errorf("orders mount = %+v, want the readable source to mount anyway", kept)
	}
}

func TestNewUntitledMakesAFileTheAppWorksOn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	info, err := service.NewUntitled()
	if err != nil {
		t.Fatalf("new untitled: %v", err)
	}
	if !info.Open || info.Name != "Untitled.muara" {
		t.Fatalf("document = %+v, want an open Untitled.muara", info)
	}
	if info.Path != filepath.Join(home, "Library", "Application Support", "muara.bi", "bundles", "Untitled.muara") &&
		!strings.HasSuffix(info.Path, filepath.Join("muara.bi", "Untitled.muara")) {
		t.Errorf("path = %q, want it inside a muara.bi folder the user can find", info.Path)
	}
	raw, err := os.ReadFile(info.Path)
	if err != nil {
		t.Fatalf("the untitled file is not on disk: %v", err)
	}
	if len(raw) == 0 {
		t.Error("the untitled file is empty, so a crash would have nothing to reopen")
	}
	if info.Dirty {
		t.Error("a freshly created file is reported dirty")
	}

	// Nothing is mounted because nothing is defined — and the window still gets a
	// document it can save into.
	empty, err := service.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if empty.Views != 0 || len(empty.Mounts) != 0 {
		t.Errorf("current = %+v, want an empty document", empty)
	}

	// The second "mulai kosong" must not destroy the first one's only copy.
	first := append([]byte(nil), raw...)
	second, err := service.NewUntitled()
	if err != nil {
		t.Fatalf("second untitled: %v", err)
	}
	if second.Path == info.Path {
		t.Fatal("NewUntitled returned the same path — it would have overwritten the first")
	}
	if filepath.Base(second.Path) != "Untitled-2.muara" {
		t.Errorf("second path = %q, want Untitled-2.muara", second.Path)
	}
	again, err := os.ReadFile(info.Path)
	if err != nil || string(again) != string(first) {
		t.Errorf("the first untitled file changed: %v", err)
	}
}

func TestSaveAsMovesTheDocumentAndReAnchorsItsPaths(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	original := workBundle(t, t.TempDir())
	if _, err := service.Open(original); err != nil {
		t.Fatalf("open: %v", err)
	}

	// The user carries the whole folder: bundle and data together.
	dest := t.TempDir()
	target := filepath.Join(dest, "kerja.muara")
	for _, pair := range []struct{ fixture, name string }{
		{"spike.csv", "orders.csv"},
		{"spike.sqlite", "warehouse.sqlite"},
	} {
		copyFixture(t, pair.fixture, filepath.Join(dest, "data", pair.name))
	}

	info, err := service.SaveAs(target)
	if err != nil {
		t.Fatalf("save as: %v", err)
	}
	if info.Path != target || info.Name != "kerja.muara" {
		t.Fatalf("document = %+v, want the app working on %s", info, target)
	}
	// And the paths inside it are relative to the new folder, not the old one:
	// that is what makes a copied folder a working copy.
	stored, err := bundle.Open(target)
	if err != nil {
		t.Fatalf("reopen the copy: %v", err)
	}
	var storedPaths []string
	for _, c := range stored.Connections {
		if c.File != nil {
			storedPaths = append(storedPaths, c.File.Path)
		}
	}
	// Both file-shaped sources — the CSV and the SQLite database — travel as
	// `data/…`, which is the proof that §5.5's rule covers a SQLite path and not
	// only a file-source one.
	if len(storedPaths) != 2 {
		t.Fatalf("stored paths = %v, want two file-shaped sources", storedPaths)
	}
	for _, p := range storedPaths {
		if !strings.HasPrefix(p, "data/") || strings.Contains(p, string(filepath.Separator)+"data"+string(filepath.Separator)) {
			t.Errorf("stored path = %q, want it to start with data/ and hold no leading slash", p)
		}
	}
	t.Logf("copy stores: %v", storedPaths)
	csv, _ := mountBy(info.Mounts, "orders")
	if !csv.Mounted || !strings.HasPrefix(csv.Target, dest) {
		t.Errorf("orders mount = %+v, want it reading from the new folder %s", csv, dest)
	}

	res, err := service.Run("SELECT COUNT(*) AS n FROM orders")
	if err != nil {
		t.Fatalf("query after SaveAs: %v", err)
	}
	if asText(res.Rows[0]["n"]) != "6" {
		t.Errorf("orders count = %v, want 6 rows from the copy", res.Rows)
	}
}

func TestSavingWithoutADocumentRefusesInsteadOfInventingOne(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	if _, err := service.Save(); err == nil || !strings.Contains(err.Error(), "no bundle is open") {
		t.Errorf("Save with nothing open = %v, want it to refuse", err)
	}
	if _, err := service.SaveAs("someone-elses-name.muara"); err == nil ||
		!strings.Contains(err.Error(), "no bundle is open") {
		t.Errorf("SaveAs with nothing open = %v, want it to refuse", err)
	}

	before, err := service.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if before.Open {
		t.Error("a refused save reported a document as open")
	}
}

func TestOpeningSomethingThatIsNotABundleLeavesTheDocumentAlone(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	path := workBundle(t, t.TempDir())
	if _, err := service.Open(path); err != nil {
		t.Fatalf("open: %v", err)
	}

	bad := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(bad, []byte("bukan bundle"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := service.Open(bad); err == nil || !errors.Is(err, bundle.ErrNotABundle) {
		t.Errorf("open of a text file = %v, want ErrNotABundle", err)
	}

	// The document the user was working on survives a failed Open, including its
	// session: the query that used to answer still answers.
	info, err := service.Current()
	if err != nil {
		t.Fatalf("current: %v", err)
	}
	if info.Path != path {
		t.Errorf("document = %q, want the bundle that was open before", info.Path)
	}
	if _, err := service.Run("SELECT COUNT(*) FROM order_lines"); err != nil {
		t.Errorf("the working document's session is gone after a failed open: %v", err)
	}
}
