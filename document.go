package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"muara.bi/desktop/internal/bundle"
	"muara.bi/desktop/internal/profile"
	"muara.bi/engine/duckdb"
)

// MountReport is one line of "what this machine did with the sources this bundle
// names". It exists because opening a bundle is not all-or-nothing: a file the
// user moved, an alias this machine has no target for, and a server whose
// credential lives in another laptop's keychain all have to be *reported* while
// the rest of the bundle still works (BRD §5.4, §5.5).
type MountReport struct {
	// Name is the alias a query reads through, or the model name.
	Name string `json:"name"`
	// Kind separates a data source from a saved piece of SQL.
	Kind string `json:"kind"`
	// Driver is the bundle's word for what the source is.
	Driver string `json:"driver,omitempty"`
	// Mounted is true only when the session can answer a query that names it.
	Mounted bool `json:"mounted"`
	// Target is the path or host:port this machine resolved the alias to, so the
	// user can see which of the two sources of truth won.
	Target string `json:"target,omitempty"`
	// Reason explains a Mounted=false, and is written to be read by an analyst
	// who has not seen this code.
	Reason string `json:"reason,omitempty"`
}

// DocumentInfo is the window's answer to "which file am I working on".
type DocumentInfo struct {
	Open   bool   `json:"open"`
	Path   string `json:"path,omitempty"`
	Name   string `json:"name,omitempty"`
	Folder string `json:"folder,omitempty"`
	// Dirty is set by the editors that mutate definitions; the store is written
	// so that it can be, and nothing sets it until FR-4 has a form to change a
	// View in.
	Dirty bool `json:"dirty"`
	// CreatedAt is the manifest's stamp, shown so two Untitled files can be
	// told apart.
	CreatedAt string `json:"createdAt,omitempty"`

	Projects       int `json:"projects"`
	Connections    int `json:"connections"`
	SemanticModels int `json:"semanticModels"`
	Views          int `json:"views"`
	Dashboards     int `json:"dashboards"`
	Alerts         int `json:"alerts"`

	Mounts []MountReport `json:"mounts,omitempty"`

	// ProfileError is shown when this machine's profile file could not be read:
	// the bundle still opens, but the user has to be told the alias map they
	// expect is not in front of the app.
	ProfileError string `json:"profileError,omitempty"`
}

// openDocument is the one file the app is working on (BRD §5.1). It travels with
// its own DuckDB session: see mountLocked for why the two are replaced together.
type openDocument struct {
	path    string
	bundle  *bundle.Bundle
	dirty   bool
	mounts  []MountReport
	created time.Time
}

// Current is what the window asks before the user does anything: engine status
// is half the story, the open file is the other half.
func (s *QueryService) Current() (*DocumentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.documentInfoLocked(), nil
}

// NewUntitled makes the file "mulai kosong" is supposed to produce (§5.1) and
// works on it. It is a real file from the first moment: the product has no
// unsaved-buffer state to lose, and Save means Save.
//
// An existing Untitled.muara is never overwritten — the previous session's only
// copy of its settings would be gone with no way back — so the next free name in
// the folder is taken instead.
func (s *QueryService) NewUntitled() (*DocumentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	folder, err := bundlesFolder()
	if err != nil {
		return nil, err
	}
	path, err := untitledPath(folder)
	if err != nil {
		return nil, err
	}

	fresh := &bundle.Bundle{}
	if err := bundle.Save(path, fresh); err != nil {
		return nil, err
	}
	return s.openLocked(path)
}

// Open reads a .muara file, mounts its sources in a session of its own, and makes
// it the document the app works on.
func (s *QueryService) Open(path string) (*DocumentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openLocked(path)
}

// Save writes the open document back where it came from.
func (s *QueryService) Save() (*DocumentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc == nil {
		return nil, fmt.Errorf("no bundle is open — make one with NewUntitled or open a file first")
	}
	return s.saveLocked(s.doc.path)
}

// SaveAs is Export in BRD §5.1's sense: the app starts working on the file it
// just wrote, so "Save As" and "export a copy" are the same operation with the
// same path through the store.
func (s *QueryService) SaveAs(path string) (*DocumentInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.doc == nil {
		return nil, fmt.Errorf("no bundle is open — make one with NewUntitled or open a file first")
	}
	info, err := s.saveLocked(path)
	if err != nil {
		return nil, err
	}
	s.doc.path = info.Path
	s.doc.dirty = false
	return s.documentInfoLocked(), nil
}

func (s *QueryService) saveLocked(path string) (*DocumentInfo, error) {
	if err := bundle.Save(path, s.doc.bundle); err != nil {
		return nil, err
	}
	s.doc.path = path
	s.doc.dirty = false
	// Re-writing a bundle can move every relative source: the anchor is the new
	// folder, so the mounts are computed again rather than shown as they were.
	return s.openLocked(path)
}

// openLocked replaces both halves of the working state at once: the bundle and
// the DuckDB session it is mounted in. They go together on purpose — a session
// keeps attached catalogs and temp views, so reusing the old one would let a
// query in file B resolve against a server file A attached, silently and
// incorrectly. If any step fails the previous document keeps its session.
func (s *QueryService) openLocked(path string) (*DocumentInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", path, err)
	}
	b, err := bundle.Open(abs)
	if err != nil {
		return nil, err
	}

	session, err := s.engine.NewSession(context.Background())
	if err != nil {
		return nil, fmt.Errorf("start an analytical session for %s: %w", filepath.Base(abs), err)
	}
	mounts := s.mountLocked(context.Background(), session, b, abs)

	if s.session != nil {
		_ = s.session.Close()
	}
	s.session = session
	s.doc = &openDocument{
		path:    abs,
		bundle:  b,
		mounts:  mounts,
		created: b.Manifest.CreatedAt,
	}
	return s.documentInfoLocked(), nil
}

// mountLocked attaches every source the bundle names and saves every semantic
// model, one at a time, so one unusable source costs only itself. The order is
// the bundle's, which is the order the user wrote them in.
func (s *QueryService) mountLocked(ctx context.Context, session duckdb.Session, b *bundle.Bundle, path string) []MountReport {
	resolved, resolveErr := b.ResolveFileSources(path)

	reports := make([]MountReport, 0, len(b.Connections)+len(b.SemanticModels))
	for _, c := range b.Connections {
		report := MountReport{Name: c.Alias, Kind: "connection", Driver: string(c.Driver)}
		switch c.Driver {
		case bundle.DriverFile, bundle.DriverSQLite:
			mountFile(ctx, session, c, resolved, resolveErr, &report)
		case bundle.DriverMongoDB:
			report.Target, report.Reason = s.serverSourceNote(c)
			report.Reason = fmt.Sprintf("alias %q is a MongoDB source: a desktop session has no MongoDB attach "+
				"path yet — the web product streams collections through its own adapter, and FR-2 has to decide "+
				"the same shape here", c.Alias)
		default:
			report.Target, report.Reason = s.serverSourceNote(c)
		}
		reports = append(reports, report)
	}

	for _, m := range b.SemanticModels {
		report := MountReport{Name: m.Name, Kind: "semantic_model"}
		if err := duckdb.MountSemanticModels(ctx, session,
			[]duckdb.SemanticModel{{ID: m.ID, Name: m.Name, Description: m.Description, RawSQL: m.RawSQL}}); err != nil {
			report.Reason = err.Error()
		} else {
			report.Mounted = true
			report.Target = "temp view"
		}
		reports = append(reports, report)
	}
	return reports
}

// mountFile reads one file-shaped source: it decides which of the three fates of
// §5.5 this source is in *before* handing the path to DuckDB, because the engine
// answers a missing file, a folder, and an unmatched pattern with one identical
// IO Error.
func mountFile(ctx context.Context, session duckdb.Session, c bundle.Connection,
	resolved map[string]string, resolveErr error, report *MountReport) {
	if resolveErr != nil {
		report.Reason = resolveErr.Error()
		return
	}
	path, ok := resolved[c.Alias]
	if !ok {
		report.Reason = "the bundle gives no path for this source"
		return
	}
	report.Target = path

	// Three fates are distinguished here rather than left to DuckDB, because the
	// engine answers a missing file, a folder, and an unmatched pattern with the
	// same IO Error (§5.5).
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		report.Reason = fmt.Sprintf("no file at %s — point this source at it again, or move the file back", path)
		return
	case err != nil:
		report.Reason = fmt.Sprintf("cannot read %s: %v", path, err)
		return
	case info.IsDir():
		report.Reason = fmt.Sprintf("%s is a folder, and a source names one file", path)
		return
	}

	if c.Driver == bundle.DriverSQLite {
		if err := session.AttachSQLite(ctx, c.Alias, path); err != nil {
			report.Reason = err.Error()
			return
		}
		report.Mounted = true
		return
	}
	if err := session.RegisterFileSource(ctx, c.Alias, path); err != nil {
		report.Reason = err.Error()
		return
	}
	report.Mounted = true
}

// serverSourceNote reports where a server source would be read from on this
// machine, and why it is not mounted in this build.
//
// The address is worth showing now: it is the half of the alias map that a user
// can already fix in Settings, and a bundle whose alias resolves to a host this
// machine has never heard of is a different problem from one that has the host
// but no credential. Neither can attach yet — the desktop reads credentials from
// the OS keychain, which is M6 work, and the libpq conninfo rule that Postgres
// needs still lives inside the web module the desktop may not import.
func (s *QueryService) serverSourceNote(c bundle.Connection) (target, reason string) {
	resolved, from := s.addressFor(c)
	if resolved == "" {
		return "", fmt.Sprintf("alias %q has no address on this machine and the bundle carries none — "+
			"fill it in under Settings › Profil", c.Alias)
	}
	if _, err := s.profile.Secret(c.Alias); err != nil {
		return resolved, fmt.Sprintf("%s (address taken from %s)", err, from)
	}
	// Both halves the user owns are in place; what is missing is code, not setup.
	// Saying which is what makes this line actionable in pre-flight.
	return resolved, fmt.Sprintf("%s source at %s (address taken from %s): this build has no conninfo builder "+
		"for a server source yet, so the address is shown and the connection is not opened — that is M6 work "+
		"together with the keychain reader", c.Driver, resolved, from)
}

// addressFor answers where an alias lives *here*. The profile wins over the
// bundle's tier-2 block because the bundle carries the exporter's server, and
// BRD §5.2 makes "change the host of an alias in the local profile" the act that
// moves every project that reads it.
func (s *QueryService) addressFor(c bundle.Connection) (display, from string) {
	if t, ok := s.profile.Look(c.Alias); ok && strings.TrimSpace(t.Host) != "" {
		return formatAddress(t.Host, t.Port, t.User), "this machine's profile"
	}
	if c.Address != nil && strings.TrimSpace(c.Address.Host) != "" {
		return formatAddress(c.Address.Host, c.Address.Port, c.Address.User), "the bundle"
	}
	return "", ""
}

func formatAddress(host string, port int, user string) string {
	out := strings.TrimSpace(host)
	if port > 0 {
		out = fmt.Sprintf("%s:%d", out, port)
	}
	if u := strings.TrimSpace(user); u != "" {
		out = u + "@" + out
	}
	return out
}

// documentInfoLocked is the JSON the window renders. Nothing here reads source
// data: it counts definitions and repeats what the mount step already decided.
func (s *QueryService) documentInfoLocked() *DocumentInfo {
	info := &DocumentInfo{}
	if s.profileError != "" {
		info.ProfileError = s.profileError
	}
	if s.doc == nil {
		return info
	}
	b := s.doc.bundle
	info.Open = true
	info.Path = s.doc.path
	info.Name = filepath.Base(s.doc.path)
	info.Folder = filepath.Dir(s.doc.path)
	info.Dirty = s.doc.dirty
	info.CreatedAt = s.doc.created.UTC().Format(time.RFC3339)
	info.Projects = len(b.Projects)
	info.Connections = len(b.Connections)
	info.SemanticModels = len(b.SemanticModels)
	info.Views = len(b.Views)
	info.Dashboards = len(b.Dashboards)
	info.Alerts = len(b.Alerts)
	info.Mounts = s.doc.mounts
	return info
}

// bundlesFolder is where a file the user never named goes, and it has to be a
// place they can find again: Documents/muara.bi when Documents exists, the
// per-user config folder otherwise (a headless CI run has no Documents).
func bundlesFolder() (string, error) {
	if home, err := os.UserHomeDir(); err == nil {
		documents := filepath.Join(home, "Documents")
		if info, err := os.Stat(documents); err == nil && info.IsDir() {
			folder := filepath.Join(documents, profile.AppFolder)
			return folder, os.MkdirAll(folder, 0o755)
		}
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("find a folder to create a bundle in: %w", err)
	}
	folder := filepath.Join(config, profile.AppFolder, "bundles")
	return folder, os.MkdirAll(folder, 0o755)
}

// untitledPath takes the first free Untitled name instead of replacing one.
func untitledPath(folder string) (string, error) {
	candidate := filepath.Join(folder, "Untitled.muara")
	for n := 2; n <= 100; n++ {
		if _, err := os.Stat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		} else if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("check %s: %w", candidate, err)
		}
		candidate = filepath.Join(folder, fmt.Sprintf("Untitled-%d.muara", n))
	}
	return "", fmt.Errorf("%s already holds 100 Untitled bundles — give the next one a real name", folder)
}
