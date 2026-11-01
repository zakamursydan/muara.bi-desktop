package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"muara.bi/engine/duckdb"
	"muara.bi/engine/sqlident"
)

// extensionDirEnv is how a developer points a build at a scratch bundle.
const extensionDirEnv = "DUCKDB_EXTENSION_DIRECTORY"

// QueryService is the only bridge between the webview and the engine: the UI
// never sees DuckDB, only these methods.
type QueryService struct {
	engine  duckdb.Engine
	session duckdb.Session
}

// EngineStatus is what the window shows before any query runs, so "the engine
// is really in this binary" is visible rather than assumed.
type EngineStatus struct {
	DuckDBVersion      string `json:"duckdbVersion"`
	MaxMemory          string `json:"maxMemory"`
	Threads            string `json:"threads"`
	TempDirectory      string `json:"tempDirectory"`
	ExtensionDirectory string `json:"extensionDirectory"`
}

func NewQueryService() (*QueryService, error) {
	cfg := duckdb.GuardrailConfig{
		MaxRows:          duckdb.DefaultMaxRows,
		StatementTimeout: 15 * time.Second,
		// MaxMemory is intentionally empty: the spike must show that the engine
		// default reaches DuckDB on its own, which is what FR-15 will override.
		ExtensionDirectory: bundledExtensionDir(),
	}
	engine, err := duckdb.NewEngine(cfg)
	if err != nil {
		return nil, err
	}
	session, err := engine.NewSession(context.Background())
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	return &QueryService{engine: engine, session: session}, nil
}

func (s *QueryService) Close() error {
	_ = s.session.Close()
	return s.engine.Close()
}

func (s *QueryService) Status() (*EngineStatus, error) {
	res, err := s.session.Query(context.Background(),
		`SELECT version() AS version,
			current_setting('max_memory') AS max_memory,
			current_setting('threads') AS threads,
			current_setting('temp_directory') AS temp_directory,
			current_setting('extension_directory') AS extension_directory`,
		duckdb.QueryOptions{})
	if err != nil {
		return nil, err
	}
	if len(res.Rows) == 0 {
		return nil, fmt.Errorf("duckdb returned no settings row")
	}
	row := res.Rows[0]
	return &EngineStatus{
		DuckDBVersion:      asText(row["version"]),
		MaxMemory:          asText(row["max_memory"]),
		Threads:            asText(row["threads"]),
		TempDirectory:      asText(row["temp_directory"]),
		ExtensionDirectory: asText(row["extension_directory"]),
	}, nil
}

// AttachSample mounts the local Postgres fixture. The DSN comes from the
// environment, never from the UI: credentials must not be typable into a
// webview, and later they live in the machine keychain (BRD §6).
func (s *QueryService) AttachSample() error {
	dsn := os.Getenv("MUARA_SPIKE_PG_DSN")
	if dsn == "" {
		return fmt.Errorf("MUARA_SPIKE_PG_DSN is not set")
	}
	const alias = "sample"
	if !sqlident.IsPlain(alias) {
		return fmt.Errorf("alias %q is not a plain identifier", alias)
	}
	return s.session.AttachPostgres(context.Background(), alias, dsn)
}

// Run executes one statement under the engine's guardrails. ValidateQuery is
// called here rather than trusted from the UI, matching the product rule that
// the desktop app is read-only.
func (s *QueryService) Run(sql string) (*duckdb.QueryResult, error) {
	if err := duckdb.ValidateQuery(sql); err != nil {
		return nil, err
	}
	return s.session.Query(context.Background(), sql, duckdb.QueryOptions{MaxRows: duckdb.DefaultMaxRows})
}

// bundledExtensionDir is how an installed .app finds the connector bundle that
// ships inside it. A double-click from Finder carries no shell environment, so
// relying on the variable alone would silently leave the packaged app with
// autoinstall enabled and no offline promise.
func bundledExtensionDir() string {
	if env := os.Getenv(extensionDirEnv); env != "" {
		return env
	}
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return ""
	}
	return extensionDirFromBinary(resolved)
}

// extensionDirFromBinary maps <App>/Contents/MacOS/<exe> to
// <App>/Contents/Resources/duckdb/extensions and answers "" when that folder
// is absent, so a plain `go run .` keeps DuckDB's own default directory.
func extensionDirFromBinary(exePath string) string {
	contents := filepath.Dir(filepath.Dir(exePath))
	candidate := filepath.Join(contents, "Resources", "duckdb", "extensions")
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return ""
}

func asText(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
