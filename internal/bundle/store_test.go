package bundle_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"muara.bi/desktop/internal/bundle"
	"muara.bi/engine/duckdb"
)

// The timestamp is pinned so a save of identical content can be compared byte
// for byte — that is what an import relies on when it dedupes a bundle that was
// handed over twice.
var stamp = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// rawJSON stays compact on purpose. The store writes an entry with MarshalIndent,
// which re-indents the raw messages it embeds — so the file on disk is prettier
// than what the caller held while carrying the same document. Round trips are
// therefore compared as decoded documents, not as bytes; whitespace is not a
// change to a View.
const (
	builderState = `{"dimensions":["channel"],"metrics":[{"agg":"sum","column":"total"}]}`
	presentation = `{"legend":false,"colors":["#1B8A6B"]}`
	viewSQL      = `SELECT channel, SUM(total) AS revenue FROM sales.orders GROUP BY 1`
	layoutDoc    = `{"cols":12,"rows":[{"view_id":"v-1","x":0,"y":0,"w":6,"h":4}]}`
	globalFilter = `{"field":"orders.channel","kind":"multi"}`
	threshold    = `{"op":"gt","value":1000}`
)

func fileSource(dir, name string) string {
	return filepath.Join(dir, "data", name)
}

// fullBundle carries one of everything the format defines, which is what makes
// the round-trip claim worth testing: an empty bundle round-trips by accident.
func fullBundle(anchor string) *bundle.Bundle {
	orders := fileSource(anchor, "orders.csv")
	return &bundle.Bundle{
		Manifest: bundle.Manifest{
			CreatedAt:         stamp,
			ExportedBy:        "zakmursydan",
			OriginWorkspaceID: "ws-7f3a",
		},
		Projects: []bundle.Project{{
			ID:            "p-1",
			Name:          "Regional",
			Color:         "#1B8A6B",
			Note:          "laporan mingguan",
			DashboardRefs: []string{"d-1"},
			ViewPins:      []string{"v-1"},
		}},
		Connections: []bundle.Connection{
			{
				ID:           "c-1",
				Alias:        "sales",
				Driver:       bundle.DriverPostgres,
				Database:     "dwh",
				SchemaFilter: []string{"public"},
				Note:         "gudang utama",
				Address: &bundle.Address{
					Host: "db.internal", Port: 5432, User: "analyst", SSLMode: "verify-full",
				},
				SSH: &bundle.SSHTarget{Host: "bastion.internal", Port: 22, User: "tunnel"},
			},
			{
				ID:     "c-2",
				Alias:  "orders",
				Driver: bundle.DriverFile,
				Note:   "csv drop",
				File:   &bundle.FileSource{Path: orders},
			},
		},
		SemanticModels: []bundle.SemanticModel{{
			ID:          "sm-1",
			Name:        "Order lines",
			Description: "one row per line",
			RawSQL:      "SELECT * FROM sales.orders",
			Aliases:     []string{"sales"},
		}},
		Views: []bundle.View{
			{
				ID:           "v-1",
				Name:         "Revenue per channel",
				Folder:       "weekly",
				Kind:         bundle.KindTable,
				QueryMode:    bundle.QuerySynced,
				BuilderState: json.RawMessage(builderState),
				Presentation: json.RawMessage(presentation),
				MaxRows:      1000,
				Aliases:      []string{"sales"},
			},
			{
				ID:        "v-2",
				Name:      "Revenue trend",
				Kind:      bundle.KindLine,
				QueryMode: bundle.QueryCustomSQL,
				RawSQL:    viewSQL,
				// A pinned max_rows and cache_ttl are part of the definition, so they
				// have to survive the round trip on their own.
				MaxRows:         5000,
				CacheTTLSeconds: 300,
				Aliases:         []string{"sales", "orders"},
			},
		},
		Dashboards: []bundle.Dashboard{{
			ID:                     "d-1",
			Name:                   "Monday pack",
			Layout:                 json.RawMessage(layoutDoc),
			ViewRefs:               []string{"v-1", "v-2"},
			GlobalFilters:          json.RawMessage(globalFilter),
			RefreshIntervalSeconds: 60,
		}},
		Alerts: []bundle.Alert{{
			ID:        "a-1",
			ViewID:    "v-2",
			Threshold: json.RawMessage(threshold),
			Channel:   "telegram",
			Schedule:  "07:00",
		}},
	}
}

// writeBundleFile saves b and returns the path, with the data folder the file
// source points at actually created.
func writeBundleFile(t *testing.T, dir string, b *bundle.Bundle) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatalf("data folder: %v", err)
	}
	if err := os.WriteFile(fileSource(dir, "orders.csv"), []byte("id,channel,total\n1,wholesale,900\n"), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	path := filepath.Join(dir, "regional.muara")
	if err := bundle.Save(path, b); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

// sameDocument compares two values by what a reader of the bundle would see:
// decoded JSON, so the indentation the store applies to an embedded raw message
// cannot be mistaken for a changed definition.
func sameDocument(got, want any) bool {
	left, err := json.Marshal(got)
	if err != nil {
		panic(err)
	}
	right, err := json.Marshal(want)
	if err != nil {
		panic(err)
	}
	var decodedLeft, decodedRight any
	if err := json.Unmarshal(left, &decodedLeft); err != nil {
		panic(err)
	}
	if err := json.Unmarshal(right, &decodedRight); err != nil {
		panic(err)
	}
	return reflect.DeepEqual(decodedLeft, decodedRight)
}

func TestSaveThenOpenKeepsEveryViewIntact(t *testing.T) {
	dir := t.TempDir()
	want := fullBundle(dir)
	path := writeBundleFile(t, dir, want)

	got, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	if !sameDocument(got.Projects, want.Projects) {
		t.Errorf("projects changed:\n got %#v\nwant %#v", got.Projects, want.Projects)
	}
	if !sameDocument(got.SemanticModels, want.SemanticModels) {
		t.Errorf("semantic models changed:\n got %#v\nwant %#v", got.SemanticModels, want.SemanticModels)
	}
	if !sameDocument(got.Dashboards, want.Dashboards) {
		t.Errorf("dashboards changed:\n got %#v\nwant %#v", got.Dashboards, want.Dashboards)
	}
	if !sameDocument(got.Alerts, want.Alerts) {
		t.Errorf("alerts changed:\n got %#v\nwant %#v", got.Alerts, want.Alerts)
	}

	// The View is the entity M3 is measured on, so every field of it is compared
	// rather than the count.
	if !sameDocument(got.Views, want.Views) {
		t.Errorf("views changed:\n got %#v\nwant %#v", got.Views, want.Views)
	}

	// The manifest is written by the app, so only the fields the caller set are
	// compared against the opened copy.
	if got.Manifest.Format != bundle.Format || got.Manifest.Version != bundle.FormatVersion {
		t.Errorf("manifest = %q version %d, want %q version %d",
			got.Manifest.Format, got.Manifest.Version, bundle.Format, bundle.FormatVersion)
	}
	if !got.Manifest.CreatedAt.Equal(stamp) {
		t.Errorf("created_at = %v, want the pinned %v", got.Manifest.CreatedAt, stamp)
	}
	if got.Manifest.MinAppVersion != bundle.AppVersion {
		t.Errorf("min_app_version = %q, want %q", got.Manifest.MinAppVersion, bundle.AppVersion)
	}
	if got.Manifest.ExportedBy != "zakmursydan" || got.Manifest.OriginWorkspaceID != "ws-7f3a" {
		t.Errorf("manifest lost its origin: exported_by=%q workspace=%q",
			got.Manifest.ExportedBy, got.Manifest.OriginWorkspaceID)
	}
	if got.Manifest.App.Name != bundle.AppName {
		t.Errorf("app.name = %q, want %q", got.Manifest.App.Name, bundle.AppName)
	}
}

func TestEmptyBundleStillWritesEveryEntry(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Untitled.muara")
	empty := &bundle.Bundle{Manifest: bundle.Manifest{CreatedAt: stamp}}
	if err := bundle.Save(path, empty); err != nil {
		t.Fatalf("save: %v", err)
	}

	names := zipEntries(t, path)
	want := []string{
		"manifest.json", "projects.json", "connections.json",
		"semantic_models.json", "views.json", "dashboards.json", "alerts.json",
	}
	if !reflect.DeepEqual(names, want) {
		t.Errorf("entries = %v, want exactly %v", names, want)
	}

	// An entity list that happens to be empty stays a JSON array, so a person
	// reading the bundle by hand sees [] instead of null.
	for _, name := range want[1:] {
		body := zipEntry(t, path, name)
		if strings.TrimSpace(string(body)) != "[]" {
			t.Errorf("%s = %s, want []", name, bytes.TrimSpace(body))
		}
	}

	got, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(got.Views)+len(got.Connections)+len(got.Projects)+len(got.Dashboards)+
		len(got.Alerts)+len(got.SemanticModels) != 0 {
		t.Errorf("empty bundle opened with content: %#v", got)
	}
}

func TestSaveOfSameContentIsByteIdentical(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatalf("data folder: %v", err)
	}
	if err := os.WriteFile(fileSource(dir, "orders.csv"), []byte("id\n1\n"), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}
	first := filepath.Join(dir, "first.muara")
	second := filepath.Join(dir, "second.muara")
	for _, path := range []string{first, second} {
		if err := bundle.Save(path, fullBundle(dir)); err != nil {
			t.Fatalf("save %s: %v", path, err)
		}
	}
	a, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	b, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Errorf("two saves of the same bundle differ (%d vs %d bytes) — an import cannot hash them",
			len(a), len(b))
	}
}

// M3 promises a save cannot destroy the file it is replacing. Both failure
// paths are exercised: one that never starts writing, and one that fails after
// the archive is complete.
func TestFailedSaveLeavesThePreviousBundleAlone(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(t, dir, fullBundle(dir))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}

	broken := fullBundle(dir)
	broken.Views[0].Kind = bundle.Kind("doughnut")
	if err := bundle.Save(path, broken); err == nil {
		t.Fatal("a view with an unknown kind saved")
	} else if !errors.Is(err, bundle.ErrInvalid) {
		t.Errorf("error = %v, want it to be recognisable as an invalid definition", err)
	}

	// A target that is a folder fails after the archive is written, which is the
	// case the tmp → rename ordering exists for.
	if err := bundle.Save(filepath.Join(dir, "data"), fullBundle(dir)); err == nil {
		t.Fatal("saving onto a folder succeeded")
	}

	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("a refused save changed %s (%d → %d bytes)", path, len(before), len(after))
	}

	leftovers := []string{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".muara-save-") {
			leftovers = append(leftovers, entry.Name())
		}
	}
	if len(leftovers) > 0 {
		t.Errorf("refused saves left temp files behind: %v", leftovers)
	}
}

func TestSaveRejectsBrokenDefinitions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.muara")

	cases := []struct {
		name   string
		mutate func(*bundle.Bundle)
		want   string
	}{
		{"duplicate view id", func(b *bundle.Bundle) {
			b.Views = append(b.Views, b.Views[0])
		}, "duplicate view id"},
		{"duplicate alias", func(b *bundle.Bundle) {
			b.Connections = append(b.Connections, bundle.Connection{
				ID: "c-3", Alias: "sales", Driver: bundle.DriverSQLite,
			})
		}, "duplicate alias"},
		{"alias that is not an identifier", func(b *bundle.Bundle) {
			b.Connections[0].Alias = "1 sales"
		}, "plain identifier"},
		{"unknown driver", func(b *bundle.Bundle) {
			b.Connections[0].Driver = bundle.Driver("oracle")
		}, "driver"},
		{"file source without a path", func(b *bundle.Bundle) {
			b.Connections[1].File = &bundle.FileSource{}
		}, "names no path"},
		{"server driver carrying a file", func(b *bundle.Bundle) {
			b.Connections[0].File = &bundle.FileSource{Path: "data/x.csv"}
		}, "yet carries a file source"},
		{"project colour that is not hex", func(b *bundle.Bundle) {
			b.Projects[0].Color = "red"
		}, "#rrggbb"},
		{"synced view with no builder state", func(b *bundle.Bundle) {
			b.Views[0].BuilderState = nil
		}, "no builder_state"},
		{"custom sql view with no sql", func(b *bundle.Bundle) {
			b.Views[1].RawSQL = "  "
		}, "no raw_sql"},
		{"view reading an alias the bundle does not define", func(b *bundle.Bundle) {
			b.Views[0].Aliases = []string{"sales", "crm"}
		}, "does not define"},
		{"project pinning a view that is not there", func(b *bundle.Bundle) {
			b.Projects[0].ViewPins = []string{"v-9"}
		}, "not in this bundle"},
		{"dashboard showing a missing view", func(b *bundle.Bundle) {
			b.Dashboards[0].ViewRefs = []string{"v-9"}
		}, "not in this bundle"},
		{"alert watching a missing view", func(b *bundle.Bundle) {
			b.Alerts[0].ViewID = "v-9"
		}, "watches view"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := fullBundle(dir)
			tc.mutate(b)
			err := bundle.Save(path, b)
			if err == nil {
				t.Fatalf("saved a bundle with %s", tc.name)
			}
			if !errors.Is(err, bundle.ErrInvalid) {
				t.Errorf("error = %v, want ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not say %q", err, tc.want)
			}
			if _, statErr := os.Stat(path); statErr == nil {
				t.Errorf("a refused save still created %s", path)
			}
		})
	}
}

func TestOpenRefusesABundleItDoesNotUnderstand(t *testing.T) {
	dir := t.TempDir()
	valid := filepath.Join(dir, "valid.muara")
	if err := bundle.Save(valid, &bundle.Bundle{Manifest: bundle.Manifest{CreatedAt: stamp}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	body := map[string]string{}
	for _, name := range zipEntries(t, valid) {
		body[name] = string(zipEntry(t, valid, name))
	}

	cases := []struct {
		name    string
		mutate  map[string]string
		drop    []string
		extra   map[string]string
		wantErr error
		wantMsg string
	}{
		{
			name:    "format this app never wrote",
			mutate:  map[string]string{"manifest.json": `{"format":"muara.weird","version":1,"min_app_version":"0.1.0"}`},
			wantErr: bundle.ErrUnknownFormat,
			wantMsg: "muara.weird",
		},
		{
			name:    "newer format version",
			mutate:  map[string]string{"manifest.json": `{"format":"muara.bundle","version":2,"min_app_version":"0.1.0"}`},
			wantErr: bundle.ErrTooNewFormat,
			wantMsg: "update muara.bi Desktop",
		},
		{
			name:    "older format version",
			mutate:  map[string]string{"manifest.json": `{"format":"muara.bundle","version":0,"min_app_version":"0.1.0"}`},
			wantErr: bundle.ErrUnknownFormat,
			wantMsg: "format version 0",
		},
		{
			name:    "needs a newer app",
			mutate:  map[string]string{"manifest.json": `{"format":"muara.bundle","version":1,"min_app_version":"9.9.0"}`},
			wantErr: bundle.ErrNeedsNewerApp,
			wantMsg: "9.9.0",
		},
		{
			name:    "manifest with no format",
			mutate:  map[string]string{"manifest.json": `{"version":1,"min_app_version":"0.1.0"}`},
			wantErr: bundle.ErrNotABundle,
			wantMsg: "no format",
		},
		{
			name:    "entity entry missing",
			drop:    []string{"views.json"},
			wantErr: bundle.ErrNotABundle,
			wantMsg: "missing views.json",
		},
		{
			name:    "entry this format does not define",
			extra:   map[string]string{"widgets.json": "[]"},
			wantErr: bundle.ErrUnknownFormat,
			wantMsg: "widgets.json",
		},
		{
			name:    "encrypted credential block",
			extra:   map[string]string{"secrets.enc": "not-a-secret-in-this-build"},
			wantErr: bundle.ErrUnknownFormat,
			wantMsg: "cannot read an encrypted credential block",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "case.muara")
			writeZip(t, path, tc.mutate, tc.drop, tc.extra, body)
			_, err := bundle.Open(path)
			if err == nil {
				t.Fatalf("opened a bundle with %s", tc.name)
			}
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("error = %v, want %v", err, tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantMsg) {
				t.Errorf("error %q does not mention %q", err, tc.wantMsg)
			}
		})
	}

	t.Run("not a zip at all", func(t *testing.T) {
		path := filepath.Join(dir, "notes.txt")
		if err := os.WriteFile(path, []byte("bukan bundle"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, err := bundle.Open(path); !errors.Is(err, bundle.ErrNotABundle) {
			t.Errorf("error = %v, want ErrNotABundle", err)
		}
	})

	t.Run("a folder is not a bundle", func(t *testing.T) {
		if _, err := bundle.Open(dir); !errors.Is(err, bundle.ErrNotABundle) {
			t.Errorf("error = %v, want ErrNotABundle", err)
		}
	})

	t.Run("the same entry twice", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "twice.muara")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create: %v", err)
		}
		zw := zip.NewWriter(f)
		for range 2 {
			w, err := zw.Create("manifest.json")
			if err != nil {
				t.Fatalf("create entry: %v", err)
			}
			fmt.Fprint(w, `{"format":"muara.bundle","version":1,"min_app_version":"0.1.0"}`)
		}
		zw.Close()
		f.Close()
		if _, err := bundle.Open(path); !errors.Is(err, bundle.ErrNotABundle) {
			t.Errorf("error = %v, want ErrNotABundle for a duplicated entry", err)
		}
	})
}

// §5.4 requires an import to be able to say "this view needs the alias sales,
// which this machine does not have" — so opening must not refuse the file for it.
func TestOpenKeepsADanglingAliasInsteadOfRefusingIt(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(t, dir, fullBundle(dir))

	replacements := map[string]string{
		"connections.json": `[{"id":"c-2","alias":"orders","driver":"file","file":{"path":"data/orders.csv"}}]`,
	}
	body := map[string]string{}
	for _, name := range zipEntries(t, path) {
		body[name] = string(zipEntry(t, path, name))
	}
	rebuilt := filepath.Join(t.TempDir(), "needs-alias.muara")
	writeZip(t, rebuilt, replacements, nil, nil, body)

	got, err := bundle.Open(rebuilt)
	if err != nil {
		t.Fatalf("a bundle whose view needs a missing alias must still open: %v", err)
	}
	if len(got.Views) != 2 {
		t.Errorf("opened %d views, want both still there", len(got.Views))
	}
	if len(got.Connections) != 1 || got.Connections[0].Alias != "orders" {
		t.Errorf("connections = %#v, want only orders", got.Connections)
	}
}

func TestFileSourcePathsTravelRelativeToTheBundle(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(t, dir, fullBundle(dir))

	got, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	stored := got.Connections[1].File.Path
	if filepath.IsAbs(stored) {
		t.Errorf("stored path = %q, want it relative to the bundle folder", stored)
	}
	if stored != "data/orders.csv" {
		t.Errorf("stored path = %q, want data/orders.csv", stored)
	}

	resolved, err := got.ResolveFileSources(path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved["orders"] != fileSource(dir, "orders.csv") {
		t.Errorf("resolved = %q, want %q", resolved["orders"], fileSource(dir, "orders.csv"))
	}
	if _, err := os.Stat(resolved["orders"]); err != nil {
		t.Errorf("resolved path does not exist: %v", err)
	}
}

// The exit criterion says a bundle and its data may sit in different folders on
// the same machine. Moving the pair has to re-point the source without touching
// the bundle, and a source that never lived inside the bundle folder has to
// stay the absolute path it was stored as.
func TestBundleAndDataInDifferentFoldersStillResolve(t *testing.T) {
	roots := t.TempDir()
	first := filepath.Join(roots, "one", "work")
	second := filepath.Join(roots, "two", "team")
	shared := filepath.Join(roots, "shared")
	for _, dir := range []string{first, second, shared} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(shared, "rates.csv"), []byte("day,rate\n1,2\n"), 0o644); err != nil {
		t.Fatalf("fixture: %v", err)
	}

	b := fullBundle(first)
	// A file outside the bundle folder is machine-local, and §5.5 says the
	// definition must admit that rather than hide it behind ../ hops.
	b.Connections = append(b.Connections, bundle.Connection{
		ID:     "c-3",
		Alias:  "rates",
		Driver: bundle.DriverFile,
		File:   &bundle.FileSource{Path: filepath.Join(shared, "rates.csv")},
	})
	path := writeBundleFile(t, first, b)

	moved := filepath.Join(second, filepath.Base(path))
	data := filepath.Join(second, "data")
	if err := os.MkdirAll(data, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Rename(path, moved); err != nil {
		t.Fatalf("move bundle: %v", err)
	}
	if err := os.Rename(filepath.Join(first, "data", "orders.csv"), filepath.Join(data, "orders.csv")); err != nil {
		t.Fatalf("move data: %v", err)
	}

	got, err := bundle.Open(moved)
	if err != nil {
		t.Fatalf("open after the move: %v", err)
	}
	resolved, err := got.ResolveFileSources(moved)
	if err != nil {
		t.Fatalf("resolve after the move: %v", err)
	}
	if resolved["orders"] != filepath.Join(data, "orders.csv") {
		t.Errorf("bundle-relative source resolved to %q, want it under the new folder %q",
			resolved["orders"], data)
	}
	if resolved["rates"] != filepath.Join(shared, "rates.csv") {
		t.Errorf("machine-local source moved with the bundle: %q", resolved["rates"])
	}
	for alias, p := range resolved {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("%s resolves to an unreadable path: %v", alias, err)
		}
	}
}

func TestFileSourcePathThatCannotBePlacedIsRefused(t *testing.T) {
	dir := t.TempDir()

	cases := []struct {
		name    string
		stored  string
		wantErr error
	}{
		{"tilde is never expanded", "~/orders.csv", duckdb.ErrFileSourceHomeRelative},
		{"climbing above the bundle folder", "../orders.csv", duckdb.ErrFileSourceOutsideAnchor},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			b := fullBundle(dir)
			b.Connections[1].File = &bundle.FileSource{Path: tc.stored}
			err := bundle.Save(filepath.Join(dir, "refused.muara"), b)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("save error = %v, want %v", err, tc.wantErr)
			}
		})
	}

	// The same two forms arriving inside a file someone else wrote.
	for _, tc := range cases {
		t.Run(tc.name+" when reading", func(t *testing.T) {
			readDir := t.TempDir()
			path := writeBundleFile(t, readDir, fullBundle(readDir))
			body := map[string]string{}
			for _, name := range zipEntries(t, path) {
				body[name] = string(zipEntry(t, path, name))
			}
			connections := fmt.Sprintf(
				`[{"id":"c-2","alias":"orders","driver":"file","file":{"path":%q}}]`, tc.stored)
			rebuilt := filepath.Join(t.TempDir(), "escapes.muara")
			writeZip(t, rebuilt, map[string]string{"connections.json": connections}, nil, nil, body)
			_, err := bundle.Open(rebuilt)
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("open error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

// A missing source file is one of the three fates §5.5 says pre-flight has to
// report, so opening a bundle must not turn it into an error.
func TestOpeningABundleWhoseDataFileIsGoneStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(t, dir, fullBundle(dir))
	if err := os.Remove(fileSource(dir, "orders.csv")); err != nil {
		t.Fatalf("remove fixture: %v", err)
	}

	got, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	resolved, err := got.ResolveFileSources(path)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if _, err := os.Stat(resolved["orders"]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("resolved path %q is not the missing file: %v", resolved["orders"], err)
	}
}

// zipEntries and zipEntry read a bundle the way a person would: unzip it and
// look inside, without the store involved.
func zipEntries(t *testing.T, path string) []string {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	names := make([]string, 0, len(reader.File))
	for _, entry := range reader.File {
		names = append(names, entry.Name)
	}
	return names
}

func zipEntry(t *testing.T, path, name string) []byte {
	t.Helper()
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Name != name {
			continue
		}
		file, err := entry.Open()
		if err != nil {
			t.Fatalf("open entry: %v", err)
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			t.Fatalf("read entry: %v", err)
		}
		return data
	}
	t.Fatalf("%s has no entry %q", path, name)
	return nil
}

// writeZip rebuilds a bundle with entries replaced, dropped or added — the way
// a newer app, or a hand-edited file, would present itself.
func writeZip(t *testing.T, path string, replace map[string]string, drop []string, extra map[string]string, base map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	zw := zip.NewWriter(f)
	dropped := map[string]bool{}
	for _, name := range drop {
		dropped[name] = true
	}
	write := func(name, body string) {
		entry, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := io.WriteString(entry, body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	for _, name := range []string{"manifest.json", "projects.json", "connections.json",
		"semantic_models.json", "views.json", "dashboards.json", "alerts.json"} {
		if dropped[name] {
			continue
		}
		body, ok := replace[name]
		if !ok {
			body = base[name]
		}
		write(name, body)
	}
	for name, body := range extra {
		write(name, body)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close file: %v", err)
	}
}
