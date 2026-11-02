// Package bundle is the .muara file format: one zip holding one JSON document
// per entity type, plus a manifest that says which layout the rest of the
// entries follow.
//
// Nothing here opens a DuckDB session — the only engine code it uses is the
// path rule from BRD §5.5 — so a bundle can be read and validated on a machine
// that never connects anywhere. That is what lets an import pre-flight (M4)
// inspect a file before any query runs.
package bundle

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"muara.bi/engine/sqlident"
)

const (
	// Format is the only value this app writes into manifest.format, and the only
	// one it will read.
	Format = "muara.bundle"

	// FormatVersion is the layout version written and accepted.
	FormatVersion = 1

	AppName    = "muara.bi Desktop"
	AppVersion = "0.1.0"
)

// ErrInvalid marks a definition that cannot be stored, as opposed to an I/O
// failure reading or writing one.
var ErrInvalid = errors.New("bundle: invalid definition")

// hexColor is the shape a project label may take; anything else reaching CSS or
// canvas is a bug, so it never leaves here.
var hexColor = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

// Driver names the bundle may record. The set is closed on purpose: an
// unrecognised driver in a stored connection means the file was written by
// something that does not respect this format.
type Driver string

const (
	DriverPostgres Driver = "postgres"
	DriverMySQL    Driver = "mysql"
	DriverSQLite   Driver = "sqlite"
	DriverMongoDB  Driver = "mongodb"
	DriverFile     Driver = "file"
)

func (d Driver) valid() bool {
	switch d {
	case DriverPostgres, DriverMySQL, DriverSQLite, DriverMongoDB, DriverFile:
		return true
	}
	return false
}

// Kind is how a View is drawn.
type Kind string

const (
	KindTable   Kind = "table"
	KindLine    Kind = "line"
	KindBar     Kind = "bar"
	KindPie     Kind = "pie"
	KindKPI     Kind = "kpi"
	KindScatter Kind = "scatter"
	KindArea    Kind = "area"
)

func (k Kind) valid() bool {
	switch k {
	case KindTable, KindLine, KindBar, KindPie, KindKPI, KindScatter, KindArea:
		return true
	}
	return false
}

// QueryMode decides where the SQL of a View comes from.
type QueryMode string

const (
	// QuerySynced compiles BuilderState through the engine's compiler so the
	// guardrails apply to it.
	QuerySynced QueryMode = "SYNCED"
	// QueryCustomSQL stores hand-written SQL, still run through the guardrails.
	QueryCustomSQL QueryMode = "CUSTOM_SQL"
)

func (q QueryMode) valid() bool {
	return q == QuerySynced || q == QueryCustomSQL
}

// AppInfo identifies the build that wrote a bundle.
type AppInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Manifest is the first thing read from a bundle, and the gate in front of
// everything else.
type Manifest struct {
	Format        string    `json:"format"`
	Version       int       `json:"version"`
	MinAppVersion string    `json:"min_app_version"`
	CreatedAt     time.Time `json:"created_at"`
	ExportedBy    string    `json:"exported_by"`
	App           AppInfo   `json:"app"`

	// OriginWorkspaceID and OriginProjectID are opaque strings: they say where a
	// bundle came from, never what tenant it belongs to. A web import must not
	// infer membership from them (BRD §6).
	OriginWorkspaceID string `json:"origin_workspace_id,omitempty"`
	OriginProjectID   string `json:"origin_project_id,omitempty"`
}

// Project wraps dashboards only. A View is not owned by a project, it is pinned
// into one, which is why membership is a list of pins rather than a project_id
// on the View (BRD §5.2).
type Project struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Color         string   `json:"color"`
	Note          string   `json:"note,omitempty"`
	DashboardRefs []string `json:"dashboard_refs"`
	ViewPins      []string `json:"view_pins"`
}

// Address is the tier-2 group of §5.4: not secret, but not something to hand to
// another team either, so it travels as one optional block that an export can
// drop entirely.
type Address struct {
	Host    string `json:"host,omitempty"`
	Port    int    `json:"port,omitempty"`
	User    string `json:"user,omitempty"`
	SSLMode string `json:"ssl_mode,omitempty"`
}

// SSHTarget is the jump host a connection is tunneled through. The key and its
// passphrase are tier-3 and have no field here at all.
type SSHTarget struct {
	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`
}

// FileSource points at one file this machine reads: a CSV/Parquet/JSON for a
// `file` connection, or the database file of a `sqlite` connection — SQLite has
// no server, so its "host" is a path and §5.5's rule applies to it in exactly
// the same way. Path is stored relative to the folder of the .muara file
// whenever the file can live there; outside it, it stays an absolute
// machine-local path (§5.5).
type FileSource struct {
	Path string `json:"path"`
}

// Connection is the portable half of a data source: what SQL needs to resolve
// plus, optionally, where the server is.
//
// There is deliberately no password, token, key, DSN or connection-string field
// on this type. Credentials live in this machine's keychain and reach a bundle
// only through the opt-in secrets.enc block. A struct that cannot hold a secret
// is a stronger guarantee for the "grep the bundle, find nothing" exit
// criterion than any scrub step could be.
type Connection struct {
	ID     string `json:"id"`
	Alias  string `json:"alias"`
	Driver Driver `json:"driver"`

	// Tier 1 of §5.4: without these the SQL of a View does not resolve.
	Database     string   `json:"database,omitempty"`
	SchemaFilter []string `json:"schema_filter,omitempty"`
	Note         string   `json:"note,omitempty"`

	Address *Address    `json:"address,omitempty"`
	SSH     *SSHTarget  `json:"ssh_config,omitempty"`
	File    *FileSource `json:"file,omitempty"`
}

// SemanticModel is a saved piece of SQL that other definitions build on.
type SemanticModel struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	RawSQL      string   `json:"raw_sql"`
	Aliases     []string `json:"aliases"`
}

// View is the entity the product is made of, and the reason this format exists:
// it must survive a save and a reopen whole, including the raw messages that
// the builder and the chart each own.
type View struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Folder    string    `json:"folder,omitempty"`
	Kind      Kind      `json:"kind"`
	QueryMode QueryMode `json:"query_mode"`

	// BuilderState and Presentation stay raw JSON: their shape belongs to the
	// visual builder (FR-4) and the chart renderer, not to the store. Keeping
	// them unmarshal-untouched is what makes a round trip lossless.
	BuilderState json.RawMessage `json:"builder_state,omitempty"`
	RawSQL       string          `json:"raw_sql,omitempty"`
	Presentation json.RawMessage `json:"presentation,omitempty"`

	MaxRows         int      `json:"max_rows,omitempty"`
	CacheTTLSeconds int      `json:"cache_ttl_seconds,omitempty"`
	Aliases         []string `json:"aliases"`
}

// Dashboard arranges Views; the layout and the global filters are the canvas's
// own document.
type Dashboard struct {
	ID                     string          `json:"id"`
	Name                   string          `json:"name"`
	Layout                 json.RawMessage `json:"layout,omitempty"`
	ViewRefs               []string        `json:"view_refs"`
	GlobalFilters          json.RawMessage `json:"global_filters,omitempty"`
	RefreshIntervalSeconds int             `json:"refresh_interval_seconds,omitempty"`
}

// Alert watches a View. Threshold stays raw until FR-9 defines its operators.
type Alert struct {
	ID        string          `json:"id"`
	ViewID    string          `json:"view_id"`
	Threshold json.RawMessage `json:"threshold,omitempty"`
	Channel   string          `json:"channel"`
	Schedule  string          `json:"schedule"`
}

// Bundle is one .muara file in memory. It is never itself encoded as a JSON
// document — each list below goes to its own entry, and Manifest to its own.
type Bundle struct {
	Manifest       Manifest
	Projects       []Project
	Connections    []Connection
	SemanticModels []SemanticModel
	Views          []View
	Dashboards     []Dashboard
	Alerts         []Alert
}

// checkRefs is true when writing, false when reading. The app that owns the
// data must not be able to save a dangling reference; the app that imports it
// must be able to open it and say which alias or View is missing rather than
// fail on the first one (§5.4, §5.2).
func (b *Bundle) validate(checkRefs bool) error {
	projects := map[string]bool{}
	for _, p := range b.Projects {
		if p.ID == "" {
			return fmt.Errorf("%w: a project has no id", ErrInvalid)
		}
		if projects[p.ID] {
			return fmt.Errorf("%w: duplicate project id %q", ErrInvalid, p.ID)
		}
		projects[p.ID] = true
		if strings.TrimSpace(p.Name) == "" {
			return fmt.Errorf("%w: project %q has no name", ErrInvalid, p.ID)
		}
		if !hexColor.MatchString(p.Color) {
			return fmt.Errorf("%w: project %q color %q is not #rrggbb", ErrInvalid, p.ID, p.Color)
		}
	}

	views := map[string]bool{}
	for _, v := range b.Views {
		if v.ID == "" {
			return fmt.Errorf("%w: a view has no id", ErrInvalid)
		}
		if views[v.ID] {
			return fmt.Errorf("%w: duplicate view id %q", ErrInvalid, v.ID)
		}
		views[v.ID] = true
		if strings.TrimSpace(v.Name) == "" {
			return fmt.Errorf("%w: view %q has no name", ErrInvalid, v.ID)
		}
		if !v.Kind.valid() {
			return fmt.Errorf("%w: view %q has kind %q, want one of table, line, bar, pie, kpi, scatter, area",
				ErrInvalid, v.ID, v.Kind)
		}
		if !v.QueryMode.valid() {
			return fmt.Errorf("%w: view %q has query_mode %q, want SYNCED or CUSTOM_SQL",
				ErrInvalid, v.ID, v.QueryMode)
		}
		// A View that cannot say what it queries would reopen as an empty widget.
		switch v.QueryMode {
		case QuerySynced:
			if len(v.BuilderState) == 0 {
				return fmt.Errorf("%w: view %q is SYNCED but carries no builder_state", ErrInvalid, v.ID)
			}
		case QueryCustomSQL:
			if strings.TrimSpace(v.RawSQL) == "" {
				return fmt.Errorf("%w: view %q is CUSTOM_SQL but carries no raw_sql", ErrInvalid, v.ID)
			}
		}
		if v.MaxRows < 0 {
			return fmt.Errorf("%w: view %q has a negative max_rows", ErrInvalid, v.ID)
		}
		if v.CacheTTLSeconds < 0 {
			return fmt.Errorf("%w: view %q has a negative cache_ttl_seconds", ErrInvalid, v.ID)
		}
	}

	aliases := map[string]bool{}
	for _, c := range b.Connections {
		if c.ID == "" {
			return fmt.Errorf("%w: a connection has no id", ErrInvalid)
		}
		if strings.TrimSpace(c.Alias) == "" {
			return fmt.Errorf("%w: connection %q has no alias", ErrInvalid, c.ID)
		}
		if !sqlident.IsPlain(c.Alias) {
			return fmt.Errorf("%w: connection alias %q is not a plain identifier — it is the name a query reads tables through",
				ErrInvalid, c.Alias)
		}
		if aliases[c.Alias] {
			return fmt.Errorf("%w: duplicate alias %q", ErrInvalid, c.Alias)
		}
		aliases[c.Alias] = true
		if !c.Driver.valid() {
			return fmt.Errorf("%w: connection %q has driver %q, want postgres, mysql, sqlite, mongodb or file",
				ErrInvalid, c.ID, c.Driver)
		}
		if c.Driver == DriverFile || c.Driver == DriverSQLite {
			if c.File == nil || strings.TrimSpace(c.File.Path) == "" {
				return fmt.Errorf("%w: connection %q reads %q from a file but names no path",
					ErrInvalid, c.ID, c.Driver)
			}
		} else if c.File != nil {
			return fmt.Errorf("%w: connection %q is driver %q yet carries a file source", ErrInvalid, c.ID, c.Driver)
		}
	}

	models := map[string]bool{}
	for _, s := range b.SemanticModels {
		if s.ID == "" {
			return fmt.Errorf("%w: a semantic model has no id", ErrInvalid)
		}
		if models[s.ID] {
			return fmt.Errorf("%w: duplicate semantic model id %q", ErrInvalid, s.ID)
		}
		models[s.ID] = true
		if strings.TrimSpace(s.Name) == "" {
			return fmt.Errorf("%w: semantic model %q has no name", ErrInvalid, s.ID)
		}
		if strings.TrimSpace(s.RawSQL) == "" {
			return fmt.Errorf("%w: semantic model %q has no raw_sql", ErrInvalid, s.ID)
		}
	}

	dashboards := map[string]bool{}
	for _, d := range b.Dashboards {
		if d.ID == "" {
			return fmt.Errorf("%w: a dashboard has no id", ErrInvalid)
		}
		if dashboards[d.ID] {
			return fmt.Errorf("%w: duplicate dashboard id %q", ErrInvalid, d.ID)
		}
		dashboards[d.ID] = true
		if strings.TrimSpace(d.Name) == "" {
			return fmt.Errorf("%w: dashboard %q has no name", ErrInvalid, d.ID)
		}
	}

	alertIDs := map[string]bool{}
	for _, a := range b.Alerts {
		if a.ID == "" {
			return fmt.Errorf("%w: an alert has no id", ErrInvalid)
		}
		if alertIDs[a.ID] {
			return fmt.Errorf("%w: duplicate alert id %q", ErrInvalid, a.ID)
		}
		alertIDs[a.ID] = true
		if strings.TrimSpace(a.Channel) == "" {
			return fmt.Errorf("%w: alert %q has no channel", ErrInvalid, a.ID)
		}
		if strings.TrimSpace(a.Schedule) == "" {
			return fmt.Errorf("%w: alert %q has no schedule", ErrInvalid, a.ID)
		}
	}

	if !checkRefs {
		return nil
	}
	for _, p := range b.Projects {
		for _, ref := range p.DashboardRefs {
			if !dashboards[ref] {
				return fmt.Errorf("%w: project %q refs dashboard %q that is not in this bundle", ErrInvalid, p.ID, ref)
			}
		}
		for _, pin := range p.ViewPins {
			if !views[pin] {
				return fmt.Errorf("%w: project %q pins view %q that is not in this bundle", ErrInvalid, p.ID, pin)
			}
		}
	}
	for _, d := range b.Dashboards {
		for _, ref := range d.ViewRefs {
			if !views[ref] {
				return fmt.Errorf("%w: dashboard %q shows view %q that is not in this bundle", ErrInvalid, d.ID, ref)
			}
		}
	}
	for _, a := range b.Alerts {
		if !views[a.ViewID] {
			return fmt.Errorf("%w: alert %q watches view %q that is not in this bundle", ErrInvalid, a.ID, a.ViewID)
		}
	}
	for _, v := range b.Views {
		for _, alias := range v.Aliases {
			if !aliases[alias] {
				return fmt.Errorf("%w: view %q reads alias %q that this bundle does not define", ErrInvalid, v.ID, alias)
			}
		}
	}
	for _, s := range b.SemanticModels {
		for _, alias := range s.Aliases {
			if !aliases[alias] {
				return fmt.Errorf("%w: semantic model %q reads alias %q that this bundle does not define",
					ErrInvalid, s.ID, alias)
			}
		}
	}
	return nil
}
