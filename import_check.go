package main

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"muara.bi/desktop/internal/bundle"
	"muara.bi/engine/duckdb"
)

// ImportCheckLine is one line of the live half of FR-11's pre-flight: a
// semantic model or View checked against the incoming bundle's own sources in
// a scratch DuckDB session. The static half (alias conflicts, dedupe, paths)
// is bundle.Preflight; this half answers the question Preflight cannot, which
// is "will this View's SQL resolve on this machine".
type ImportCheckLine struct {
	// Name is the model or View name, as the user knows it.
	Name string `json:"name"`
	// ID is the model or View id, so the dialog can bind a decision (skip,
	// pin) to the entity Merge will see.
	ID string `json:"id,omitempty"`
	// Kind separates a saved piece of SQL from a View.
	Kind string `json:"kind"`
	// Level is FR-11's merah/kuning/hijau: merah means the SQL does not
	// resolve here (table or column missing, or the SQL itself is broken),
	// kuning means it resolves but something is off or cannot be proven on
	// this machine, hijau means the schema resolved.
	Level string `json:"level"`
	// Reason explains a merah or kuning, and is written to be read by an
	// analyst who has not seen this code. For a hijau line it carries the
	// described column count.
	Reason string `json:"reason,omitempty"`
}

// ImportLiveCheck mounts the incoming bundle's file sources and semantic
// models in a scratch session — never the open document's — and describes
// every View, so the import screen can mark Views merah/kuning before a single
// byte is written (FR-11). A View that reads a server source is honestly
// reported kuning rather than merah: this build cannot attach it, which is a
// property of the machine, not of the View.
func (s *QueryService) ImportLiveCheck(path string) ([]ImportCheckLine, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.importLiveCheckLocked(path)
}

// importLiveCheckLocked opens the incoming bundle and runs the live half of
// FR-11's pre-flight in a scratch session — never the open document's.
func (s *QueryService) importLiveCheckLocked(path string) ([]ImportCheckLine, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", path, err)
	}
	b, err := bundle.Open(abs)
	if err != nil {
		return nil, err
	}
	return s.liveCheckBundle(b, abs)
}

// liveCheckBundle takes an already-open bundle so ImportInspect can share the
// open it made for Preflight instead of paying for a second one.
func (s *QueryService) liveCheckBundle(b *bundle.Bundle, abs string) ([]ImportCheckLine, error) {
	session, err := s.engine.NewSession(context.Background())
	if err != nil {
		return nil, fmt.Errorf("start a scratch session for %s: %w", filepath.Base(abs), err)
	}
	defer session.Close()

	return checkBundleSources(context.Background(), session, b, abs), nil
}

// checkBundleSources is the whole live check, separated from the locking so
// the tests can reach it with a session of their own. bundlePath is the
// .muara file itself: ResolveFileSources derives the anchor folder from it.
func checkBundleSources(ctx context.Context, session duckdb.Session, b *bundle.Bundle, bundlePath string) []ImportCheckLine {
	resolved, resolveErr := b.ResolveFileSources(bundlePath)

	serverAliases := map[string]bool{}
	for _, c := range b.Connections {
		if c.Driver == bundle.DriverFile || c.Driver == bundle.DriverSQLite {
			var report MountReport
			mountFile(ctx, session, c, resolved, resolveErr, &report)
			continue
		}
		serverAliases[c.Alias] = true
	}

	lines := make([]ImportCheckLine, 0, len(b.SemanticModels)+len(b.Views))
	lines = append(lines, checkModels(ctx, session, b)...)
	for _, v := range b.Views {
		lines = append(lines, checkView(ctx, session, b, v, serverAliases))
	}
	return lines
}

// checkModels mounts each model one at a time, retrying the failures up to
// twice: a model may read a model listed later in the bundle, and one
// retry pass is what turns that ordering mistake back into a green line
// instead of a false merah.
func checkModels(ctx context.Context, session duckdb.Session, b *bundle.Bundle) []ImportCheckLine {
	results := make(map[string]error, len(b.SemanticModels))
	pending := append([]bundle.SemanticModel(nil), b.SemanticModels...)
	for pass := 0; pass < 3 && len(pending) > 0; pass++ {
		var retry []bundle.SemanticModel
		for _, m := range pending {
			err := duckdb.MountSemanticModels(ctx, session,
				[]duckdb.SemanticModel{{ID: m.ID, Name: m.Name, Description: m.Description, RawSQL: m.RawSQL}})
			results[m.ID] = err
			if err != nil {
				retry = append(retry, m)
			}
		}
		pending = retry
	}

	lines := make([]ImportCheckLine, 0, len(b.SemanticModels))
	for _, m := range b.SemanticModels {
		line := ImportCheckLine{Name: m.Name, ID: m.ID, Kind: "semantic_model"}
		if err := results[m.ID]; err != nil {
			line.Level, line.Reason = classifySQLResult(err)
		} else {
			line.Level = "hijau"
		}
		lines = append(lines, line)
	}
	return lines
}

// checkView describes one View's SQL. The guardrail runs first: a View whose
// SQL carries a forbidden pattern could never run in this product, so merah
// is the honest line for it.
func checkView(ctx context.Context, session duckdb.Session, b *bundle.Bundle, v bundle.View, serverAliases map[string]bool) ImportCheckLine {
	line := ImportCheckLine{Name: v.Name, ID: v.ID, Kind: "view"}

	if v.QueryMode == bundle.QuerySynced {
		line.Level = "kuning"
		line.Reason = "SYNCED: builder_state belum bisa dikompilasi di desktop (milik FR-4), jadi skemanya belum bisa divalidasi"
		return line
	}
	if err := duckdb.ValidateQuery(v.RawSQL); err != nil {
		line.Level = "merah"
		line.Reason = fmt.Sprintf("SQL View ini tidak lolos guardrail: %v", err)
		return line
	}
	if alias, ok := serverTouch(b, v, serverAliases); ok {
		line.Level = "kuning"
		line.Reason = fmt.Sprintf("tidak bisa divalidasi di mesin ini: alias %q adalah sumber server, "+
			"dan build ini belum punya jalur attach — koneksikan lewat profil dulu", alias)
		return line
	}

	res, err := session.Query(ctx, "DESCRIBE "+v.RawSQL, duckdb.QueryOptions{MaxRows: duckdb.DefaultMaxRows})
	if err != nil {
		line.Level, line.Reason = classifySQLResult(err)
		return line
	}
	line.Level = "hijau"
	line.Reason = fmt.Sprintf("%d kolom", res.RowCount)
	return line
}

// serverTouch reports whether a View reads — directly or through the models it
// names — any alias that is a server source on the incoming bundle.
func serverTouch(b *bundle.Bundle, v bundle.View, serverAliases map[string]bool) (string, bool) {
	aliases := append([]string(nil), v.Aliases...)
	for _, m := range b.SemanticModels {
		for _, ref := range v.ModelRefs {
			if m.ID == ref {
				aliases = append(aliases, m.Aliases...)
			}
		}
	}
	for _, a := range aliases {
		if serverAliases[a] {
			return a, true
		}
	}
	return "", false
}

// classifySQLResult maps DuckDB's own error families onto FR-11's merah/kuning
// split. The spellings are measured on DuckDB v1.1.3, not guessed: a missing
// table says "Catalog Error", a missing column says "not found in FROM clause"
// or "does not have a column named", a type mismatch says "No function
// matches", and a broken statement says "Parser Error". Everything else is
// kuning on purpose — guessing merah would block an import that might run.
func classifySQLResult(err error) (level, reason string) {
	msg := err.Error()
	for _, marker := range []string{
		"Catalog Error",
		"does not exist",
		"not found in FROM clause",
		"does not have a column named",
		"Parser Error",
	} {
		if strings.Contains(msg, marker) {
			return "merah", msg
		}
	}
	return "kuning", msg
}
