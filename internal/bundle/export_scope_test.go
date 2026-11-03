package bundle

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var scopeStamp = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

// exportFixture is one of everything FR-10's scopes have to tell apart: two
// projects that share a View, a model that builds on another model, and a
// folder boundary. What an export carries and what it leaves behind are both
// asserted — carrying is the feature, leaving behind is the privacy half.
func exportFixture() *Bundle {
	return &Bundle{
		Manifest: Manifest{CreatedAt: scopeStamp, ExportedBy: "zakmursydan"},
		Projects: []Project{
			{ID: "p-1", Name: "Retail ID", Color: "#1B8A6B", DashboardRefs: []string{"d-1"}, ViewPins: []string{"v-1"}},
			{ID: "p-2", Name: "Klien Lain", Color: "#2C3E50", DashboardRefs: []string{"d-2"}, ViewPins: []string{"v-2"}},
		},
		Connections: []Connection{
			{ID: "c-1", Alias: "sales", Driver: DriverPostgres, Database: "dwh",
				Address: &Address{Host: "db.internal", Port: 5432, User: "analyst"}},
			{ID: "c-2", Alias: "orders", Driver: DriverFile, File: &FileSource{Path: "data/orders.csv"}},
			{ID: "c-3", Alias: "events", Driver: DriverFile, File: &FileSource{Path: "data/events.json"}},
		},
		SemanticModels: []SemanticModel{
			{ID: "sm-1", Name: "order_lines", RawSQL: "SELECT id, total FROM orders", Aliases: []string{"orders"}},
			{ID: "sm-2", Name: "retail", RawSQL: "SELECT SUM(total) AS total FROM order_lines", Aliases: []string{"orders"}},
			{ID: "sm-3", Name: "event_log", RawSQL: "SELECT * FROM events", Aliases: []string{"events"}},
		},
		Views: []View{
			{ID: "v-1", Name: "Revenue table", Folder: "weekly", Kind: KindTable, QueryMode: QuerySynced,
				BuilderState: json.RawMessage(`{"source":"order_lines"}`),
				Aliases:      []string{"orders"}, ModelRefs: []string{"sm-1"}},
			{ID: "v-2", Name: "Retail KPI", Folder: "weekly", Kind: KindKPI, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT total FROM retail", Aliases: []string{"orders"}},
			{ID: "v-3", Name: "Event counts", Folder: "monthly", Kind: KindBar, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT kind, COUNT(*) AS n FROM event_log GROUP BY 1", Aliases: []string{"events"}},
		},
		Dashboards: []Dashboard{
			{ID: "d-1", Name: "Monday pack", ViewRefs: []string{"v-1", "v-2"}},
			{ID: "d-2", Name: "Event board", ViewRefs: []string{"v-3"}},
		},
		Alerts: []Alert{{ID: "a-1", ViewID: "v-2", Channel: "telegram", Schedule: "07:00"}},
	}
}

func ids[T any](of func(T) string, list []T) map[string]bool {
	out := map[string]bool{}
	for _, item := range list {
		out[of(item)] = true
	}
	return out
}

func viewIDs(list []View) map[string]bool    { return ids(func(v View) string { return v.ID }, list) }
func modelIDs(list []SemanticModel) map[string]bool {
	return ids(func(m SemanticModel) string { return m.ID }, list)
}
func connAliases(list []Connection) map[string]bool {
	return ids(func(c Connection) string { return c.Alias }, list)
}
func dashIDs(list []Dashboard) map[string]bool {
	return ids(func(d Dashboard) string { return d.ID }, list)
}
func projectIDs(list []Project) map[string]bool {
	return ids(func(p Project) string { return p.ID }, list)
}

func wantIDs(want ...string) map[string]bool {
	out := map[string]bool{}
	for _, w := range want {
		out[w] = true
	}
	return out
}

func assertIDSet(t *testing.T, what string, got, want map[string]bool) {
	t.Helper()
	for id := range want {
		if !got[id] {
			t.Errorf("%s: %q missing from the export", what, id)
		}
	}
	for id := range got {
		if !want[id] {
			t.Errorf("%s: %q exported but not wanted", what, id)
		}
	}
}

// An exported subset has to be a bundle its own right: it saves (which runs
// the strict validation) and reopens whole.
func mustRoundTrip(t *testing.T, b *Bundle) *Bundle {
	t.Helper()
	path := filepath.Join(t.TempDir(), "out.muara")
	if err := Save(path, b); err != nil {
		t.Fatalf("the exported subset does not validate: %v", err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen the exported subset: %v", err)
	}
	return reopened
}

func TestExportAllCarriesEverythingButTheSealedBlock(t *testing.T) {
	src := exportFixture()
	src.Sealed = []byte{0xde, 0xad}

	out, err := src.Export(Scope{All: true})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	assertIDSet(t, "projects", projectIDs(out.Projects), wantIDs("p-1", "p-2"))
	assertIDSet(t, "connections", connAliases(out.Connections), wantIDs("sales", "orders", "events"))
	assertIDSet(t, "models", modelIDs(out.SemanticModels), wantIDs("sm-1", "sm-2", "sm-3"))
	assertIDSet(t, "views", viewIDs(out.Views), wantIDs("v-1", "v-2", "v-3"))
	assertIDSet(t, "dashboards", dashIDs(out.Dashboards), wantIDs("d-1", "d-2"))
	if len(out.Alerts) != 1 {
		t.Errorf("alerts = %d, want the one alert carried by All", len(out.Alerts))
	}
	// The sealed block is bound to the source file's passphrase; an export makes
	// its own credential decision instead of inheriting one.
	if out.Sealed != nil {
		t.Error("export carries the sealed block — the passphrase decision must be the export dialog's")
	}
	mustRoundTrip(t, out)
}

// A View scope is the definition export: one View, the models it needs — even
// the one its SQL only reaches through another model — and nothing that merely
// displays it.
func TestExportViewScopeResolvesModelsTransitively(t *testing.T) {
	src := exportFixture()
	out, err := src.Export(Scope{ViewID: "v-2"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	assertIDSet(t, "views", viewIDs(out.Views), wantIDs("v-2"))
	// v-2's SQL names retail, retail's SQL names order_lines: both must travel
	// or the View cannot answer at the destination.
	assertIDSet(t, "models", modelIDs(out.SemanticModels), wantIDs("sm-2", "sm-1"))
	assertIDSet(t, "connections", connAliases(out.Connections), wantIDs("orders"))
	if len(out.Dashboards) != 0 || len(out.Projects) != 0 || len(out.Alerts) != 0 {
		t.Errorf("view scope dragged layout along: dashboards %d projects %d alerts %d",
			len(out.Dashboards), len(out.Projects), len(out.Alerts))
	}
	mustRoundTrip(t, out)
}

// The stored model_refs is the trace that does not depend on parsing anything:
// a SYNCED View whose builder_state never spells the model still carries it.
func TestExportViewScopeFollowsStoredModelRefs(t *testing.T) {
	src := exportFixture()
	out, err := src.Export(Scope{ViewID: "v-1"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	assertIDSet(t, "views", viewIDs(out.Views), wantIDs("v-1"))
	assertIDSet(t, "models", modelIDs(out.SemanticModels), wantIDs("sm-1"))
	assertIDSet(t, "connections", connAliases(out.Connections), wantIDs("orders"))
	mustRoundTrip(t, out)
}

// BRD §5.2: a project references Views, it does not own them. Exporting p-1
// carries the View its dashboard shows even though only p-2 pins it — and does
// not carry p-2 itself, which is the privacy half of the rule.
func TestExportProjectScopeReferencesNotOwnership(t *testing.T) {
	src := exportFixture()
	out, err := src.Export(Scope{ProjectID: "p-1"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	assertIDSet(t, "projects", projectIDs(out.Projects), wantIDs("p-1"))
	assertIDSet(t, "dashboards", dashIDs(out.Dashboards), wantIDs("d-1"))
	assertIDSet(t, "views", viewIDs(out.Views), wantIDs("v-1", "v-2"))
	assertIDSet(t, "models", modelIDs(out.SemanticModels), wantIDs("sm-1", "sm-2"))
	assertIDSet(t, "connections", connAliases(out.Connections), wantIDs("orders"))
	mustRoundTrip(t, out)
}

func TestExportFolderScopeCarriesOnlyThatFolder(t *testing.T) {
	src := exportFixture()
	out, err := src.Export(Scope{Folder: "weekly"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	assertIDSet(t, "views", viewIDs(out.Views), wantIDs("v-1", "v-2"))
	assertIDSet(t, "models", modelIDs(out.SemanticModels), wantIDs("sm-1", "sm-2"))
	assertIDSet(t, "connections", connAliases(out.Connections), wantIDs("orders"))
	mustRoundTrip(t, out)
}

func TestExportDashboardScopeCarriesItsViews(t *testing.T) {
	src := exportFixture()
	out, err := src.Export(Scope{DashboardID: "d-2"})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	assertIDSet(t, "dashboards", dashIDs(out.Dashboards), wantIDs("d-2"))
	assertIDSet(t, "views", viewIDs(out.Views), wantIDs("v-3"))
	assertIDSet(t, "models", modelIDs(out.SemanticModels), wantIDs("sm-3"))
	assertIDSet(t, "connections", connAliases(out.Connections), wantIDs("events"))
	if len(out.Projects) != 0 {
		t.Errorf("dashboard scope carried %d projects", len(out.Projects))
	}
	mustRoundTrip(t, out)
}

func TestExportRefusesWhatItCannotSelect(t *testing.T) {
	src := exportFixture()
	cases := []struct {
		name  string
		scope Scope
		want  string
	}{
		{"zero scope", Scope{}, "exactly one"},
		{"two options", Scope{All: true, ViewID: "v-1"}, "exactly one"},
		{"unknown view", Scope{ViewID: "nope"}, `view "nope"`},
		{"unknown project", Scope{ProjectID: "nope"}, `project "nope"`},
		{"unknown dashboard", Scope{DashboardID: "nope"}, `dashboard "nope"`},
		{"empty folder", Scope{Folder: "nope"}, `folder "nope"`},
	}
	for _, tc := range cases {
		if _, err := src.Export(tc.scope); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want it to say %q", tc.name, err, tc.want)
		}
	}
}

// Exporting a file with a dangling reference would hand the destination a
// definition that cannot work; the strict write-side validation refuses it
// with the reason instead of guessing a subset.
func TestExportRefusesAnInconsistentSource(t *testing.T) {
	src := exportFixture()
	src.Views[0].Aliases = append(src.Views[0].Aliases, "ghost")
	_, err := src.Export(Scope{ViewID: "v-1"})
	if err == nil || !strings.Contains(err.Error(), `alias "ghost"`) {
		t.Errorf("err = %v, want it to name the dangling alias", err)
	}
}

func TestSaveRefusesAViewModelRefThatIsNotInTheBundle(t *testing.T) {
	src := exportFixture()
	src.Views[0].ModelRefs = []string{"ghost"}
	err := Save(filepath.Join(t.TempDir(), "broken.muara"), src)
	if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), `semantic model "ghost"`) {
		t.Errorf("err = %v, want ErrInvalid naming the missing model", err)
	}
}

func TestSQLIdentifiersSkipsLiteralsAndComments(t *testing.T) {
	cases := []struct {
		sql  string
		want []string
	}{
		{"SELECT channel FROM orders", []string{"SELECT", "channel", "FROM", "orders"}},
		// A string literal that happens to spell a model name is not a reference.
		{"SELECT 'order_lines' AS label", []string{"SELECT", "AS", "label"}},
		{"SELECT 'it''s fine' AS t", []string{"SELECT", "AS", "t"}},
		{"-- order_lines is not read here\nSELECT 1", []string{"SELECT"}},
		{"/* retail */ SELECT 1", []string{"SELECT"}},
		{`SELECT "retail"."total" FROM "retail"`, []string{"SELECT", "retail", "total", "FROM", "retail"}},
		{`SELECT "a""b" FROM x`, []string{"SELECT", `a"b`, "FROM", "x"}},
		{"SELECT * FROM events_json", []string{"SELECT", "FROM", "events_json"}},
	}
	for _, tc := range cases {
		got := sqlIdentifiers(tc.sql)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("sqlIdentifiers(%q) = %v, want %v", tc.sql, got, tc.want)
		}
	}
}
