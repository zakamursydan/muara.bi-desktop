// Package profile is this machine's half of a data source.
//
// A `.muara` bundle says which *alias* a query reads from and, when the user
// chose to include tier 2 of BRD §5.4, which server that alias meant on the
// machine that exported it. The same bundle has to work on a laptop that reaches
// that server over a VPN, on a colleague's machine whose alias points at their
// own box, and on this machine — so the answer to "where does alias X actually
// live *here*" cannot live in the bundle. It lives in one file per machine, and
// this package is the only reader of it.
//
// The rule that makes that safe is the same one the bundle is built on: a
// password, token or key has no field on any type here. What is stored is where
// to *look* for the secret on this machine, not the secret. That is why this file
// can sit in the user's config folder without becoming a credential dump, and why
// "grep the profile, find no secret" is a property of the struct rather than of a
// scrub step.
package profile

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"muara.bi/engine/sqlident"
)

// AppFolder is the per-user folder this app keeps its machine-local state in
// (the profile today; the keychain seam and the settings file land in the same
// place). It is deliberately not the folder of a bundle: nothing here is
// exportable.
const AppFolder = "muara.bi"

// FileName is the profile's file name inside AppFolder.
const FileName = "profile.json"

// ErrInvalid is returned for a profile this machine cannot use.
var ErrInvalid = errors.New("profile: invalid entry")

// ErrNoSecretSource says a connection needs a credential and this profile does
// not say where to get one. It is an answer, not a crash: the mount report shows
// it so the user can fill it in instead of watching a query fail.
var ErrNoSecretSource = errors.New("profile: no credential source for this machine")

// SecretKind names the mechanism a credential is read through on this machine.
type SecretKind string

const (
	// SecretKeychain is the OS credential store — Windows Credential Manager,
	// the macOS Keychain, libsecret — which FR-15/M6 wires up. Naming it here is
	// what lets the profile be written and validated *before* the reader exists.
	SecretKeychain SecretKind = "keychain"

	// SecretEnv reads the value from a named environment variable. It exists for
	// the spike and for CI, where a machine has a shell but no keychain entry.
	SecretEnv SecretKind = "env"
)

// SecretRef points at a credential instead of holding it. Label is the keychain
// account name, or the environment variable's name.
type SecretRef struct {
	Kind  SecretKind `json:"kind"`
	Label string     `json:"label"`
}

// Target is one alias as this machine reads it.
type Target struct {
	Alias string `json:"alias"`

	Host string `json:"host,omitempty"`
	Port int    `json:"port,omitempty"`
	User string `json:"user,omitempty"`

	// SSLMode is libpq's word and mysql_scanner's word for the same column in a
	// bundle, so it is kept under the name the engines use.
	SSLMode string `json:"ssl_mode,omitempty"`

	Secret *SecretRef `json:"secret,omitempty"`
	Note   string     `json:"note,omitempty"`
}

// Profile is one machine's alias map. It is loaded, never merged into a bundle,
// and never written by Import: a bundle may name an alias this machine has no
// target for, and that is a pre-flight finding (§5.4), not a broken profile.
type Profile struct {
	// Machine is a label for the settings screen ("laptop-ani"); the hostname is
	// written here once so a user can tell three profiles apart after syncing
	// their folder.
	Machine string `json:"machine,omitempty"`

	Targets []Target `json:"targets"`
}

// DefaultPath is where this machine's profile lives.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("profile: find the per-user config folder: %w", err)
	}
	return filepath.Join(dir, AppFolder, FileName), nil
}

// Look returns the target for an alias, if this machine has one.
func (p *Profile) Look(alias string) (Target, bool) {
	if p == nil {
		return Target{}, false
	}
	for _, t := range p.Targets {
		if t.Alias == alias {
			return t, true
		}
	}
	return Target{}, false
}

// Secret resolves the credential for one alias through the source the profile
// names. The value never leaves this function's caller as text stored anywhere:
// it is handed to a DSN and dropped.
//
// A keychain reference is refused rather than answered with an empty password:
// the reader is M6 work, and attaching with "" would surface as "password
// authentication failed" — a message that points the user at the wrong door.
func (p *Profile) Secret(alias string) (string, error) {
	target, ok := p.Look(alias)
	if !ok {
		return "", fmt.Errorf("%w: %q has no target on this machine", ErrNoSecretSource, alias)
	}
	if target.Secret == nil {
		return "", fmt.Errorf("%w: %q names no credential source — fill it in under Settings › Profil",
			ErrNoSecretSource, alias)
	}
	ref := *target.Secret
	label := strings.TrimSpace(ref.Label)
	if label == "" {
		return "", fmt.Errorf("%w: %q points at a %s with no name", ErrNoSecretSource, alias, ref.Kind)
	}
	switch ref.Kind {
	case SecretEnv:
		value := os.Getenv(label)
		if value == "" {
			return "", fmt.Errorf("%w: %q reads $%s and it is empty", ErrNoSecretSource, alias, label)
		}
		return value, nil
	case SecretKeychain:
		return "", fmt.Errorf("%w: %q lives in this machine's keychain (%s), which this build cannot read yet",
			ErrNoSecretSource, alias, label)
	default:
		return "", fmt.Errorf("%w: %q names credential kind %q, want keychain or env",
			ErrNoSecretSource, alias, ref.Kind)
	}
}

// validate enforces what the mount code relies on: alias is the join key between
// a bundle and this machine, so it must be plain and unique, and a port outside
// the wire's range would only be caught by the connector as a failed connection.
func (p *Profile) validate() error {
	seen := map[string]bool{}
	for _, t := range p.Targets {
		alias := strings.TrimSpace(t.Alias)
		if alias == "" {
			return fmt.Errorf("%w: a target has no alias", ErrInvalid)
		}
		if !sqlident.IsPlain(alias) {
			return fmt.Errorf("%w: alias %q is not a plain identifier — it is the name a query reads tables through",
				ErrInvalid, t.Alias)
		}
		if seen[alias] {
			return fmt.Errorf("%w: duplicate alias %q", ErrInvalid, alias)
		}
		seen[alias] = true
		if t.Port < 0 || t.Port > 65535 {
			return fmt.Errorf("%w: alias %q has port %d, outside 0-65535", ErrInvalid, alias, t.Port)
		}
		if t.Secret == nil {
			continue
		}
		switch t.Secret.Kind {
		case SecretKeychain, SecretEnv:
		case "":
			return fmt.Errorf("%w: alias %q names a credential with no kind — keychain or env", ErrInvalid, alias)
		default:
			return fmt.Errorf("%w: alias %q names credential kind %q, want keychain or env", ErrInvalid, alias, t.Secret.Kind)
		}
		if strings.TrimSpace(t.Secret.Label) == "" {
			return fmt.Errorf("%w: alias %q names a %s credential with no name", ErrInvalid, alias, t.Secret.Kind)
		}
	}
	return nil
}

// Load reads this machine's profile. A file that is not there yet is an empty
// profile, not an error: a first run has no alias map, and the app must still
// open bundles whose sources are files.
func Load(path string) (*Profile, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Profile{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("profile: %s: %w", path, err)
	}
	var p Profile
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("profile: %s: %w", path, err)
	}
	if err := p.validate(); err != nil {
		return nil, fmt.Errorf("profile: %s: %w", path, err)
	}
	return &p, nil
}

// Save writes the profile with the same atomicity rule as a bundle: the file is
// replaced by a rename from a fully-written temporary in the same folder, so a
// crash mid-write cannot leave this machine without its alias map.
func Save(path string, p *Profile) error {
	if p == nil {
		return fmt.Errorf("%w: nothing to write", ErrInvalid)
	}
	if err := p.validate(); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("profile: create %s: %w", dir, err)
	}
	raw, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("profile: encode: %w", err)
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(dir, ".profile-*.tmp")
	if err != nil {
		return fmt.Errorf("profile: create temporary in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return fmt.Errorf("profile: write %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("profile: sync %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("profile: close %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("profile: replace %s: %w", path, err)
	}
	return nil
}
