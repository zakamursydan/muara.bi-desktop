package bundle

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ImportIssue is one line of the pre-flight report (FR-11). Level is the color
// the UI renders: "merah" blocks the apply until the user decides, "kuning"
// warns, "hijau" confirms. Kind is the machine-readable word; Title is the
// sentence an analyst reads.
type ImportIssue struct {
	Level  string `json:"level"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`

	Alias   string `json:"alias,omitempty"`
	ViewID  string `json:"viewId,omitempty"`
	ModelID string `json:"modelId,omitempty"`
	Path    string `json:"path,omitempty"`
}

// ImportReport is everything the import dialog shows before one byte is
// written. It runs no queries: names, hashes and file existence only — the
// red/yellow schema check on tables and columns needs a mounted session and
// lives in the app layer.
type ImportReport struct {
	Issues []ImportIssue `json:"issues"`
	// ViewHashes and ModelHashes identify each incoming entity by its content,
	// so the dialog and the merge agree on what "the same" means even when the
	// two files were written by different app versions.
	ViewHashes  map[string]string `json:"viewHashes"`
	ModelHashes map[string]string `json:"modelHashes"`
}

// Preflight compares an incoming bundle against the document it will be
// imported into. anchor is the folder of the incoming .muara file on this
// machine — the anchor its relative source paths resolve against.
func (local *Bundle) Preflight(incoming *Bundle, anchor string) (*ImportReport, error) {
	report := &ImportReport{ViewHashes: map[string]string{}, ModelHashes: map[string]string{}}
	issues := &report.Issues

	localByAlias := map[string]Connection{}
	for _, c := range local.Connections {
		localByAlias[c.Alias] = c
	}
	incomingByAlias := map[string]Connection{}
	for _, c := range incoming.Connections {
		incomingByAlias[c.Alias] = c
	}

	// ResolveFileSources takes a bundle *path* and anchors on its folder, so a
	// made-up file name under the real folder is the honest way to say "resolve
	// against this anchor".
	resolved, err := incoming.ResolveFileSources(filepath.Join(anchor, "imported.muara"))
	if err != nil {
		return nil, err
	}

	if incoming.Sealed != nil {
		*issues = append(*issues, ImportIssue{
			Level: "kuning", Kind: "sealed_ignored",
			Title: "berkas ini membawa kredensial ber-passphrase — isinya belum diimpor",
			Detail: "kredensial hanya masuk lewat keychain mesin ini (M6); blok secrets.enc tidak ikut disimpan",
		})
	}

	for _, c := range incoming.Connections {
		localConn, exists := localByAlias[c.Alias]
		switch {
		case exists && localConn.Driver == c.Driver:
			*issues = append(*issues, ImportIssue{
				Level: "hijau", Kind: "alias_reuse", Alias: c.Alias,
				Title: fmt.Sprintf("alias %s sudah ada di dokumen ini (driver %s) — memakai yang ada", c.Alias, c.Driver),
				Detail: "alamat dan kredensial diambil dari mesin ini, bukan dari berkas",
			})
		case exists:
			*issues = append(*issues, ImportIssue{
				Level: "merah", Kind: "alias_conflict", Alias: c.Alias,
				Title: fmt.Sprintf("alias %s bentrok: berkas membawa driver %s, dokumen ini memakai %s",
					c.Alias, c.Driver, localConn.Driver),
				Detail: "pilih 'pakai yang ada' secara eksplisit, atau beri nama lain dulu di dokumen",
			})
		default:
			if c.File != nil {
				path := resolved[c.Alias]
				switch info, statErr := os.Stat(path); {
				case errors.Is(statErr, os.ErrNotExist):
					*issues = append(*issues, ImportIssue{
						Level: "kuning", Kind: "file_missing", Alias: c.Alias, Path: path,
						Title: fmt.Sprintf("alias %s menunjuk berkas yang tidak ada: %s", c.Alias, path),
						Detail: "tunjuk ulang berkasnya sebelum menerapkan, atau mount akan melaporkannya",
					})
				case statErr != nil:
					*issues = append(*issues, ImportIssue{
						Level: "kuning", Kind: "file_missing", Alias: c.Alias, Path: path,
						Title: fmt.Sprintf("berkas untuk alias %s tidak bisa dibaca: %v", c.Alias, statErr),
					})
				case info.IsDir():
					*issues = append(*issues, ImportIssue{
						Level: "kuning", Kind: "file_folder", Alias: c.Alias, Path: path,
						Title: fmt.Sprintf("alias %s menunjuk folder, bukan berkas: %s", c.Alias, path),
					})
				default:
					*issues = append(*issues, ImportIssue{
						Level: "hijau", Kind: "alias_new", Alias: c.Alias,
						Title: fmt.Sprintf("alias %s akan dibuat dari berkas %s", c.Alias, c.File.Path),
					})
				}
				continue
			}
			*issues = append(*issues, ImportIssue{
				Level: "hijau", Kind: "alias_new", Alias: c.Alias,
				Title: fmt.Sprintf("alias %s akan dibuat (driver %s)", c.Alias, c.Driver),
				Detail: "passwordnya diisi sekali lewat keychain mesin ini",
			})
		}
	}

	// The alias a view or model reads is "defined" when the file carries it or
	// this document already has it; anything else is the §13 variant-7 case:
	// the user creates the alias first, then the views work.
	defined := map[string]bool{}
	for alias := range localByAlias {
		defined[alias] = true
	}
	for alias := range incomingByAlias {
		defined[alias] = true
	}

	localViews := map[string]View{}
	for _, v := range local.Views {
		localViews[v.ID] = v
	}
	for _, v := range incoming.Views {
		hash, err := entityHash(v)
		if err != nil {
			return nil, err
		}
		report.ViewHashes[v.ID] = hash

		if localV, ok := localViews[v.ID]; ok {
			localHash, err := entityHash(localV)
			if err != nil {
				return nil, err
			}
			if hash == localHash {
				*issues = append(*issues, ImportIssue{
					Level: "hijau", Kind: "duplicate_same", ViewID: v.ID,
					Title: fmt.Sprintf("View %q sudah ada dan isinya identik — dilewati", v.Name),
				})
			} else {
				*issues = append(*issues, ImportIssue{
					Level: "merah", Kind: "duplicate_diff", ViewID: v.ID,
					Title: fmt.Sprintf("View %q sudah ada di dokumen ini tapi isinya berbeda", v.Name),
					Detail: "putuskan: lewati, atau timpa — menimpa diam-diam tidak pernah terjadi",
				})
			}
		}
		for _, alias := range v.Aliases {
			if !defined[alias] {
				*issues = append(*issues, ImportIssue{
					Level: "merah", Kind: "alias_missing", ViewID: v.ID, Alias: alias,
					Title: fmt.Sprintf("View %q membaca alias %q yang tidak didefinisikan berkas ini maupun dokumen ini",
						v.Name, alias),
					Detail: "buat aliasnya dulu (Settings › Profil), lalu terapkan",
				})
				continue
			}
			if in, ok := incomingByAlias[alias]; ok {
				if lc, ok := localByAlias[alias]; ok && lc.Driver != in.Driver {
					*issues = append(*issues, ImportIssue{
						Level: "kuning", Kind: "driver_diff", ViewID: v.ID, Alias: alias,
						Title: fmt.Sprintf("View %q membaca alias %q: ditulis untuk driver %s, di dokumen ini %s",
							v.Name, alias, in.Driver, lc.Driver),
						Detail: "SQL mungkin tidak jalan terhadap driver yang berbeda",
					})
				}
			}
		}
	}

	localModels := map[string]SemanticModel{}
	for _, m := range local.SemanticModels {
		localModels[m.ID] = m
	}
	for _, m := range incoming.SemanticModels {
		hash, err := entityHash(m)
		if err != nil {
			return nil, err
		}
		report.ModelHashes[m.ID] = hash

		if localM, ok := localModels[m.ID]; ok {
			localHash, err := entityHash(localM)
			if err != nil {
				return nil, err
			}
			if hash == localHash {
				*issues = append(*issues, ImportIssue{
					Level: "hijau", Kind: "duplicate_same", ModelID: m.ID,
					Title: fmt.Sprintf("Semantic model %q sudah ada dan isinya identik — dilewati", m.Name),
				})
			} else {
				*issues = append(*issues, ImportIssue{
					Level: "merah", Kind: "duplicate_diff", ModelID: m.ID,
					Title: fmt.Sprintf("Semantic model %q sudah ada di dokumen ini tapi isinya berbeda", m.Name),
					Detail: "putuskan: lewati, atau timpa",
				})
			}
		}
		for _, alias := range m.Aliases {
			if !defined[alias] {
				*issues = append(*issues, ImportIssue{
					Level: "merah", Kind: "alias_missing", ModelID: m.ID, Alias: alias,
					Title: fmt.Sprintf("Semantic model %q membaca alias %q yang tidak didefinisikan berkas ini maupun dokumen ini",
						m.Name, alias),
					Detail: "buat aliasnya dulu, lalu terapkan",
				})
			}
		}
	}

	return report, nil
}

// ImportPlan is the user's answer to what pre-flight surfaced. Every choice is
// explicit: a field left empty means "no", never "decide for me".
type ImportPlan struct {
	// SkipViews / SkipModels leave the incoming entity out. A skipped View
	// also loses its pins, dashboard refs and alerts — the skip is the user's
	// statement that the View must not come in, and a dangling reference would
	// make the merged file unsaveable. A skipped Model that a kept View still
	// references is refused instead: the View would silently query a name that
	// does not exist.
	SkipViews  map[string]bool `json:"skipViews,omitempty"`
	SkipModels map[string]bool `json:"skipModels,omitempty"`

	// OverwriteViews / OverwriteModels resolve duplicate_diff: the incoming
	// entity replaces the local one. Without it the merge refuses — identical
	// entities are skipped automatically and never need this.
	OverwriteViews  map[string]bool `json:"overwriteViews,omitempty"`
	OverwriteModels map[string]bool `json:"overwriteModels,omitempty"`

	// ForceReuseAliases resolves alias_conflict: "pakai yang ada" said out
	// loud. The local connection keeps the alias; the incoming one is dropped.
	ForceReuseAliases map[string]bool `json:"forceReuseAliases,omitempty"`

	// NewPaths points an incoming file/sqlite connection at a file on this
	// machine (§13 step 4). Only read for aliases that do not exist locally —
	// a reused alias keeps the local file.
	NewPaths map[string]string `json:"newPaths,omitempty"`

	// PinViews answers FR-11's "tanyakan proyek mana yang boleh mempin-nya":
	// incoming view ID → local project IDs that gain a pin.
	PinViews map[string][]string `json:"pinViews,omitempty"`
}

// Merge builds the document that results from importing incoming into local
// under the plan. It writes nothing and mutates nothing: the caller saves the
// result through Save (atomic), so "batal" is simply never calling this and
// there is no partial import by construction.
//
// The rules that make it safe:
//   - an alias that exists locally is kept, whatever the file carries — the
//     profile gives it this machine's address and keychain (§5.2);
//   - a conflict (same alias, different driver) refuses unless
//     ForceReuseAliases says otherwise, and a duplicate with different content
//     refuses unless Overwrite or Skip decides;
//   - a kept View may not reference a skipped Model, and every alias a kept
//     entity reads must be defined after the merge;
//   - the result must pass the strict write-side validation, so an import can
//     never produce a file the app itself would refuse to save.
func (local *Bundle) Merge(incoming *Bundle, plan ImportPlan) (*Bundle, error) {
	localByAlias := map[string]Connection{}
	for _, c := range local.Connections {
		localByAlias[c.Alias] = c
	}
	incomingByAlias := map[string]Connection{}
	for _, c := range incoming.Connections {
		incomingByAlias[c.Alias] = c
	}
	localViews := map[string]View{}
	for _, v := range local.Views {
		localViews[v.ID] = v
	}
	localModels := map[string]SemanticModel{}
	for _, m := range local.SemanticModels {
		localModels[m.ID] = m
	}

	var problems []error

	defined := map[string]bool{}
	for alias := range localByAlias {
		defined[alias] = true
	}
	for _, c := range incoming.Connections {
		defined[c.Alias] = true
	}

	for _, c := range incoming.Connections {
		if lc, ok := localByAlias[c.Alias]; ok && lc.Driver != c.Driver && !plan.ForceReuseAliases[c.Alias] {
			problems = append(problems, fmt.Errorf(
				"import: alias %q exists in this document as driver %q and the file carries %q — "+
					"say 'pakai yang ada' (ForceReuseAliases) or rename one of them first", c.Alias, lc.Driver, c.Driver))
		}
	}

	inViews := map[string]View{}
	for _, v := range incoming.Views {
		inViews[v.ID] = v
		if plan.SkipViews[v.ID] {
			continue
		}
		for _, alias := range v.Aliases {
			if !defined[alias] {
				problems = append(problems, fmt.Errorf(
					"import: view %q reads alias %q, defined neither in this document nor in the file — create the alias first",
					v.ID, alias))
			}
		}
		if lv, ok := localViews[v.ID]; ok && !plan.SkipViews[v.ID] {
			same, err := sameEntity(v, lv)
			if err != nil {
				return nil, err
			}
			if !same && !plan.OverwriteViews[v.ID] {
				problems = append(problems, fmt.Errorf(
					"import: view %q exists here with different content — decide: OverwriteViews or SkipViews", v.ID))
			}
		}
	}

	inModels := map[string]SemanticModel{}
	for _, m := range incoming.SemanticModels {
		inModels[m.ID] = m
		if plan.SkipModels[m.ID] {
			continue
		}
		for _, alias := range m.Aliases {
			if !defined[alias] {
				problems = append(problems, fmt.Errorf(
					"import: semantic model %q reads alias %q, defined neither in this document nor in the file",
					m.ID, alias))
			}
		}
		if lm, ok := localModels[m.ID]; ok && !plan.SkipModels[m.ID] {
			same, err := sameEntity(m, lm)
			if err != nil {
				return nil, err
			}
			if !same && !plan.OverwriteModels[m.ID] {
				problems = append(problems, fmt.Errorf(
					"import: semantic model %q exists here with different content — decide: OverwriteModels or SkipModels",
					m.ID))
			}
		}
	}

	// A kept View may not point at a model that was skipped: its query would
	// silently read a name nothing mounted.
	modelsKept := map[string]bool{}
	for _, m := range incoming.SemanticModels {
		if !plan.SkipModels[m.ID] {
			modelsKept[m.ID] = true
		}
	}
	for _, v := range incoming.Views {
		if plan.SkipViews[v.ID] {
			continue
		}
		for _, ref := range v.ModelRefs {
			if _, inFile := inModels[ref]; inFile && !modelsKept[ref] {
				problems = append(problems, fmt.Errorf(
					"import: view %q refs semantic model %q which is skipped — keep the model or skip the view", v.ID, ref))
			}
		}
	}

	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}

	out := *local
	out.Connections = cloneSlice(local.Connections)
	out.SemanticModels = cloneSlice(local.SemanticModels)
	out.Views = cloneSlice(local.Views)
	out.Projects = cloneSlice(local.Projects)
	out.Dashboards = cloneSlice(local.Dashboards)
	out.Alerts = cloneSlice(local.Alerts)
	out.Sealed = nil

	for _, c := range incoming.Connections {
		if _, exists := localByAlias[c.Alias]; exists {
			continue
		}
		if path, ok := plan.NewPaths[c.Alias]; ok && c.File != nil {
			c.File = &FileSource{Path: path}
		}
		out.Connections = append(out.Connections, c)
	}

	out.SemanticModels = mergeEntities(out.SemanticModels, incoming.SemanticModels,
		plan.SkipModels, plan.OverwriteModels)

	out.Views = mergeEntities(out.Views, incoming.Views, plan.SkipViews, plan.OverwriteViews)

	// Projects and dashboards travel whole (FR-11 does not offer per-project
	// decisions); an ID that exists locally keeps the local entity. References
	// to skipped Views are dropped — a skip means the View must not come in.
	skipped := plan.SkipViews
	out.Projects = mergeProjects(out.Projects, incoming.Projects, skipped)
	out.Dashboards = mergeDashboards(out.Dashboards, incoming.Dashboards, skipped)

	viewsInOut := map[string]bool{}
	for _, v := range out.Views {
		viewsInOut[v.ID] = true
	}
	projectsInOut := map[string]int{}
	for i, p := range out.Projects {
		projectsInOut[p.ID] = i
	}
	for id, pins := range plan.PinViews {
		if !viewsInOut[id] {
			problems = append(problems, fmt.Errorf("import: PinViews names view %q which is not in the merged document", id))
			continue
		}
		for _, projectID := range pins {
			idx, ok := projectsInOut[projectID]
			if !ok {
				problems = append(problems, fmt.Errorf("import: PinViews names project %q which is not in the merged document", projectID))
				continue
			}
			if !containsString(out.Projects[idx].ViewPins, id) {
				out.Projects[idx].ViewPins = append(out.Projects[idx].ViewPins, id)
			}
		}
	}
	if len(problems) > 0 {
		return nil, errors.Join(problems...)
	}

	// Alerts follow the same identity rule as the rest: an ID that exists
	// locally keeps the local entity, so importing the same file twice does
	// not stack a second watcher.
	alertIDs := map[string]bool{}
	for _, a := range out.Alerts {
		alertIDs[a.ID] = true
	}
	for _, a := range incoming.Alerts {
		if viewsInOut[a.ViewID] && !alertIDs[a.ID] {
			out.Alerts = append(out.Alerts, a)
		}
	}

	if err := out.validate(true); err != nil {
		return nil, fmt.Errorf("import: the merged document would not save: %w", err)
	}
	return &out, nil
}

// mergeEntities appends the incoming entities that are new and replaces the
// local ones the plan says to overwrite, in place so the order stays the
// document's.
func mergeEntities[T interface{ EntityID() string }](local, incoming []T,
	skip, overwrite map[string]bool) []T {
	localByID := map[string]int{}
	for i, e := range local {
		localByID[e.EntityID()] = i
	}
	for _, e := range incoming {
		id := e.EntityID()
		if skip[id] {
			continue
		}
		if idx, ok := localByID[id]; ok {
			if overwrite[id] {
				local[idx] = e
			}
			continue
		}
		local = append(local, e)
	}
	return local
}

func mergeProjects(local, incoming []Project, skippedViews map[string]bool) []Project {
	localByID := map[string]bool{}
	for _, p := range local {
		localByID[p.ID] = true
	}
	for _, p := range incoming {
		if localByID[p.ID] {
			continue
		}
		p.ViewPins = withoutSkipped(p.ViewPins, skippedViews)
		local = append(local, p)
	}
	return local
}

func mergeDashboards(local, incoming []Dashboard, skippedViews map[string]bool) []Dashboard {
	localByID := map[string]bool{}
	for _, d := range local {
		localByID[d.ID] = true
	}
	for _, d := range incoming {
		if localByID[d.ID] {
			continue
		}
		d.ViewRefs = withoutSkipped(d.ViewRefs, skippedViews)
		local = append(local, d)
	}
	return local
}

func withoutSkipped(ids []string, skipped map[string]bool) []string {
	var out []string
	for _, id := range ids {
		if !skipped[id] {
			out = append(out, id)
		}
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// sameEntity reports whether two entities carry identical content. The hash is
// over the canonical JSON the store itself would write, so two files that
// differ only in whitespace still count as the same View — dedupe is by
// content, not by disk bytes.
func sameEntity(a, b any) (bool, error) {
	ha, err := entityHash(a)
	if err != nil {
		return false, err
	}
	hb, err := entityHash(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func entityHash(v any) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

// EntityID lets the generic merge read an entity's identity without knowing
// which type it is.
func (v View) EntityID() string          { return v.ID }
func (m SemanticModel) EntityID() string { return m.ID }
