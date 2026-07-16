package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestResolveContentType pins the single MIME resolver's builtin table,
// including the two types added in Phase 3: .jsonl (database backups) and
// the web fonts. newTestApp has an empty Config.MimeTypes, so these exercise
// the builtin layer directly.
func TestResolveContentType(t *testing.T) {
	a := newTestApp(t)
	cases := map[string]string{
		"/backup.jsonl":      "application/jsonl", // NEW in Phase 3
		"/css/fonts/x.woff":  "font/woff",         // was only in the startup mime table
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

	phys, ok := a.materializeAsset("/js/omn-go-core.js")
	if !ok {
		t.Fatal("embedded asset was not materialized")
	}
	want := filepath.Join(a.StorageDir, "html", "js", "omn-go-core.js")
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
// .jsonl asset (a database backup) is now served with the correct
// content-type through the single asset handler.
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
	if ct := rec.Header().Get("Content-Type"); ct != "application/jsonl" {
		t.Errorf("Content-Type = %q, want application/jsonl", ct)
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
// application/jsonl - the Phase 6 change that lets JSON Lines uploads be
// served with their own type instead of being forced to application/json.
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
		"data.jsonl": "application/jsonl",
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
