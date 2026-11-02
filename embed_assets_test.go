//go:build cgo

package main

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// assetRef finds the hashed bundle Vite links from the page it generated.
var assetRef = regexp.MustCompile(`(?:src|href)="/(assets/[^"]+)"`)

// TestEmbeddedViteBuildIsServed proves the shipped UI is the Vite output and
// that the Go binary serves it without a dev server: go:embed has no network
// path, so anything not in the embedded tree cannot appear at runtime.
func TestEmbeddedViteBuildIsServed(t *testing.T) {
	handler := application.BundledAssetFileServer(bundledAssets())

	page := get(t, handler, "/")
	if page.Code != 200 {
		t.Fatalf("GET / = %d, want 200 (body %q)", page.Code, page.Body.String())
	}
	body := page.Body.String()
	if !strings.Contains(body, `id="root"`) {
		t.Fatalf("served page is not the Vite build: %q", body)
	}
	if strings.Contains(body, "wails/runtime.js") {
		t.Fatal("the React build imports the runtime from npm and bundles it, so the page must not depend on /wails/runtime.js")
	}

	refs := assetRef.FindAllStringSubmatch(body, -1)
	if len(refs) < 2 {
		t.Fatalf("expected the page to link a JS and a CSS bundle, got %d refs in %q", len(refs), body)
	}

	var jsSeen []string
	for _, ref := range refs {
		asset := get(t, handler, "/"+ref[1])
		if asset.Code != 200 {
			t.Fatalf("GET /%s = %d; a bundle the page links must be embedded", ref[1], asset.Code)
		}
		if strings.HasSuffix(ref[1], ".js") {
			jsSeen = append(jsSeen, asset.Body.String())
		}
	}

	// duckdbVersion is the engine's own JSON tag, carried verbatim through the
	// generated binding model into this bundle. Its presence proves the webview
	// can deserialize what QueryService.Status() returns.
	joined := strings.Join(jsSeen, "")
	if !strings.Contains(joined, "duckdbVersion") {
		t.Fatal("the JS bundle does not contain the generated binding model; the page cannot call the engine")
	}
}

func get(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec
}

// TestDocumentControlsAreInShippedBundle keeps "the app works on one open file"
// honest at the only layer that is not testable from Go: the window the user
// actually clicks. A bundle that can be opened, saved and reported in
// `document.go` but never reaches the embedded JS is still a spike.
//
// The assertions are on strings the UI itself writes. Generated bindings are not
// named in the bundle — `$Call.ByID(<hash>)` replaces the method name, and
// minification removes what is left — so the binding layer is guarded by
// regenerating it, not by grepping it here.
func TestDocumentControlsAreInShippedBundle(t *testing.T) {
	js := shippedJS(t)

	for _, want := range []string{
		// The four file actions, and the state the window shows before any of
		// them has been used.
		"Mulai kosong", "Buka…", "Simpan", "Simpan sebagai", "Tidak ada berkas terbuka",
		// The mount report, whose whole point is that a source that did not mount
		// is counted and readable rather than swallowed.
		"ter-mount dari",
	} {
		if !strings.Contains(js, want) {
			t.Errorf("the shipped bundle does not contain %q; the document bar is not in the app the user runs", want)
		}
	}

	// JSON tags of DocumentInfo: their presence means the generated model, not a
	// hand-copied shape, is what the window reads.
	for _, tag := range []string{"duckdbVersion", "profileError", "semanticModels"} {
		if !strings.Contains(js, tag) {
			t.Errorf("the shipped bundle does not read %q; the binding model did not reach the UI", tag)
		}
	}
}

func shippedJS(t *testing.T) string {
	t.Helper()
	handler := application.BundledAssetFileServer(bundledAssets())
	refs := assetRef.FindAllStringSubmatch(get(t, handler, "/").Body.String(), -1)
	var out strings.Builder
	for _, ref := range refs {
		if !strings.HasSuffix(ref[1], ".js") {
			continue
		}
		asset := get(t, handler, "/"+ref[1])
		if asset.Code != 200 {
			t.Fatalf("GET /%s = %d", ref[1], asset.Code)
		}
		out.WriteString(asset.Body.String())
	}
	return out.String()
}
