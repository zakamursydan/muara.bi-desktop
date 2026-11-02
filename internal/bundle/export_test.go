package bundle_test

import (
	"errors"
	"path/filepath"
	"testing"

	"muara.bi/desktop/internal/bundle"
)

// TestWithoutServerDetailsKeepsTierOneAndDropsTierTwo is the three-tier rule of
// §5.4 as code: tier 1 always travels, tier 2 travels only when the export
// says so, and the caller's bundle is not the thing being modified.
func TestWithoutServerDetailsKeepsTierOneAndDropsTierTwo(t *testing.T) {
	dir := t.TempDir()
	original := fullBundle(dir)

	stripped := original.WithoutServerDetails()

	// Tier 1 survives: alias, driver, database, schema filter, note.
	c1 := stripped.Connections[0]
	if c1.Alias != "sales" || c1.Driver != bundle.DriverPostgres || c1.Database != "dwh" ||
		c1.Note != "gudang utama" || len(c1.SchemaFilter) != 1 || c1.SchemaFilter[0] != "public" {
		t.Errorf("tier 1 changed: %+v", c1)
	}
	// Tier 2 is dropped as one decision — address and ssh block together.
	if c1.Address != nil || c1.SSH != nil {
		t.Errorf("tier 2 survived: address=%+v ssh=%+v", c1.Address, c1.SSH)
	}

	// A file source's path is not tier 2: without it the source does not exist.
	if stripped.Connections[1].File == nil || stripped.Connections[1].File.Path == "" {
		t.Errorf("the file source lost its path: %+v", stripped.Connections[1])
	}

	// The caller's bundle keeps what it was given.
	if original.Connections[0].Address == nil || original.Connections[0].SSH == nil {
		t.Error("stripping modified the original bundle")
	}

	// What is left is a complete bundle: it saves, opens, and the reopened
	// copy still carries no tier 2.
	path := writeBundleFile(t, dir, stripped)
	got, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, c := range got.Connections {
		if c.Address != nil || c.SSH != nil {
			t.Errorf("reopened connection %q still carries tier 2", c.Alias)
		}
	}
}

// The sealed block is tier 3: stripping tier 2 must not silently decide what
// happens to it — Save still refuses, and Unseal still works.
func TestWithoutServerDetailsKeepsTheSealedBlock(t *testing.T) {
	dir := t.TempDir()
	plaintext := []byte(`{"sales":{"user":"analyst","db_password":"s3kret"}}`)
	path := filepath.Join(dir, "sealed.muara")
	if err := bundle.SaveWithSecrets(path, fullBundle(dir), plaintext, "kata sandi"); err != nil {
		t.Fatalf("save: %v", err)
	}
	opened, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}

	stripped := opened.WithoutServerDetails()
	if stripped.Sealed == nil {
		t.Fatal("stripping tier 2 dropped the sealed block")
	}
	if err := bundle.Save(filepath.Join(dir, "plain.muara"), stripped); !errors.Is(err, bundle.ErrSealedBlock) {
		t.Errorf("plain save error = %v, want ErrSealedBlock", err)
	}
	got, err := stripped.Unseal("kata sandi")
	if err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if string(got) != string(plaintext) {
		t.Errorf("unsealed = %q, want %q", got, plaintext)
	}
}

// The two decisions compose: an export with the address toggle off and the
// credential toggle on produces a bundle with no tier 2 and a working block.
func TestStrippedBundleWithSecretsRoundTrips(t *testing.T) {
	dir := t.TempDir()
	plaintext := []byte(`{"sales":{"user":"analyst","db_password":"s3kret"}}`)
	stripped := fullBundle(dir).WithoutServerDetails()

	path := filepath.Join(dir, "export.muara")
	if err := bundle.SaveWithSecrets(path, stripped, plaintext, "kata sandi"); err != nil {
		t.Fatalf("save: %v", err)
	}

	got, err := bundle.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for _, c := range got.Connections {
		if c.Address != nil || c.SSH != nil {
			t.Errorf("reopened connection %q still carries tier 2", c.Alias)
		}
	}
	opened, err := got.Unseal("kata sandi")
	if err != nil {
		t.Fatalf("unseal: %v", err)
	}
	if string(opened) != string(plaintext) {
		t.Errorf("unsealed = %q, want %q", opened, plaintext)
	}
}
