//go:build cgo

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"muara.bi/desktop/internal/bundle"
)

// These tests are the app half of the S5 screen: the dialog-free cores behind
// ExportBundle and ImportApply/ImportInspect. The OS dialogs stay out of the
// picture on purpose — no Go test can summon them, and every decision that can
// be proven without a window lives in the _locked cores.

// exportFixture writes a bundle every export scope can name: two projects, one
// dashboard, a folder, an alert, a file source and a server source with an
// address.
func exportFixture(t *testing.T, folder string) string {
	t.Helper()
	csv := copyFixture(t, "spike.csv", filepath.Join(folder, "data", "orders.csv"))

	path := filepath.Join(folder, "export.muara")
	b := &bundle.Bundle{
		Connections: []bundle.Connection{
			{ID: "c-orders", Alias: "orders", Driver: bundle.DriverFile,
				File: &bundle.FileSource{Path: csv}},
			{ID: "c-sales", Alias: "sales", Driver: bundle.DriverPostgres, Database: "dwh",
				Address: &bundle.Address{Host: "db.internal", Port: 5432, User: "analyst", SSLMode: "require"}},
		},
		SemanticModels: []bundle.SemanticModel{{
			ID: "sm-lines", Name: "order_lines", RawSQL: "SELECT id, channel, total FROM orders",
			Aliases: []string{"orders"},
		}},
		Views: []bundle.View{
			{ID: "v-rev", Name: "Revenue", Folder: "laporan", Kind: bundle.KindTable, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT channel, SUM(total) AS revenue FROM order_lines GROUP BY 1", Aliases: []string{"orders"}},
			{ID: "v-sales", Name: "Sales", Folder: "laporan", Kind: bundle.KindKPI, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT COUNT(*) AS n FROM sales.public.orders", Aliases: []string{"sales"}},
		},
		Dashboards: []bundle.Dashboard{{
			ID: "d-1", Name: "Ringkasan", ViewRefs: []string{"v-rev"},
		}},
		Projects: []bundle.Project{
			{ID: "p-1", Name: "Laporan", Color: "#3366cc", DashboardRefs: []string{"d-1"}, ViewPins: []string{"v-rev"}},
			{ID: "p-2", Name: "Operasional", Color: "#33cc66", ViewPins: []string{"v-sales"}},
		},
		Alerts: []bundle.Alert{{ID: "a-1", ViewID: "v-rev", Channel: "telegram", Schedule: "0 9 * * *"}},
	}
	if err := bundle.Save(path, b); err != nil {
		t.Fatalf("save export fixture: %v", err)
	}
	return path
}

// conflictBundle is an incoming file whose alias "orders" collides with the
// workBundle document's "orders": file there, sqlite here. Applying it needs
// the ForceReuseAliases decision, which is exactly what the test proves.
func conflictBundle(t *testing.T, folder string) string {
	t.Helper()
	path := filepath.Join(folder, "incoming.muara")
	b := &bundle.Bundle{
		Connections: []bundle.Connection{
			{ID: "cx-orders", Alias: "orders", Driver: bundle.DriverSQLite, Database: "x",
				File: &bundle.FileSource{Path: filepath.Join(folder, "x.sqlite")}},
		},
		Views: []bundle.View{
			{ID: "vx-1", Name: "Row count", Kind: bundle.KindKPI, QueryMode: bundle.QueryCustomSQL,
				RawSQL: "SELECT COUNT(*) AS n FROM orders", Aliases: []string{"orders"}},
		},
	}
	if err := bundle.Save(path, b); err != nil {
		t.Fatalf("save conflict fixture: %v", err)
	}
	return path
}

func issueByKind(issues []bundle.ImportIssue, kind string) (bundle.ImportIssue, bool) {
	for _, i := range issues {
		if i.Kind == kind {
			return i, true
		}
	}
	return bundle.ImportIssue{}, false
}

func openAt(t *testing.T, path string) *bundle.Bundle {
	t.Helper()
	b, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	return b
}

func TestExportPreviewNamesTheClosure(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	if _, err := service.ExportPreview(ExportRequest{All: true}); err == nil ||
		!strings.Contains(err.Error(), "no bundle is open") {
		t.Fatalf("nil-doc preview err = %v, want the no-bundle refusal", err)
	}

	if _, err := service.Open(exportFixture(t, t.TempDir())); err != nil {
		t.Fatalf("open: %v", err)
	}
	preview, err := service.ExportPreview(ExportRequest{ViewID: "v-rev"})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Views) != 1 || preview.Views[0] != "Revenue" ||
		len(preview.SemanticModels) != 1 || preview.SemanticModels[0] != "order_lines" ||
		len(preview.Connections) != 1 || preview.Connections[0] != "orders" {
		t.Fatalf("preview = %+v", preview)
	}
	if len(preview.Projects) != 0 || len(preview.Dashboards) != 0 || len(preview.Alerts) != 0 {
		t.Fatalf("view scope must not carry layout: %+v", preview)
	}

	all, err := service.ExportPreview(ExportRequest{All: true})
	if err != nil {
		t.Fatalf("preview all: %v", err)
	}
	if len(all.Alerts) != 1 || len(all.Projects) != 2 || len(all.Dashboards) != 1 {
		t.Fatalf("all preview = %+v", all)
	}
}

func TestExportScopeCarriesOnlyWhatTheScopeNames(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	doc := exportFixture(t, t.TempDir())
	if _, err := service.Open(doc); err != nil {
		t.Fatalf("open: %v", err)
	}

	cases := []struct {
		name      string
		req       ExportRequest
		views     int
		models    int
		conns     int
		dashboards int
		projects  int
		alerts    int
	}{
		{"all", ExportRequest{All: true}, 2, 1, 2, 1, 2, 1},
		// v-rev reads order_lines (a model) which reads orders (a file).
		{"view", ExportRequest{ViewID: "v-rev"}, 1, 1, 1, 0, 0, 0},
		// Both views live in "laporan"; v-sales drags the sales connection in.
		{"folder", ExportRequest{Folder: "laporan"}, 2, 1, 2, 0, 0, 0},
		{"dashboard", ExportRequest{DashboardID: "d-1"}, 1, 1, 1, 1, 0, 0},
		// A project references its dashboard and pinned views; it does not own
		// them, so p-2's pins stay home.
		{"project", ExportRequest{ProjectID: "p-1"}, 1, 1, 1, 1, 1, 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out.muara")
			res, err := service.exportLocked(tc.req, out)
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			if res.Cancelled || res.Sealed {
				t.Fatalf("plain export reported cancelled=%v sealed=%v", res.Cancelled, res.Sealed)
			}
			if res.Views != tc.views || res.SemanticModels != tc.models || res.Connections != tc.conns ||
				res.Dashboards != tc.dashboards || res.Projects != tc.projects || res.Alerts != tc.alerts {
				t.Fatalf("result counts = %+v, want views=%d models=%d conns=%d dashboards=%d projects=%d alerts=%d",
					res, tc.views, tc.models, tc.conns, tc.dashboards, tc.projects, tc.alerts)
			}
			b := openAt(t, out)
			if len(b.Views) != tc.views || len(b.SemanticModels) != tc.models || len(b.Connections) != tc.conns ||
				len(b.Dashboards) != tc.dashboards || len(b.Projects) != tc.projects || len(b.Alerts) != tc.alerts {
				t.Fatalf("saved counts = views:%d models:%d conns:%d dashboards:%d projects:%d alerts:%d",
					len(b.Views), len(b.SemanticModels), len(b.Connections), len(b.Dashboards), len(b.Projects), len(b.Alerts))
			}
		})
	}
}

func TestExportTierTwoToggleAndPassphrase(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	doc := exportFixture(t, t.TempDir())
	if _, err := service.Open(doc); err != nil {
		t.Fatalf("open: %v", err)
	}
	salesByAlias := func(b *bundle.Bundle) *bundle.Connection {
		for i := range b.Connections {
			if b.Connections[i].Alias == "sales" {
				return &b.Connections[i]
			}
		}
		return nil
	}

	t.Run("toggle on keeps the address in the clear", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "clear.muara")
		res, err := service.exportLocked(ExportRequest{All: true, IncludeServerDetails: true}, out)
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		if res.Sealed {
			t.Fatal("no passphrase was given, nothing should be sealed")
		}
		b := openAt(t, out)
		if b.Sealed != nil {
			t.Fatal("unexpected secrets.enc on a plain export")
		}
		if got := salesByAlias(b).Address; got == nil || got.Host != "db.internal" {
			t.Fatalf("sales address = %+v, want db.internal", got)
		}
	})

	t.Run("toggle off drops every address block", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "stripped.muara")
		if _, err := service.exportLocked(ExportRequest{All: true, IncludeServerDetails: false}, out); err != nil {
			t.Fatalf("export: %v", err)
		}
		b := openAt(t, out)
		for _, c := range b.Connections {
			if c.Address != nil || c.SSH != nil {
				t.Fatalf("connection %q kept a tier-2 block: %+v", c.Alias, c)
			}
		}
	})

	t.Run("passphrase seals the address blocks", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "sealed.muara")
		res, err := service.exportLocked(ExportRequest{All: true, IncludeServerDetails: true, Passphrase: "rahasia-sekali"}, out)
		if err != nil {
			t.Fatalf("export: %v", err)
		}
		if !res.Sealed {
			t.Fatal("passphrase was given, the result must say sealed")
		}
		b := openAt(t, out)
		if b.Sealed == nil {
			t.Fatal("sealed export carries no secrets.enc block")
		}
		// The addresses travel only inside the block, never twice.
		for _, c := range b.Connections {
			if c.Address != nil || c.SSH != nil {
				t.Fatalf("connection %q still carries a clear-text tier-2 block", c.Alias)
			}
		}
		plaintext, err := b.Unseal("rahasia-sekali")
		if err != nil {
			t.Fatalf("unseal: %v", err)
		}
		if !strings.Contains(string(plaintext), "db.internal") || !strings.Contains(string(plaintext), `"alias":"sales"`) {
			t.Fatalf("sealed plaintext = %s, want the sales address block", plaintext)
		}
	})

	t.Run("passphrase with nothing to seal is refused", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "nothing.muara")
		if _, err := service.exportLocked(ExportRequest{All: true, IncludeServerDetails: false, Passphrase: "x"}, out); err == nil ||
			!strings.Contains(err.Error(), "nothing to seal") {
			t.Fatalf("toggle-off + passphrase err = %v, want the nothing-to-seal refusal", err)
		}
		if _, err := service.exportLocked(ExportRequest{ViewID: "v-rev", Passphrase: "x"}, out); err == nil ||
			!strings.Contains(err.Error(), "nothing to seal") {
			t.Fatalf("file-only scope + passphrase err = %v, want the nothing-to-seal refusal", err)
		}
		if _, err := os.Stat(out); !os.IsNotExist(err) {
			t.Fatalf("a refused export wrote a file at %s", out)
		}
	})
}

func TestExportRefusesOverwritingTheOpenDocument(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	doc := exportFixture(t, t.TempDir())
	if _, err := service.Open(doc); err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := service.exportLocked(ExportRequest{All: true}, doc); err == nil ||
		!strings.Contains(err.Error(), "open file itself") {
		t.Fatalf("overwrite err = %v, want the open-file refusal", err)
	}
	if b := openAt(t, doc); len(b.Views) != 2 {
		t.Fatalf("the open document lost content: %d views", len(b.Views))
	}

	empty, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer empty.Close()
	if _, err := empty.exportLocked(ExportRequest{All: true}, filepath.Join(t.TempDir(), "x.muara")); err == nil ||
		!strings.Contains(err.Error(), "no bundle is open") {
		t.Fatalf("nil-doc export err = %v, want the no-bundle refusal", err)
	}
}

func TestCatalogListsScopeOptions(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	cat, err := service.Catalog()
	if err != nil || len(cat.Views) != 0 || len(cat.Projects) != 0 {
		t.Fatalf("empty catalog = %+v, %v", cat, err)
	}

	if _, err := service.Open(exportFixture(t, t.TempDir())); err != nil {
		t.Fatalf("open: %v", err)
	}
	cat, err = service.Catalog()
	if err != nil {
		t.Fatalf("catalog: %v", err)
	}
	if len(cat.Views) != 2 || cat.Views[0].Name != "Revenue" || cat.Views[1].Name != "Sales" {
		t.Fatalf("views = %+v", cat.Views)
	}
	if len(cat.Folders) != 1 || cat.Folders[0] != "laporan" {
		t.Fatalf("folders = %+v", cat.Folders)
	}
	if len(cat.Dashboards) != 1 || cat.Dashboards[0].Name != "Ringkasan" {
		t.Fatalf("dashboards = %+v", cat.Dashboards)
	}
	if len(cat.Projects) != 2 || cat.Projects[0].Name != "Laporan" || cat.Projects[1].Name != "Operasional" {
		t.Fatalf("projects = %+v", cat.Projects)
	}
}

func TestImportInspectCombinesStaticAndLive(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	incoming := checkBundle(t, t.TempDir())

	t.Run("against the open document", func(t *testing.T) {
		if _, err := service.Open(workBundle(t, t.TempDir())); err != nil {
			t.Fatalf("open: %v", err)
		}
		res, err := service.ImportInspect(incoming)
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if res.Name != "check.muara" {
			t.Fatalf("name = %q", res.Name)
		}
		if i, ok := issueByKind(res.Issues, "alias_reuse"); !ok || i.Alias != "orders" {
			t.Fatalf("no alias_reuse for orders in %+v", res.Issues)
		}
		if i, ok := issueByKind(res.Issues, "file_missing"); !ok || i.Alias != "gone" {
			t.Fatalf("no file_missing for gone in %+v", res.Issues)
		}
		wantLine(t, res.Lines, "view", "Revenue", "hijau", "2 kolom")
		wantLine(t, res.Lines, "view", "Ghost table", "merah", "")
		wantLine(t, res.Lines, "view", "Server", "kuning", "sales")
		wantLine(t, res.Lines, "semantic_model", "order_lines", "hijau", "")
		wantLine(t, res.Lines, "semantic_model", "bad_model", "merah", "")

		// Inspect writes nothing and must not disturb the document's session.
		info, err := service.Current()
		if err != nil || !info.Open || info.Views != 1 {
			t.Fatalf("document after inspect = %+v, %v", info, err)
		}
		if _, err := service.Run("SELECT COUNT(*) AS n FROM orders"); err != nil {
			t.Fatalf("document session broken by inspect: %v", err)
		}
	})

	t.Run("with no document open", func(t *testing.T) {
		empty, err := NewQueryService()
		if err != nil {
			t.Fatalf("new service: %v", err)
		}
		defer empty.Close()
		res, err := empty.ImportInspect(incoming)
		if err != nil {
			t.Fatalf("inspect: %v", err)
		}
		if i, ok := issueByKind(res.Issues, "alias_new"); !ok || i.Alias != "orders" {
			t.Fatalf("no alias_new for orders in %+v", res.Issues)
		}
		wantLine(t, res.Lines, "view", "Revenue", "hijau", "2 kolom")
	})
}

func TestImportApplyRefusesUntilDecisionsAreMadeThenApplies(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	if _, err := service.Open(workBundle(t, t.TempDir())); err != nil {
		t.Fatalf("open: %v", err)
	}
	incoming := conflictBundle(t, t.TempDir())

	res, err := service.ImportInspect(incoming)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if i, ok := issueByKind(res.Issues, "alias_conflict"); !ok || i.Level != "merah" {
		t.Fatalf("no merah alias_conflict in %+v", res.Issues)
	}

	// An empty plan leaves the conflict undecided: refused, and nothing changed.
	if _, err := service.ImportApply(incoming, bundle.ImportPlan{}); err == nil {
		t.Fatal("apply with an empty plan succeeded, want the conflict refusal")
	}
	info, err := service.Current()
	if err != nil || info.Views != 1 {
		t.Fatalf("document after refused apply = %+v, %v", info, err)
	}
	if _, err := service.Run("SELECT COUNT(*) AS n FROM orders"); err != nil {
		t.Fatalf("document session broken by refused apply: %v", err)
	}

	// The decision resolves it: merge, atomic save, reopen.
	info, err = service.ImportApply(incoming, bundle.ImportPlan{
		ForceReuseAliases: map[string]bool{"orders": true},
	})
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if info.Views != 2 {
		t.Fatalf("views after apply = %d, want 2", info.Views)
	}
	if b := openAt(t, info.Path); len(b.Views) != 2 {
		t.Fatalf("saved document has %d views, want 2", len(b.Views))
	}
	if _, err := service.Run("SELECT COUNT(*) AS n FROM orders"); err != nil {
		t.Fatalf("document session broken by apply: %v", err)
	}

	// Applying the same file again is a no-op: the identical View is skipped.
	info, err = service.ImportApply(incoming, bundle.ImportPlan{
		ForceReuseAliases: map[string]bool{"orders": true},
	})
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if info.Views != 2 {
		t.Fatalf("views after second apply = %d, want 2 (idempotent)", info.Views)
	}
}

func TestImportApplyRefusesWithoutAnOpenOrSealedDocument(t *testing.T) {
	incoming := conflictBundle(t, t.TempDir())
	plan := bundle.ImportPlan{ForceReuseAliases: map[string]bool{"orders": true}}

	empty, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer empty.Close()
	if _, err := empty.ImportApply(incoming, plan); err == nil ||
		!strings.Contains(err.Error(), "no bundle is open") {
		t.Fatalf("nil-doc apply err = %v, want the no-bundle refusal", err)
	}

	// A sealed document refuses before any merge: unseal-then-merge is M6.
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()
	folder := t.TempDir()
	sealed := filepath.Join(folder, "sealed.muara")
	b := &bundle.Bundle{
		Connections: []bundle.Connection{
			{ID: "c-orders", Alias: "orders", Driver: bundle.DriverFile,
				File: &bundle.FileSource{Path: copyFixture(t, "spike.csv", filepath.Join(folder, "data", "orders.csv"))}},
		},
	}
	if err := bundle.SaveWithSecrets(sealed, b, []byte(`{"note":"tier-2"} `), "pw-sealed"); err != nil {
		t.Fatalf("save sealed fixture: %v", err)
	}
	if _, err := service.Open(sealed); err != nil {
		t.Fatalf("open sealed: %v", err)
	}
	if _, err := service.ImportApply(incoming, plan); err == nil ||
		!strings.Contains(err.Error(), "sealed") {
		t.Fatalf("sealed-doc apply err = %v, want the sealed refusal", err)
	}
}
