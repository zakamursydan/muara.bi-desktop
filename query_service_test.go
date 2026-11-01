//go:build cgo

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"muara.bi/engine/duckdb"
)

// The window is the part a human has to look at, but everything it shows is
// produced here, so the spike's real questions get asserted without a GUI.
func TestSpikeStatusReportsEngineDefaults(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	status, err := service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	// MaxMemory is left unset in the config, so this is the engine default as
	// DuckDB echoes it — the same proof the CLI spike printed, now through Wails.
	if status.MaxMemory != "2.0 GiB" {
		t.Errorf("MaxMemory = %q, want the engine default 2.0 GiB", status.MaxMemory)
	}
	if status.Threads != "2" {
		t.Errorf("Threads = %q, want 2", status.Threads)
	}
	if status.DuckDBVersion == "" {
		t.Error("DuckDBVersion is empty, so the engine is not really answering")
	}
}

func TestSpikeQueryGuardrails(t *testing.T) {
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	plain, err := service.Run("SELECT 1 AS one")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if plain.RowCount != 1 || plain.Truncated {
		t.Errorf("small result reported as %d rows, truncated=%v", plain.RowCount, plain.Truncated)
	}

	if _, err := service.Run("DROP TABLE widgets"); !errors.Is(err, duckdb.ErrForbiddenStatement) {
		t.Errorf("DDL accepted by the desktop bridge: %v", err)
	}

	// 20000 rows against the platform cap: the truncation contract (M1.4) has to
	// survive the trip through the binding layer, not only inside the engine.
	cut, err := service.Run("SELECT * FROM range(20000) AS t(i)")
	if err != nil {
		t.Fatalf("run capped query: %v", err)
	}
	if !cut.Truncated || cut.TruncatedBy != duckdb.TruncateRowLimit {
		t.Errorf("capped result reported truncated=%v by=%q", cut.Truncated, cut.TruncatedBy)
	}
	if int64(len(cut.Rows)) != duckdb.DefaultMaxRows {
		t.Errorf("got %d rows, want the cap of %d", len(cut.Rows), duckdb.DefaultMaxRows)
	}
}

// AttachSample reads its DSN from the environment, so a machine without one
// must say so instead of trying to connect somewhere.
func TestSpikeAttachNeedsDSN(t *testing.T) {
	t.Setenv("MUARA_SPIKE_PG_DSN", "")
	service, err := NewQueryService()
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	defer service.Close()

	if err := service.AttachSample(); err == nil {
		t.Fatal("attach succeeded without a DSN")
	}
}

// The offline promise only holds if an installed app actually reads the bundle
// from inside itself: a double-click from Finder carries no shell environment.
// Path layout is what makes that work, so it is asserted without a GUI — and a
// bare `go run .` must keep DuckDB's own default rather than a missing folder.
func TestExtensionDirIsFoundFromAppBundleLayout(t *testing.T) {
	root := t.TempDir()
	app := filepath.Join(root, "muara-bi.app")
	exe := filepath.Join(app, "Contents", "MacOS", "muara-bi")
	bundle := filepath.Join(app, "Contents", "Resources", "duckdb", "extensions")
	if err := os.MkdirAll(bundle, 0o755); err != nil {
		t.Fatalf("make bundle: %v", err)
	}
	if got := extensionDirFromBinary(exe); got != bundle {
		t.Errorf("bundle dir = %q, want %q", got, bundle)
	}

	// Same executable, but the folder was never packaged.
	other := filepath.Join(root, "loose", "Contents", "MacOS", "muara-bi")
	if got := extensionDirFromBinary(other); got != "" {
		t.Errorf("missing bundle resolved to %q, want empty", got)
	}
}

func TestExtensionDirEnvOverridesTheBundle(t *testing.T) {
	t.Setenv(extensionDirEnv, "/tmp/scratch-bundle")
	if got := bundledExtensionDir(); got != "/tmp/scratch-bundle" {
		t.Errorf("bundledExtensionDir() = %q, want the environment to win", got)
	}
}
