package bundle

import (
	"errors"
	"strings"
	"testing"
)

// These fixtures are the static half of BRD §13: PC A's file meets a document
// that already has some of the same names — a reused alias, a conflicting one,
// an identical View, a changed one, a new one, and one that reads an alias
// nobody defines. What the pre-flight reports and what the merge refuses are
// both asserted; the live schema check is the app layer's job (M4d).

func localViewV1() View {
	return View{
		ID: "v-l", Name: "Revenue", Kind: KindTable, QueryMode: QueryCustomSQL,
		RawSQL:   "SELECT channel, SUM(total) FROM order_lines GROUP BY 1",
		Aliases:  []string{"orders"},
		ModelRefs: []string{"sm-l"},
	}
}

func localDoc() *Bundle {
	return &Bundle{
		Manifest: Manifest{CreatedAt: scopeStamp, ExportedBy: "zakmursydan"},
		Projects: []Project{{
			ID: "p-l", Name: "Lokal", Color: "#1B8A6B",
			DashboardRefs: []string{"d-l"}, ViewPins: []string{"v-l"},
		}},
		Connections: []Connection{
			{ID: "c-orders", Alias: "orders", Driver: DriverFile, File: &FileSource{Path: "data/orders.csv"}},
			{ID: "c-sales", Alias: "sales", Driver: DriverPostgres, Database: "dwh",
				Address: &Address{Host: "db.local", Port: 5432, User: "analyst"}},
			{ID: "c-wh", Alias: "wh", Driver: DriverSQLite, File: &FileSource{Path: "data/warehouse.sqlite"}},
		},
		SemanticModels: []SemanticModel{
			{ID: "sm-l", Name: "order_lines", RawSQL: "SELECT id, total FROM orders", Aliases: []string{"orders"}},
		},
		Views: []View{
			localViewV1(),
			{ID: "v-l2", Name: "Second", Kind: KindTable, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT 1 AS n", Aliases: []string{"orders"}},
		},
		Dashboards: []Dashboard{{ID: "d-l", Name: "Lokal board", ViewRefs: []string{"v-l"}}},
	}
}

func incomingFile() *Bundle {
	return &Bundle{
		Manifest: Manifest{CreatedAt: scopeStamp, ExportedBy: "rekan"},
		Projects: []Project{{
			ID: "p-in", Name: "Incoming", Color: "#2C3E50", ViewPins: []string{"v-new"},
		}},
		Connections: []Connection{
			{ID: "c-orders2", Alias: "orders", Driver: DriverFile, File: &FileSource{Path: "data/incoming.csv"}},
			{ID: "c-sales2", Alias: "sales", Driver: DriverPostgres, Database: "dwh",
				Address: &Address{Host: "other.internal", Port: 5432, User: "rekan"}},
			{ID: "c-wh2", Alias: "wh", Driver: DriverMySQL, Database: "dwh2"},
			{ID: "c-targets", Alias: "targets", Driver: DriverFile, File: &FileSource{Path: "data/events.json"}},
		},
		SemanticModels: []SemanticModel{
			{ID: "sm-l", Name: "order_lines", RawSQL: "SELECT id, total FROM orders", Aliases: []string{"orders"}},
			{ID: "sm-targets", Name: "targets_model", RawSQL: "SELECT * FROM targets", Aliases: []string{"targets"}},
		},
		Views: []View{
			localViewV1(),
			{ID: "v-l2", Name: "Second", Kind: KindKPI, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT 2 AS n", Aliases: []string{"orders"}},
			{ID: "v-new", Name: "Targets", Kind: KindBar, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT * FROM targets_model", Aliases: []string{"targets"},
				ModelRefs: []string{"sm-targets"}},
			{ID: "v-ghost", Name: "Ghost", Kind: KindTable, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT 1", Aliases: []string{"ghost"}},
			{ID: "v-wh", Name: "Warehouse", Kind: KindTable, QueryMode: QueryCustomSQL,
				RawSQL: "SELECT 1", Aliases: []string{"wh"}},
		},
		Dashboards: []Dashboard{{ID: "d-in", Name: "Incoming board", ViewRefs: []string{"v-new"}}},
		Alerts:     []Alert{{ID: "a-in", ViewID: "v-new", Channel: "telegram", Schedule: "07:00"}},
	}
}

func issuesByKind(report *ImportReport, kind string) []ImportIssue {
	var out []ImportIssue
	for _, issue := range report.Issues {
		if issue.Kind == kind {
			out = append(out, issue)
		}
	}
	return out
}

func assertIssue(t *testing.T, report *ImportReport, kind, level string, atLeast int) {
	t.Helper()
	got := issuesByKind(report, kind)
	if len(got) < atLeast {
		t.Fatalf("%s issues = %d, want at least %d (report: %+v)", kind, len(got), atLeast, report.Issues)
	}
	for _, issue := range got {
		if issue.Level != level {
			t.Errorf("%s issue %q has level %q, want %q", kind, issue.Title, issue.Level, level)
		}
	}
}

// The anchor folder exists but holds none of the file the incoming bundle
// names, so the file check measures what §13 step 3 promises: the pre-flight
// names the path that needs re-pointing.
func TestPreflightReportsEveryDecisionTheImportNeeds(t *testing.T) {
	local := localDoc()
	incoming := incomingFile()
	anchor := t.TempDir()

	report, err := local.Preflight(incoming, anchor)
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}

	assertIssue(t, report, "alias_reuse", "hijau", 2)      // orders + sales
	assertIssue(t, report, "alias_conflict", "merah", 1)   // wh
	assertIssue(t, report, "file_missing", "kuning", 1)    // targets
	assertIssue(t, report, "duplicate_same", "hijau", 2)   // v-l + sm-l
	assertIssue(t, report, "duplicate_diff", "merah", 1)   // v-l2
	assertIssue(t, report, "alias_missing", "merah", 1)    // v-ghost
	assertIssue(t, report, "driver_diff", "kuning", 1)     // v-wh

	conflict := issuesByKind(report, "alias_conflict")[0]
	if conflict.Alias != "wh" || !strings.Contains(conflict.Title, "mysql") || !strings.Contains(conflict.Title, "sqlite") {
		t.Errorf("wh conflict = %+v, want it to name both drivers", conflict)
	}
	missing := issuesByKind(report, "file_missing")[0]
	if missing.Alias != "targets" || !strings.Contains(missing.Path, "events.json") {
		t.Errorf("targets file issue = %+v, want the alias and the resolved path", missing)
	}
	aliasMissing := issuesByKind(report, "alias_missing")[0]
	if aliasMissing.ViewID != "v-ghost" || aliasMissing.Alias != "ghost" {
		t.Errorf("ghost issue = %+v, want view v-ghost and alias ghost", aliasMissing)
	}

	if report.ViewHashes["v-l"] == "" || report.ModelHashes["sm-l"] == "" {
		t.Errorf("hashes = views %v models %v, want an identity hash for every incoming entity",
			report.ViewHashes, report.ModelHashes)
	}
	// "Identical" means identical content, so the hash of the locally built
	// twin must match what the report computed for the incoming one.
	want, err := entityHash(localViewV1())
	if err != nil {
		t.Fatalf("hash local twin: %v", err)
	}
	if report.ViewHashes["v-l"] != want {
		t.Errorf("v-l hash = %s, want the same content to hash the same on both sides", report.ViewHashes["v-l"])
	}
}

func TestPreflightReportsASealedBlockItWillNotImport(t *testing.T) {
	local := localDoc()
	incoming := incomingFile()
	incoming.Sealed = []byte{0xde, 0xad}

	report, err := local.Preflight(incoming, t.TempDir())
	if err != nil {
		t.Fatalf("preflight: %v", err)
	}
	assertIssue(t, report, "sealed_ignored", "kuning", 1)
}

// FR-11's "tidak ada import sebagian" is structural: an unresolved conflict, an
// undecided duplicate, and a missing alias each refuse the whole merge, and
// the error names all three — nothing is written because there is nothing to
// save.
func TestMergeRefusesUntilEveryRedIssueIsDecided(t *testing.T) {
	local := localDoc()
	_, err := local.Merge(incomingFile(), ImportPlan{})
	if err == nil {
		t.Fatal("merge with an empty plan succeeded — every merah issue must be decided first")
	}
	for _, want := range []string{`alias "wh"`, `view "v-l2"`, `alias "ghost"`} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("merge error does not name %s: %v", want, err)
		}
	}
}

func resolvedPlan() ImportPlan {
	return ImportPlan{
		ForceReuseAliases: map[string]bool{"wh": true},
		OverwriteViews:    map[string]bool{"v-l2": true},
		SkipViews:         map[string]bool{"v-ghost": true},
		NewPaths:          map[string]string{"targets": "/data/baru/events.json"},
	}
}

func TestMergeResolvesEveryDecisionAndMutatesNothing(t *testing.T) {
	local := localDoc()
	localBefore := localDoc()

	merged, err := local.Merge(incomingFile(), resolvedPlan())
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	if len(merged.Connections) != 4 {
		t.Errorf("connections = %d, want 3 local + targets", len(merged.Connections))
	}
	var targets *Connection
	for i := range merged.Connections {
		if merged.Connections[i].Alias == "targets" {
			targets = &merged.Connections[i]
		}
		if merged.Connections[i].Alias == "wh" && merged.Connections[i].Driver != DriverSQLite {
			t.Error("wh was replaced by the incoming mysql — 'pakai yang ada' keeps the local one")
		}
	}
	if targets == nil || targets.File == nil || targets.File.Path != "/data/baru/events.json" {
		t.Errorf("targets = %+v, want the new path this machine picked", targets)
	}

	if len(merged.SemanticModels) != 2 || len(merged.Views) != 4 {
		t.Fatalf("models = %d views = %d, want 2 and 4", len(merged.SemanticModels), len(merged.Views))
	}
	var replaced, ghostSeen bool
	for _, v := range merged.Views {
		switch v.ID {
		case "v-l2":
			replaced = true
			if v.RawSQL != "SELECT 2 AS n" || v.Kind != KindKPI {
				t.Errorf("v-l2 = %+v, want the incoming content after OverwriteViews", v)
			}
		case "v-ghost":
			ghostSeen = true
		}
	}
	if !replaced || ghostSeen {
		t.Errorf("views after merge: replaced=%v ghostSeen=%v", replaced, ghostSeen)
	}
	if len(merged.Projects) != 2 || len(merged.Dashboards) != 2 || len(merged.Alerts) != 1 {
		t.Errorf("projects %d dashboards %d alerts %d, want 2, 2, 1",
			len(merged.Projects), len(merged.Dashboards), len(merged.Alerts))
	}

	mustRoundTrip(t, merged)

	// The merge works on copies: the document it was called on is byte-for-byte
	// the caller's business, not the import's scratch pad.
	if local.Views[1].RawSQL != "SELECT 1 AS n" {
		t.Error("the local document's view was mutated by the merge")
	}
	if len(local.Connections) != len(localBefore.Connections) || len(local.Views) != len(localBefore.Views) {
		t.Error("the local document grew during the merge")
	}
}

// §13 step 8: importing the same file twice leaves one View, one project, one
// alert — the second pass recognizes everything as "already here, identical".
func TestMergeSecondImportIsIdempotent(t *testing.T) {
	local := localDoc()
	plan := resolvedPlan()

	first, err := local.Merge(incomingFile(), plan)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, err := first.Merge(incomingFile(), plan)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}

	if len(second.Views) != len(first.Views) || len(second.Connections) != len(first.Connections) {
		t.Errorf("second merge changed the counts: views %d→%d connections %d→%d",
			len(first.Views), len(second.Views), len(first.Connections), len(second.Connections))
	}
	if len(second.Projects) != 2 || len(second.Alerts) != 1 || len(second.Dashboards) != 2 {
		t.Errorf("second merge stacked entities: projects %d dashboards %d alerts %d",
			len(second.Projects), len(second.Dashboards), len(second.Alerts))
	}
	mustRoundTrip(t, second)
}

// A skipped View is the user's "this View must not come in" — its pins,
// dashboard refs and alerts must not dangle in the merged file.
func TestMergeSkippingAViewDropsItsReferences(t *testing.T) {
	local := localDoc()
	plan := resolvedPlan()
	plan.SkipViews["v-new"] = true

	merged, err := local.Merge(incomingFile(), plan)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	for _, p := range merged.Projects {
		if p.ID == "p-in" && len(p.ViewPins) != 0 {
			t.Errorf("p-in pins = %v, want none after its View was skipped", p.ViewPins)
		}
	}
	for _, d := range merged.Dashboards {
		if d.ID == "d-in" && len(d.ViewRefs) != 0 {
			t.Errorf("d-in refs = %v, want none after its View was skipped", d.ViewRefs)
		}
	}
	if len(merged.Alerts) != 0 {
		t.Errorf("alerts = %d, want the watcher of a skipped View dropped", len(merged.Alerts))
	}
	mustRoundTrip(t, merged)
}

// Skipping a Model a kept View references would leave the View querying a name
// nothing mounts — that is refused, not silently produced.
func TestMergeRefusesASkippedModelAKeptViewReferences(t *testing.T) {
	local := localDoc()
	plan := resolvedPlan()
	plan.SkipModels = map[string]bool{"sm-targets": true}

	_, err := local.Merge(incomingFile(), plan)
	if err == nil || !strings.Contains(err.Error(), `semantic model "sm-targets" which is skipped`) {
		t.Errorf("err = %v, want it to name the skipped model a kept view refs", err)
	}
}

// FR-11's "tanyakan proyek mana yang boleh mempin-nya" is the plan's PinViews:
// the caller says which local projects may pin which new View.
func TestMergePinsNewViewsWhereTheUserAllows(t *testing.T) {
	local := localDoc()
	plan := resolvedPlan()
	plan.PinViews = map[string][]string{"v-new": {"p-l"}}

	merged, err := local.Merge(incomingFile(), plan)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	for _, p := range merged.Projects {
		if p.ID == "p-l" && !containsString(p.ViewPins, "v-new") {
			t.Errorf("p-l pins = %v, want the user-approved pin of v-new", p.ViewPins)
		}
	}
	mustRoundTrip(t, merged)

	// An approval for a project that is not in the document is refused, not
	// ignored.
	bad := resolvedPlan()
	bad.PinViews = map[string][]string{"v-new": {"p-unknown"}}
	if _, err := local.Merge(incomingFile(), bad); err == nil {
		t.Error("PinViews naming an unknown project succeeded")
	}
}

// The whole merge must produce a file the app can save: a dangling reference
// anywhere fails the strict validation and therefore the import.
func TestMergeValidatesItsOwnResult(t *testing.T) {
	local := localDoc()
	incoming := incomingFile()
	// v-wh keeps its alias wh (reused), but a ModelRefs entry pointing into
	// the void would pass every earlier check — only the final validation
	// catches it, and it must.
	incoming.Views = append(incoming.Views, View{
		ID: "v-dangle", Name: "Dangle", Kind: KindTable, QueryMode: QueryCustomSQL,
		RawSQL: "SELECT 1", Aliases: []string{"orders"}, ModelRefs: []string{"sm-nope"},
	})
	_, err := local.Merge(incoming, resolvedPlan())
	if err == nil || !strings.Contains(err.Error(), "would not save") {
		t.Errorf("err = %v, want the final validation refusal", err)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Errorf("err = %v, want ErrInvalid from the validation", err)
	}
}
