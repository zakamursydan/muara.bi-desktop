package bundle

import "fmt"

// Scope selects what an export carries (FR-10). Exactly one option has to be
// set; a zero Scope is refused rather than guessed.
type Scope struct {
	// All carries the whole file.
	All bool
	// ViewID carries one View plus its transitive dependencies.
	ViewID string
	// Folder carries every View whose Folder equals this string, plus their
	// transitive dependencies. Folder is a plain string today; FR-4 defines
	// whether it grows a hierarchy.
	Folder string
	// DashboardID carries one Dashboard plus the Views it shows.
	DashboardID string
	// ProjectID carries one Project, the Views it pins, the Dashboards it
	// references and the Views those show. A Project references Views, it does
	// not own them, so other projects that pin the same View are not carried
	// (BRD §5.2).
	ProjectID string
}

// Export returns the subset of the bundle that the scope names, with
// dependencies resolved transitively: a View brings the semantic models it
// references, a model brings the models its own SQL reads, and everything
// brings the connections its aliases name.
//
// Two rules that keep an export honest:
//   - The source bundle must be consistent (validate(true)): exporting a file
//     with a dangling reference would silently produce a definition that cannot
//     work anywhere, so it is refused with the reason instead.
//   - Sealed is never carried. The credential decision is the export dialog's
//     own (SaveWithSecrets on the exported bundle), never a leftover of the
//     source file's passphrase.
//
// The Manifest travels as the source wrote it. Alerts travel only with All:
// FR-10's scopes do not mention them, and carrying a watcher without deciding
// its channel and schedule for the destination would be a guess.
func (b *Bundle) Export(scope Scope) (*Bundle, error) {
	if err := b.validate(true); err != nil {
		return nil, fmt.Errorf("export: %w", err)
	}
	if !scope.exactlyOne() {
		return nil, fmt.Errorf("export: scope must set exactly one of All, ViewID, Folder, DashboardID, ProjectID")
	}

	if scope.All {
		out := *b
		out.Projects = cloneSlice(b.Projects)
		out.Connections = cloneSlice(b.Connections)
		out.SemanticModels = cloneSlice(b.SemanticModels)
		out.Views = cloneSlice(b.Views)
		out.Dashboards = cloneSlice(b.Dashboards)
		out.Alerts = cloneSlice(b.Alerts)
		out.Sealed = nil
		return &out, nil
	}

	views, err := b.seedViews(scope)
	if err != nil {
		return nil, err
	}
	dashboards, projects := b.seedLayout(scope)

	modelsByID := map[string]SemanticModel{}
	modelsByName := map[string][]SemanticModel{}
	for _, m := range b.SemanticModels {
		modelsByID[m.ID] = m
		modelsByName[m.Name] = append(modelsByName[m.Name], m)
	}

	// Model closure. The stored reference is the primary trace — it survives a
	// rename and a name that is not a plain identifier — and the View's own
	// text adds a fallback scan so hand-written SQL (or a builder_state written
	// before the field existed) that reads a model keeps working. A scan is a
	// superset — a label that happens to equal a model name carries one model
	// too many, never one too few.
	modelIDs := map[string]bool{}
	for _, v := range views {
		for _, ref := range v.ModelRefs {
			if _, ok := modelsByID[ref]; ok {
				modelIDs[ref] = true
			}
		}
		if v.QueryMode == QueryCustomSQL {
			addModelsNamed(modelsByName, modelIDs, sqlIdentifiers(v.RawSQL))
		} else if len(v.BuilderState) > 0 {
			addModelsNamed(modelsByName, modelIDs, sqlIdentifiers(string(v.BuilderState)))
		}
	}
	for changed := true; changed; {
		changed = false
		for _, m := range b.SemanticModels {
			if !modelIDs[m.ID] {
				continue
			}
			for _, name := range sqlIdentifiers(m.RawSQL) {
				for _, dep := range modelsByName[name] {
					if !modelIDs[dep.ID] {
						modelIDs[dep.ID] = true
						changed = true
					}
				}
			}
		}
	}

	// Connections: the aliases the kept Views and models read. A consistent
	// source bundle guarantees each exists.
	connByAlias := map[string]Connection{}
	for _, c := range b.Connections {
		connByAlias[c.Alias] = c
	}
	keptAliases := map[string]bool{}
	for _, v := range views {
		for _, alias := range v.Aliases {
			keptAliases[alias] = true
		}
	}
	for _, m := range b.SemanticModels {
		if modelIDs[m.ID] {
			for _, alias := range m.Aliases {
				keptAliases[alias] = true
			}
		}
	}

	out := &Bundle{Manifest: b.Manifest}
	out.Views = views
	out.Dashboards = dashboards
	out.Projects = projects
	for _, c := range b.Connections {
		if keptAliases[c.Alias] {
			out.Connections = append(out.Connections, c)
		}
	}
	for _, m := range b.SemanticModels {
		if modelIDs[m.ID] {
			out.SemanticModels = append(out.SemanticModels, m)
		}
	}
	return out, nil
}

func (s Scope) exactlyOne() bool {
	n := 0
	for _, on := range []bool{s.All, s.ViewID != "", s.Folder != "", s.DashboardID != "", s.ProjectID != ""} {
		if on {
			n++
		}
	}
	return n == 1
}

// seedViews is the per-scope starting set; the dependencies come on top.
func (b *Bundle) seedViews(scope Scope) ([]View, error) {
	switch {
	case scope.ViewID != "":
		for _, v := range b.Views {
			if v.ID == scope.ViewID {
				return []View{v}, nil
			}
		}
		return nil, fmt.Errorf("export: view %q is not in this bundle", scope.ViewID)

	case scope.Folder != "":
		var out []View
		for _, v := range b.Views {
			if v.Folder == scope.Folder {
				out = append(out, v)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("export: folder %q holds no view in this bundle", scope.Folder)
		}
		return out, nil

	case scope.DashboardID != "":
		for _, d := range b.Dashboards {
			if d.ID == scope.DashboardID {
				return b.viewsNamed(d.ViewRefs), nil
			}
		}
		return nil, fmt.Errorf("export: dashboard %q is not in this bundle", scope.DashboardID)

	case scope.ProjectID != "":
		for _, p := range b.Projects {
			if p.ID == scope.ProjectID {
				refs := append(append([]string(nil), p.ViewPins...), dashboardViewRefs(b.Dashboards, p.DashboardRefs)...)
				return b.viewsNamed(refs), nil
			}
		}
		return nil, fmt.Errorf("export: project %q is not in this bundle", scope.ProjectID)
	}
	return nil, fmt.Errorf("export: no scope option set")
}

// seedLayout is what travels next to the Views: a Dashboard scope carries that
// dashboard, a Project scope carries the project and the dashboards it
// references. View and Folder scopes carry no layout — they export a
// definition, not an arrangement.
func (b *Bundle) seedLayout(scope Scope) (dashboards []Dashboard, projects []Project) {
	switch {
	case scope.DashboardID != "":
		for _, d := range b.Dashboards {
			if d.ID == scope.DashboardID {
				return []Dashboard{d}, nil
			}
		}
	case scope.ProjectID != "":
		for _, p := range b.Projects {
			if p.ID == scope.ProjectID {
				projects = []Project{p}
				for _, ref := range p.DashboardRefs {
					for _, d := range b.Dashboards {
						if d.ID == ref {
							dashboards = append(dashboards, d)
						}
					}
				}
				return dashboards, projects
			}
		}
	}
	return dashboards, projects
}

func dashboardViewRefs(dashboards []Dashboard, refs []string) []string {
	var out []string
	for _, d := range dashboards {
		for _, ref := range refs {
			if d.ID == ref {
				out = append(out, d.ViewRefs...)
			}
		}
	}
	return out
}

// viewsNamed keeps the source order — the order the user wrote them in — and
// drops repeats: a project may pin a View its dashboard also shows, and that
// still exports as one View.
func (b *Bundle) viewsNamed(names []string) []View {
	want := map[string]bool{}
	for _, name := range names {
		want[name] = true
	}
	var out []View
	for _, v := range b.Views {
		if want[v.ID] {
			out = append(out, v)
		}
	}
	return out
}

func addModelsNamed(modelsByName map[string][]SemanticModel, modelIDs map[string]bool, names []string) {
	for _, name := range names {
		for _, m := range modelsByName[name] {
			modelIDs[m.ID] = true
		}
	}
}

func cloneSlice[T any](in []T) []T {
	out := make([]T, len(in))
	copy(out, in)
	return out
}

// sqlIdentifiers extracts the identifiers an SQL text mentions: unquoted words
// and "double-quoted" names (with "" as the escaped quote). Single-quoted
// string literals and -- and /* */ comments are skipped, so `SELECT
// 'order_lines'` and `-- order_lines` do not look like references.
//
// It is a scanner, not a parser: SELECT/WHERE and friends come out too, which
// is fine for name matching. Backticks are not DuckDB quoting, so they are not
// treated as one.
func sqlIdentifiers(sql string) []string {
	var out []string
	i, n := 0, len(sql)
	for i < n {
		switch c := sql[i]; {
		case c == '\'':
			i = skipDelimited(sql, i, '\'')
		case c == '"':
			end := i + 1
			var sb []byte
			for end < n {
				if sql[end] == '"' {
					if end+1 < n && sql[end+1] == '"' {
						sb = append(sb, '"')
						end += 2
						continue
					}
					break
				}
				sb = append(sb, sql[end])
				end++
			}
			if end < n {
				out = append(out, string(sb))
				i = end + 1
			} else {
				i = n
			}
		case c == '-' && i+1 < n && sql[i+1] == '-':
			for i < n && sql[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && sql[i+1] == '*':
			i += 2
			for i+1 < n && !(sql[i] == '*' && sql[i+1] == '/') {
				i++
			}
			if i < n {
				i += 2
			}
		case isIdentStart(c):
			j := i + 1
			for j < n && isIdentPart(sql[j]) {
				j++
			}
			out = append(out, sql[i:j])
			i = j
		default:
			i++
		}
	}
	return out
}

func skipDelimited(s string, start int, quote byte) int {
	i := start + 1
	for i < len(s) {
		if s[i] == quote {
			if i+1 < len(s) && s[i+1] == quote {
				i += 2
				continue
			}
			return i + 1
		}
		i++
	}
	return len(s)
}

func isIdentStart(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || ('0' <= c && c <= '9')
}
