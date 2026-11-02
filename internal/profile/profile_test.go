package profile

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// credentialWords is the same list the bundle store is checked against, so the
// two machine-local halves of the product are held to one rule.
var credentialWords = []string{
	"password", "passwd", "token", "secret", "credential",
	"api_key", "apikey", "dsn", "sasl", "keyfile", "private_key",
}

func TestNoProfileTypeHasACredentialField(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(Profile{}),
		reflect.TypeOf(Target{}),
		reflect.TypeOf(SecretRef{}),
	} {
		if typ.Kind() != reflect.Struct {
			t.Fatalf("%s is not a struct", typ)
		}
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			// The rule is about the value, not the vocabulary: a field that points
			// at where a credential lives is the design, a string that could hold
			// the credential itself is the hole. SecretRef.Kind and .Label are
			// names, and Target.Secret is a pointer to a struct.
			if field.Type.Kind() != reflect.String {
				continue
			}
			name := strings.ToLower(field.Name)
			tag := strings.ToLower(field.Tag.Get("json"))
			for _, word := range credentialWords {
				if strings.Contains(name, word) || strings.Contains(tag, word) {
					t.Errorf("%s.%s (%q) can hold a credential string", typ, field.Name, tag)
				}
			}
		}
	}
}

func TestLookAndSecretResolveOnlyWhatThisMachineNames(t *testing.T) {
	t.Setenv("MUARA_TEST_SALES_PW", "hunter2")

	p := &Profile{
		Machine: "laptop-uni",
		Targets: []Target{
			{Alias: "sales", Host: "10.0.0.5", Port: 5432, User: "analyst",
				SSLMode: "require", Secret: &SecretRef{Kind: SecretEnv, Label: "MUARA_TEST_SALES_PW"}},
			{Alias: "staging", Host: "db.staging.internal", Secret: &SecretRef{Kind: SecretKeychain, Label: "muara.staging"}},
			{Alias: "nosecret", Host: "localhost"},
		},
	}
	if err := p.validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}

	target, ok := p.Look("sales")
	if !ok || target.Host != "10.0.0.5" || target.Port != 5432 {
		t.Errorf("Look(sales) = %+v, %v; want the 10.0.0.5:5432 target", target, ok)
	}
	if _, ok := p.Look("absent"); ok {
		t.Error("Look(absent) reported a target this profile does not have")
	}

	value, err := p.Secret("sales")
	if err != nil || value != "hunter2" {
		t.Errorf("Secret(sales) = %q, %v; want the environment's value", value, err)
	}

	// The keychain seam must refuse by name, never answer with an empty password:
	// "" would reach the server and come back as "password authentication
	// failed", which points the user at the wrong door.
	if _, err := p.Secret("staging"); err == nil ||
		!strings.Contains(err.Error(), "keychain") || !strings.Contains(err.Error(), "muara.staging") {
		t.Errorf("Secret(staging) error = %v, want the keychain label named", err)
	}
	if _, err := p.Secret("nosecret"); !errors.Is(err, ErrNoSecretSource) {
		t.Errorf("Secret(nosecret) error = %v, want ErrNoSecretSource", err)
	}
	if _, err := p.Secret("absent"); !errors.Is(err, ErrNoSecretSource) {
		t.Errorf("Secret(absent) error = %v, want ErrNoSecretSource", err)
	}
}

func TestSecretRefusesAnEmptyEnvironmentValue(t *testing.T) {
	t.Setenv("MUARA_TEST_EMPTY_PW", "")
	p := &Profile{Targets: []Target{{Alias: "sales", Secret: &SecretRef{Kind: SecretEnv, Label: "MUARA_TEST_EMPTY_PW"}}}}
	_, err := p.Secret("sales")
	if !errors.Is(err, ErrNoSecretSource) {
		t.Fatalf("Secret on an empty $VAR error = %v, want ErrNoSecretSource", err)
	}
	if !strings.Contains(err.Error(), "MUARA_TEST_EMPTY_PW") {
		t.Errorf("error %q does not name the variable to fill in", err)
	}
}

func TestSaveThenLoadKeepsEveryTarget(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	want := &Profile{
		Machine: "laptop-uni",
		Targets: []Target{
			{Alias: "sales", Host: "10.0.0.5", Port: 5432, User: "analyst", SSLMode: "verify-full",
				Secret: &SecretRef{Kind: SecretKeychain, Label: "muara.sales"}, Note: "lewat VPN kantor"},
			{Alias: "orders_file"},
		},
	}
	if err := Save(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Machine != want.Machine || !reflect.DeepEqual(got.Targets, want.Targets) {
		t.Errorf("round trip = %+v, want %+v", got, want)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	for _, word := range credentialWords {
		if strings.Contains(strings.ToLower(string(raw)), word+"=") {
			t.Errorf("profile file carries %q= after a round trip", word)
		}
	}
}

func TestLoadOfNoProfileYetIsAnEmptyOne(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	p, err := Load(path)
	if err != nil {
		t.Fatalf("a first run with no profile must not be an error: %v", err)
	}
	if len(p.Targets) != 0 {
		t.Errorf("fresh profile carries %d targets, want none", len(p.Targets))
	}
	if _, err := p.Secret("sales"); !errors.Is(err, ErrNoSecretSource) {
		t.Errorf("Secret on an empty profile = %v, want ErrNoSecretSource", err)
	}
}

func TestBrokenProfilesAreRefusedBeforeTheyReachDisk(t *testing.T) {
	cases := []struct {
		name    string
		p       *Profile
		wantsub string
	}{
		{"alias that is not an identifier",
			&Profile{Targets: []Target{{Alias: "sales prod"}}}, "plain identifier"},
		{"the same alias twice",
			&Profile{Targets: []Target{{Alias: "sales"}, {Alias: "sales", Host: "other"}}}, "duplicate alias"},
		{"port above the wire's range",
			&Profile{Targets: []Target{{Alias: "sales", Port: 70000}}}, "outside 0-65535"},
		{"credential with an unknown kind",
			&Profile{Targets: []Target{{Alias: "sales", Secret: &SecretRef{Kind: "vault", Label: "x"}}}}, "credential kind"},
		{"credential with no name",
			&Profile{Targets: []Target{{Alias: "sales", Secret: &SecretRef{Kind: SecretKeychain}}}}, "no name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), FileName)
			err := Save(path, tc.p)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("Save error = %v, want ErrInvalid", err)
			}
			if !strings.Contains(err.Error(), tc.wantsub) {
				t.Errorf("error %q does not say %q", err, tc.wantsub)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a refused save still left a profile file behind")
			}
		})
	}
}

func TestLoadRefusesAFileItCannotTrust(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)

	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Error("a malformed profile loaded without an error")
	}

	if err := os.WriteFile(path, []byte(`{"targets":[{"alias":"1 sales"}]}`), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := Load(path); !errors.Is(err, ErrInvalid) {
		t.Errorf("Load error = %v, want ErrInvalid for an alias a query cannot use", err)
	}
}

func TestDefaultPathIsOutsideAnyBundle(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("default path: %v", err)
	}
	if filepath.Base(path) != FileName {
		t.Errorf("DefaultPath() = %q, want it to end in %s", path, FileName)
	}
	if filepath.Base(filepath.Dir(path)) != AppFolder {
		t.Errorf("DefaultPath() = %q, want it inside the %s folder", path, AppFolder)
	}
	if strings.Contains(path, ".muara") {
		t.Errorf("DefaultPath() = %q — machine-local state must not live in a bundle", path)
	}
}
