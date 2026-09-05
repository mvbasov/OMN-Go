package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestResolveContentType pins the single MIME resolver's builtin table,
// including .jsonl (database backups) and the web fonts. .jsonl is
// text/plain so a browser - and above all the Android WebView, which has no
// download handler - shows a backup instead of doing nothing with it. newTestApp has an empty Config.MimeTypes, so these exercise
// the builtin layer directly.
func TestResolveContentType(t *testing.T) {
	a := newTestApp(t)
	cases := map[string]string{
		"/backup.jsonl":      "text/plain; charset=utf-8", // shown inline, never downloaded
		"/css/fonts/x.woff":  "font/woff",                 // was only in the startup mime table
		"/css/fonts/x.woff2": "font/woff2",
		"/css/fonts/x.ttf":   "font/ttf",
		"/img.svg":           "image/svg+xml",
		"/img.png":           "image/png",
		"/img.webp":          "image/webp",
		"/img.gif":           "image/gif",
	}
	for p, want := range cases {
		if got := a.resolveContentType(p); got != want {
			t.Errorf("resolveContentType(%q) = %q, want %q", p, got, want)
		}
	}
	// An extension nothing knows resolves to "" (caller then lets net/http
	// sniff the content).
	if got := a.resolveContentType("/mystery.zzz"); got != "" {
		t.Errorf("resolveContentType(unknown) = %q, want empty", got)
	}
}

// TestResolveContentTypeConfigOverride pins the resolver precedence: the
// per-install Config.MimeTypes wins over the builtin table, and extensions
// the config does not list still fall through to builtin.
func TestResolveContentTypeConfigOverride(t *testing.T) {
	a := newTestApp(t)
	a.Config.MimeTypes = map[string]string{".js": "application/javascript"}

	if got := a.resolveContentType("/app.js"); got != "application/javascript" {
		t.Errorf("Config override not honored: got %q, want application/javascript", got)
	}
	if got := a.resolveContentType("/font.woff"); got != "font/woff" {
		t.Errorf("builtin fallback broken under a partial Config override: got %q", got)
	}
}

// TestMaterializeAssetFromEmbed pins the single lazy embed-extraction: a
// known embedded asset is written to disk on first request and its physical
// path returned.
func TestMaterializeAssetFromEmbed(t *testing.T) {
	a := newTestApp(t)

	phys, ok := a.materializeAsset("/js/OMN-Go/omn-go-core.js")
	if !ok {
		t.Fatal("embedded asset was not materialized")
	}
	want := filepath.Join(a.StorageDir, "html", "js", "OMN-Go", "omn-go-core.js")
	if phys != want {
		t.Errorf("physical path = %q, want %q", phys, want)
	}
	if _, err := os.Stat(phys); err != nil {
		t.Fatalf("asset not written to disk on first request: %v", err)
	}
}

func TestMaterializeAssetMissing(t *testing.T) {
	a := newTestApp(t)
	if _, ok := a.materializeAsset("/js/definitely-not-here.js"); ok {
		t.Error("missing asset should report ok=false (404)")
	}
}

func TestMaterializeAssetDirectory(t *testing.T) {
	a := newTestApp(t)
	if err := os.MkdirAll(filepath.Join(a.StorageDir, "html", "sub"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, ok := a.materializeAsset("/sub"); ok {
		t.Error("a directory should report ok=false, not be served")
	}
}

// TestServeEmbeddableAssetSetsContentType is the end-to-end proof that a
// .jsonl asset (a database backup) is served with the content-type that
// makes the "view" link on the Database Backups page work on each
// platform.
func TestServeEmbeddableAssetSetsContentType(t *testing.T) {
	a := newTestApp(t)
	body := "{\"kind\":\"row\"}\n"
	if err := os.WriteFile(filepath.Join(a.StorageDir, "html", "backup.jsonl"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/backup.jsonl", nil)
	rec := httptest.NewRecorder()
	a.serveEmbeddableAsset(rec, req, "/backup.jsonl")

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
	if rec.Body.String() != body {
		t.Errorf("body = %q, want %q", rec.Body.String(), body)
	}
}

// TestServeEmbeddableAssetMissing pins that an unknown asset is a 404.
func TestServeEmbeddableAssetMissing(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/nope.xyz", nil)
	rec := httptest.NewRecorder()
	a.serveEmbeddableAsset(rec, req, "/nope.xyz")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
}

// TestServeStorageSubdir covers the /images and /user_json handler: both now
// resolve the content-type per file through resolveContentType (forcedType
// == ""). /user_json therefore serves .json as application/json AND .jsonl as
// text/plain, so an uploaded JSON Lines file opens in the browser instead of
// being forced to one type for the whole tree.
func TestServeStorageSubdir(t *testing.T) {
	a := newTestApp(t)

	// /user_json - per-file content-type resolution (forcedType "").
	ujDir := filepath.Join(a.StorageDir, "html", "user_json")
	if err := os.MkdirAll(ujDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ujDir, "data.json"), []byte(`{"a":1}`), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ujDir, "data.jsonl"), []byte("{\"a\":1}\n{\"b\":2}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	uj := a.serveStorageSubdir("user_json", "")
	for name, wantCT := range map[string]string{
		"data.json":  "application/json",
		"data.jsonl": "text/plain; charset=utf-8",
	} {
		req := httptest.NewRequest(http.MethodGet, "/user_json/"+name, nil)
		rec := httptest.NewRecorder()
		uj.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("user_json/%s status %d", name, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != wantCT {
			t.Errorf("user_json/%s Content-Type = %q, want %q", name, ct, wantCT)
		}
	}

	// /images - per-file resolution via resolveContentType.
	imgDir := filepath.Join(a.StorageDir, "html", "images")
	if err := os.MkdirAll(imgDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(imgDir, "p.png"), []byte("\x89PNG\r\n\x1a\n"), 0644); err != nil {
		t.Fatal(err)
	}
	img := a.serveStorageSubdir("images", "")
	req2 := httptest.NewRequest(http.MethodGet, "/images/p.png", nil)
	rec2 := httptest.NewRecorder()
	img.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("images status %d", rec2.Code)
	}
	if ct := rec2.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("images Content-Type = %q, want image/png", ct)
	}
}

// ----------------------------------------------------------------------
// Detailed 404 page
// ----------------------------------------------------------------------

// serveNotFound answers a browser navigation with the themed page and a
// programmatic caller with plain text. The distinction is what keeps
// /api/note's 404 readable in the editor, which renders that body as its
// error message (see loadContent in omn-go-editor.js).
func TestServeNotFoundNegotiatesContentType(t *testing.T) {
	a := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,*/*;q=0.8")
	rec := httptest.NewRecorder()
	a.serveNotFound(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("browser request got content-type %q, want text/html", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "/missing") {
		t.Error("HTML 404 does not name the requested URL")
	}

	// fetch()/XHR send "*/*" - plain text, still detailed.
	req = httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.Header.Set("Accept", "*/*")
	rec = httptest.NewRecorder()
	a.serveNotFound(rec, req)
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("fetch request got content-type %q, want text/plain", ct)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "<div") {
		t.Errorf("plain-text 404 leaked markup:\n%s", body)
	}
	for _, want := range []string{"404 Not Found", "/missing", "GET"} {
		if !strings.Contains(body, want) {
			t.Errorf("plain-text 404 missing %q:\n%s", want, body)
		}
	}
}

// The requested URL and the Referer are attacker-controlled and are echoed
// into HTML, so this pins that they arrive escaped. renderNotFoundPage does
// the escaping by hand - this package deliberately avoids html/template (see
// the note at the top of templates.go), so nothing else will catch a
// regression here.
func TestServeNotFoundEscapesRequestedURL(t *testing.T) {
	a := newTestApp(t)
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.URL.Path = `/<script>alert(1)</script>`
	req.URL.RawQuery = `q="><img src=x onerror=alert(1)>`
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	a.serveNotFound(rec, req)

	body := rec.Body.String()
	for _, sink := range []string{"<script>alert", "<img src=x", "onerror=alert(1)>"} {
		if strings.Contains(body, sink) {
			t.Errorf("unescaped %q reached the 404 page", sink)
		}
	}
	if !strings.Contains(body, "&lt;script&gt;") {
		t.Error("expected the URL to appear HTML-escaped")
	}
}

// A Referer is only turned into a link when it points at this server; an
// off-site or scheme-bearing value must not become an href.
func TestServeNotFoundRefererFiltering(t *testing.T) {
	a := newTestApp(t)
	cases := []struct {
		referer  string
		wantLink bool
	}{
		{"http://127.0.0.1:8080/Notes.html", true},
		{"/Notes.html", true},
		{"javascript:alert(1)", false},
		{"http://evil.example/x", false},
		{"", false},
	}
	for _, tc := range cases {
		req := httptest.NewRequest(http.MethodGet, "/missing", nil)
		req.Host = "127.0.0.1:8080"
		if tc.referer != "" {
			req.Header.Set("Referer", tc.referer)
		}
		req.Header.Set("Accept", "text/html")
		rec := httptest.NewRecorder()
		a.serveNotFound(rec, req)
		body := rec.Body.String()
		if got := strings.Contains(body, "Linked from"); got != tc.wantLink {
			t.Errorf("referer %q: shown=%v, want %v", tc.referer, got, tc.wantLink)
		}
		if strings.Contains(body, `href="javascript:`) {
			t.Errorf("referer %q: produced a javascript: href", tc.referer)
		}
	}
}

// The [text](name) instead of [text](name.html) mistake is the most common
// way to reach a 404 here, since a missing .html is auto-created rather than
// 404ing. The suggestion must fire for a real note and stay silent
// otherwise - especially for traversal attempts, which must not become a
// way to probe for files outside the note tree.
func TestNotFoundSuggestion(t *testing.T) {
	a := newTestApp(t)
	if err := os.WriteFile(filepath.Join(a.StorageDir, "html", "Recipes.html"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"/Recipes":          "/Recipes.html", // compiled note exists
		"/Missing":          "",              // no such note
		"/Recipes.html":     "",              // already has an extension
		"/style.css":        "",              // asset, not a page
		"/":                 "",
		"/../../etc/passwd": "", // traversal never suggests
	}
	for in, want := range cases {
		if got := a.notFoundSuggestion(in); got != want {
			t.Errorf("notFoundSuggestion(%q) = %q, want %q", in, got, want)
		}
	}
}

// /images and /user_json are served by http.FileServer, whose own 404 is
// intercepted so every miss reports the same way.
func TestServeStorageSubdirNotFoundIsIntercepted(t *testing.T) {
	a := newTestApp(t)
	h := a.serveStorageSubdir("images", "")
	req := httptest.NewRequest(http.MethodGet, "/images/nope.png", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "/images/nope.png") {
		t.Errorf("intercepted 404 does not name the requested URL:\n%s", body)
	}
	// FileServer's own body must not be appended after ours.
	if strings.Contains(body, "404 page not found") {
		t.Error("FileServer's default 404 body leaked through the interceptor")
	}
}

func TestSafeLocalPath(t *testing.T) {
	cases := map[string]bool{
		"/Notes.html":         true,
		"/":                   true,
		"//evil.example/x":    false, // protocol-relative
		"javascript:alert(1)": false,
		"http://evil/x":       false,
		"Notes.html":          false, // not rooted
		"":                    false,
	}
	for in, want := range cases {
		if got := safeLocalPath(in); got != want {
			t.Errorf("safeLocalPath(%q) = %v, want %v", in, got, want)
		}
	}
}
