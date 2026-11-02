package bundle_test

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"muara.bi/desktop/internal/bundle"
)

// The words a credential is called by in this product's connectors.
var credentialWords = []string{
	"password", "passwd", "token", "secret", "credential", "api_key", "apikey", "dsn", "sasl", "keyfile", "private_key",
}

// TestNoStoredTypeCanHoldACredential is the structural half of the M3 exit
// criterion: the grep finds no credential string because no field could carry
// one, not because a scrub step remembered to remove it.
func TestNoStoredTypeCanHoldACredential(t *testing.T) {
	names := map[string][]string{}
	for _, typ := range []reflect.Type{
		reflect.TypeOf(bundle.Bundle{}),
		reflect.TypeOf(bundle.Manifest{}),
		reflect.TypeOf(bundle.Connection{}),
		reflect.TypeOf(bundle.Address{}),
		reflect.TypeOf(bundle.SSHTarget{}),
		reflect.TypeOf(bundle.FileSource{}),
		reflect.TypeOf(bundle.Project{}),
		reflect.TypeOf(bundle.SemanticModel{}),
		reflect.TypeOf(bundle.View{}),
		reflect.TypeOf(bundle.Dashboard{}),
		reflect.TypeOf(bundle.Alert{}),
	} {
		names[typ.Name()] = jsonFieldNames(typ)
	}

	for typeName, fields := range names {
		for _, field := range fields {
			lowered := strings.ToLower(field)
			for _, word := range credentialWords {
				if strings.Contains(lowered, word) {
					t.Errorf("%s has field %q, which names a credential — §5.4 tier 3 lives in the keychain, not the bundle",
						typeName, field)
				}
			}
		}
	}
}

// jsonFieldNames collects the JSON key of every field a type can write, walking
// into nested structs so an address or ssh block cannot hide one.
func jsonFieldNames(typ reflect.Type) []string {
	var out []string
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(typ reflect.Type) {
		for typ.Kind() == reflect.Pointer || typ.Kind() == reflect.Slice || typ.Kind() == reflect.Array {
			typ = typ.Elem()
		}
		if typ.Kind() != reflect.Struct || seen[typ] {
			return
		}
		seen[typ] = true
		for i := range typ.NumField() {
			field := typ.Field(i)
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "" || name == "-" {
				name = field.Name
			}
			out = append(out, name)
			walk(field.Type)
		}
	}
	walk(typ)
	return out
}

// TestBundleBytesCarryNoCredentialString is the grep the exit criterion asks
// for, done on the decompressed entries: a deflate-compressed zip would "find"
// nothing whatever the bundle held, so searching the archive itself proves
// nothing.
func TestBundleBytesCarryNoCredentialString(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(t, dir, fullBundle(dir))

	for _, name := range zipEntries(t, path) {
		body := string(zipEntry(t, path, name))
		lowered := strings.ToLower(body)
		for _, word := range credentialWords {
			if strings.Contains(lowered, word) {
				t.Errorf("%s contains %q:\n%s", name, word, body)
			}
		}
	}
}

// TestBundleKeysAreExactlyTheDocumentedSet keeps the format honest: adding a
// field to a stored type is a format change and has to be written down in BRD
// §6 before it can be saved.
func TestBundleKeysAreExactlyTheDocumentedSet(t *testing.T) {
	dir := t.TempDir()
	path := writeBundleFile(t, dir, fullBundle(dir))

	want := map[string][]string{
		"manifest.json":        {"app", "created_at", "exported_by", "format", "min_app_version", "origin_workspace_id", "version"},
		"projects.json":        {"color", "dashboard_refs", "id", "name", "note", "view_pins"},
		"connections.json":     {"address", "alias", "database", "driver", "file", "id", "note", "schema_filter", "ssh_config"},
		"semantic_models.json": {"aliases", "description", "id", "name", "raw_sql"},
		"views.json":           {"aliases", "builder_state", "cache_ttl_seconds", "folder", "id", "kind", "max_rows", "name", "presentation", "query_mode", "raw_sql"},
		"dashboards.json":      {"global_filters", "id", "layout", "name", "refresh_interval_seconds", "view_refs"},
		"alerts.json":          {"channel", "id", "schedule", "threshold", "view_id"},
	}

	for name, expected := range want {
		body := zipEntry(t, path, name)
		keys := map[string]bool{}
		collectKeys(body, keys)
		got := sortedKeys(keys)
		if !reflect.DeepEqual(got, expected) {
			t.Errorf("%s keys = %v, want the documented %v", name, got, expected)
		}
	}
}

func collectKeys(data []byte, into map[string]bool) {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return
	}
	switch value := doc.(type) {
	case []any:
		for _, item := range value {
			collectKeys(mustJSON(item), into)
		}
	case map[string]any:
		for key := range value {
			into[key] = true
		}
	}
}

func mustJSON(v any) []byte {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return data
}

func sortedKeys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for key := range set {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
