package bundle

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"muara.bi/desktop/internal/secrets"
	"muara.bi/engine/duckdb"
)

// The entries a v1 bundle holds. Everything is JSON at the root of the zip so
// `unzip -p file.muara views.json | grep ...` works without a decoder, which is
// how the "no credential string in the bundle" rule gets checked by hand too.
const (
	entryManifest       = "manifest.json"
	entryProjects       = "projects.json"
	entryConnections    = "connections.json"
	entrySemanticModels = "semantic_models.json"
	entryViews          = "views.json"
	entryDashboards     = "dashboards.json"
	entryAlerts         = "alerts.json"
	entrySecrets        = "secrets.enc"
)

var entityEntries = []string{
	entryProjects,
	entryConnections,
	entrySemanticModels,
	entryViews,
	entryDashboards,
	entryAlerts,
}

// The reasons a file is refused rather than opened. Each carries the numbers it
// compared so the message can say what to do about it instead of failing in a
// way the user has to guess at (BRD §6).
var (
	ErrNotABundle    = errors.New("bundle: not a .muara file")
	ErrUnknownFormat = errors.New("bundle: format this app does not know")
	ErrTooNewFormat  = errors.New("bundle: format newer than this app reads")
	ErrNeedsNewerApp = errors.New("bundle: needs a newer muara.bi Desktop")

	// ErrSealedBlock marks the one case Save refuses on purpose: a bundle that
	// carries a sealed credential block cannot be saved without deciding what
	// happens to it, because silently dropping the block would lose the only
	// copy of credentials that travel with the file (BRD §6).
	ErrSealedBlock = errors.New("bundle: sealed credential block")
)

// Save writes the bundle to path as a zip, atomically: the archive is built in
// a temp file next to the target and renamed over it only once it is complete
// and flushed. A save that fails therefore leaves the previous file untouched —
// which matters because this file is the user's only copy of their settings.
//
// File source paths are canonicalised against the folder of path (§5.5), so a
// file inside the bundle folder is stored relative and travels with it, and a
// file outside stays an absolute machine-local path that says so.
//
// A bundle whose Sealed block is set is refused: keeping the credentials means
// SaveWithSecrets, dropping them means setting Sealed to nil first.
func Save(path string, b *Bundle) error {
	if b.Sealed != nil {
		return fmt.Errorf("%w: this bundle carries an encrypted credential block — save it with "+
			"SaveWithSecrets and the passphrase, or drop the block on purpose first", ErrSealedBlock)
	}
	return save(path, b, nil)
}

// SaveWithSecrets is the opt-in path of BRD §6: the plaintext credentials are
// sealed with the passphrase (argon2id → AES-256-GCM, new code in
// internal/secrets) and written as the eighth entry. The caller's bundle keeps
// the sealed blob afterwards, so a later plain Save refuses instead of silently
// dropping the block.
func SaveWithSecrets(path string, b *Bundle, plaintext []byte, passphrase string) error {
	// Validate before the KDF, which costs hundreds of milliseconds on purpose.
	if err := b.validate(true); err != nil {
		return err
	}
	sealed, err := secrets.Encrypt(plaintext, passphrase)
	if err != nil {
		return err
	}
	if err := save(path, b, sealed); err != nil {
		return err
	}
	b.Sealed = sealed
	return nil
}

// Unseal opens the credential block that Open carried in Sealed. A wrong
// passphrase and a damaged block answer the same error on purpose: GCM cannot
// tell them apart, and pretending it could would send the user to the wrong
// door.
func (b *Bundle) Unseal(passphrase string) ([]byte, error) {
	if b.Sealed == nil {
		return nil, fmt.Errorf("%w: this bundle carries no encrypted credential block", ErrSealedBlock)
	}
	return secrets.Decrypt(b.Sealed, passphrase)
}

func save(path string, b *Bundle, sealed []byte) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("%w: no file name given", ErrNotABundle)
	}
	// The app that owns the data must not be able to store a broken definition.
	if err := b.validate(true); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	anchor, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("bundle: %s: %w", dir, err)
	}
	out, err := b.forAnchor(anchor)
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".muara-save-*.tmp")
	if err != nil {
		return fmt.Errorf("bundle: cannot save into %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if err := writeBundle(tmp, out, sealed, out.Manifest.CreatedAt); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("bundle: flush %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("bundle: close %s: %w", tmpName, err)
	}
	// No directory fsync: the rename is what makes the save atomic for the app,
	// and Windows has no portable way to fsync a directory handle.
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("bundle: replacing %s: %w", path, err)
	}
	return nil
}

func writeBundle(w io.Writer, b *Bundle, sealed []byte, stamp time.Time) error {
	zw := zip.NewWriter(w)
	put := func(name string, payload any) error {
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return fmt.Errorf("bundle: encode %s: %w", name, err)
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		// One timestamp for the whole archive keeps a save of the same content
		// byte-identical, which is what an import needs before it can dedupe a
		// bundle that was handed over twice (M4).
		header.Modified = stamp
		entry, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("bundle: create %s: %w", name, err)
		}
		if _, err := entry.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("bundle: write %s: %w", name, err)
		}
		return nil
	}

	if err := put(entryManifest, b.Manifest); err != nil {
		return err
	}
	// Each entity type gets its own entry even when empty, so the entry list of
	// a bundle without a sealed block is always the same seven names; a sealed
	// bundle appends secrets.enc after them.
	for _, spec := range []struct {
		name    string
		payload any
	}{
		{entryProjects, emptyIfNil(b.Projects)},
		{entryConnections, emptyIfNil(b.Connections)},
		{entrySemanticModels, emptyIfNil(b.SemanticModels)},
		{entryViews, emptyIfNil(b.Views)},
		{entryDashboards, emptyIfNil(b.Dashboards)},
		{entryAlerts, emptyIfNil(b.Alerts)},
	} {
		if err := put(spec.name, spec.payload); err != nil {
			return err
		}
	}
	if sealed != nil {
		header := &zip.FileHeader{Name: entrySecrets, Method: zip.Deflate}
		header.Modified = stamp
		entry, err := zw.CreateHeader(header)
		if err != nil {
			return fmt.Errorf("bundle: create %s: %w", entrySecrets, err)
		}
		if _, err := entry.Write(sealed); err != nil {
			return fmt.Errorf("bundle: write %s: %w", entrySecrets, err)
		}
	}
	return zw.Close()
}

// Open reads a bundle. The manifest is checked before any entity entry is
// decoded, so a file from a newer layout is refused with the version numbers
// and what to do about them, rather than half-imported and half-wrong.
//
// A bundle whose Views read an alias this machine does not have still opens:
// the missing alias is something to report and re-point at import time (§5.4),
// not a reason to refuse the whole file.
func Open(path string) (*Bundle, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("bundle: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%w: %s is a folder, not a bundle", ErrNotABundle, path)
	}
	reader, err := zip.NewReader(file, info.Size())
	if err != nil {
		return nil, fmt.Errorf("%w %s: %v", ErrNotABundle, path, err)
	}

	entries := map[string]*zip.File{}
	for _, entry := range reader.File {
		if _, seen := entries[entry.Name]; seen {
			return nil, fmt.Errorf("%w: %s lists %q twice", ErrNotABundle, path, entry.Name)
		}
		entries[entry.Name] = entry
	}

	manifestEntry, ok := entries[entryManifest]
	if !ok {
		return nil, fmt.Errorf("%w: %s has no %s", ErrNotABundle, path, entryManifest)
	}
	var manifest Manifest
	if err := decodeEntry(manifestEntry, &manifest); err != nil {
		return nil, err
	}
	if err := manifest.check(); err != nil {
		return nil, err
	}

	var sealed []byte
	for name := range entries {
		switch name {
		case entryManifest:
		case entrySecrets:
			// The block is carried, not opened: unsealing needs the passphrase,
			// which is Unseal's business. Only its envelope is checked here, so a
			// bundle whose block this app cannot read at all is refused with a
			// reason instead of carried broken.
			raw, err := readEntry(entries[name])
			if err != nil {
				return nil, err
			}
			if err := secrets.Check(raw); err != nil {
				return nil, fmt.Errorf("%w: %s holds %s, which this app cannot read: %v",
					ErrUnknownFormat, path, entrySecrets, err)
			}
			sealed = raw
		default:
			if !isEntityEntry(name) {
				return nil, fmt.Errorf("%w: %s holds %s, which this format does not define",
					ErrUnknownFormat, path, name)
			}
		}
	}

	b := &Bundle{Manifest: manifest, Sealed: sealed}
	for _, name := range entityEntries {
		entry, ok := entries[name]
		if !ok {
			return nil, fmt.Errorf("%w: %s is missing %s", ErrNotABundle, path, name)
		}
		switch name {
		case entryProjects:
			err = decodeEntry(entry, &b.Projects)
		case entryConnections:
			err = decodeEntry(entry, &b.Connections)
		case entrySemanticModels:
			err = decodeEntry(entry, &b.SemanticModels)
		case entryViews:
			err = decodeEntry(entry, &b.Views)
		case entryDashboards:
			err = decodeEntry(entry, &b.Dashboards)
		case entryAlerts:
			err = decodeEntry(entry, &b.Alerts)
		}
		if err != nil {
			// decodeEntry already names the entry that failed.
			return nil, err
		}
	}

	// Reading is tolerant about references, never about shape: an entity with no
	// id or a View of an unknown kind could not be drawn at all.
	if err := b.validate(false); err != nil {
		return nil, err
	}
	if _, err := b.ResolveFileSources(path); err != nil {
		return nil, err
	}
	return b, nil
}

func isEntityEntry(name string) bool {
	for _, entry := range entityEntries {
		if entry == name {
			return true
		}
	}
	return false
}

func readEntry(entry *zip.File) ([]byte, error) {
	reader, err := entry.Open()
	if err != nil {
		return nil, fmt.Errorf("bundle: reading %s: %w", entry.Name, err)
	}
	defer reader.Close()
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("bundle: reading %s: %w", entry.Name, err)
	}
	return data, nil
}

func decodeEntry[T any](entry *zip.File, out *T) error {
	data, err := readEntry(entry)
	if err != nil {
		// readEntry already names the entry that failed.
		return err
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("bundle: %s is not valid JSON: %w", entry.Name, err)
	}
	return nil
}

// emptyIfNil keeps an entity entry a JSON array even when nothing is stored, so
// `views.json` of an empty bundle reads [] and not null.
func emptyIfNil[S ~[]E, E any](s S) any {
	if s == nil {
		return []E{}
	}
	return s
}

// manifestForWrite fills in what the app knows, leaving a pinned CreatedAt
// alone: two saves of the same content must produce the same bytes.
func (b *Bundle) manifestForWrite() Manifest {
	m := b.Manifest
	m.Format = Format
	m.Version = FormatVersion
	if m.MinAppVersion == "" {
		m.MinAppVersion = AppVersion
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.App = AppInfo{Name: AppName, Version: AppVersion}
	return m
}

// forAnchor returns the bundle as it should be written, with every file source
// path put into its canonical form relative to the folder of the target file.
// The caller's bundle keeps the paths it was given.
func (b *Bundle) forAnchor(anchor string) (*Bundle, error) {
	out := *b
	out.Manifest = b.manifestForWrite()
	out.Connections = make([]Connection, len(b.Connections))
	copy(out.Connections, b.Connections)
	for i, c := range out.Connections {
		if c.File == nil {
			continue
		}
		stored, err := duckdb.CanonicalFileSourcePath(c.File.Path, anchor)
		if err != nil {
			return nil, fmt.Errorf("bundle: connection %q: %w", c.Alias, err)
		}
		file := *c.File
		file.Path = stored
		out.Connections[i].File = &file
	}
	return &out, nil
}

// ResolveFileSources maps the alias of every connection that reads a file —
// `file` and `sqlite` — to a path this machine can read, resolved against the
// folder of bundlePath. It is the other half of §5.5: paths travel relative and
// are re-read here, so moving a bundle to another folder re-points every source
// at once.
//
// A path that resolves but names a missing file is not an error at this layer —
// pre-flight has to be able to report it as one of the three fates rather than
// lose the rest of the bundle.
func (b *Bundle) ResolveFileSources(bundlePath string) (map[string]string, error) {
	anchor, err := filepath.Abs(filepath.Dir(bundlePath))
	if err != nil {
		return nil, fmt.Errorf("bundle: %s: %w", bundlePath, err)
	}
	resolved := map[string]string{}
	for _, c := range b.Connections {
		if c.File == nil {
			continue
		}
		path, err := duckdb.ResolveFileSourcePath(c.File.Path, anchor)
		if err != nil {
			return nil, fmt.Errorf("bundle: connection %q stores %q: %w", c.Alias, c.File.Path, err)
		}
		resolved[c.Alias] = path
	}
	return resolved, nil
}

// check is the gate in front of every other entry.
func (m Manifest) check() error {
	if strings.TrimSpace(m.Format) == "" {
		return fmt.Errorf("%w: manifest has no format", ErrNotABundle)
	}
	if m.Format != Format {
		return fmt.Errorf("%w: %q — this app reads %q only", ErrUnknownFormat, m.Format, Format)
	}
	if m.Version != FormatVersion {
		if m.Version > FormatVersion {
			return fmt.Errorf("%w: format version %d, this app understands up to %d — update %s to open it",
				ErrTooNewFormat, m.Version, FormatVersion, AppName)
		}
		return fmt.Errorf("%w: format version %d, this app reads version %d",
			ErrUnknownFormat, m.Version, FormatVersion)
	}
	if strings.TrimSpace(m.MinAppVersion) == "" {
		return fmt.Errorf("%w: manifest has no min_app_version, so the safest version this bundle needs is unknown",
			ErrNotABundle)
	}
	if compareDotted(AppVersion, m.MinAppVersion) < 0 {
		return fmt.Errorf("%w: this bundle needs %s %s or newer, this app is %s — update the app before opening it",
			ErrNeedsNewerApp, AppName, m.MinAppVersion, AppVersion)
	}
	return nil
}

// parseDotted reads up to three numeric components of a version string. A
// pre-release suffix is ignored on purpose: "0.2.0-rc1" must still be able to
// read a bundle whose min_app_version says "0.2.0".
func parseDotted(s string) ([3]int, error) {
	var parts [3]int
	head := s
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		head = s[:i]
	}
	fields := strings.Split(head, ".")
	if len(fields) > len(parts) {
		return parts, fmt.Errorf("version %q has more than %d parts", s, len(parts))
	}
	for i, field := range fields {
		n, err := strconv.Atoi(field)
		if err != nil || n < 0 {
			return parts, fmt.Errorf("version %q is not numeric", s)
		}
		parts[i] = n
	}
	return parts, nil
}

func compareDotted(a, b string) int {
	pa, errA := parseDotted(a)
	pb, errB := parseDotted(b)
	// An unreadable version cannot be trusted to be satisfied by anything, so it
	// sorts lowest and the caller refuses the bundle.
	if errA != nil {
		if errB != nil {
			return 0
		}
		return -1
	}
	if errB != nil {
		return 1
	}
	for i := range pa {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}
