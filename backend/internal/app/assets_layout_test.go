package app

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// A note that a person wrote before the move names the old URL. The
// answer must be the file, and not a 404. md/Bookmarks.md is the case
// that matters: it is user-owned, thus an upgrade never rewrites it.
func TestLegacyAssetURLServesTheNewFile(t *testing.T) {
	a := newTestApp(t)

	cases := []struct{ old, current string }{
		{"/js/omn-go-core.js", "/js/OMN-Go/omn-go-core.js"},
		{"/js/Bookmarker.js", "/js/OMN-Go/Bookmarker.js"},
		{"/css/Bookmarker.css", "/css/OMN-Go/Bookmarker.css"},
		{"/css/katex.min.css", "/css/OMN-Go/katex.min.css"},
		{"/css/fonts/KaTeX_Main-Regular.woff2", "/css/OMN-Go/fonts/KaTeX_Main-Regular.woff2"},
	}
	for _, c := range cases {
		oldPath, oldOK := a.materializeAsset(c.old)
		newPath, newOK := a.materializeAsset(c.current)
		if !oldOK {
			t.Errorf("%s answers nothing, thus a note that names it is broken", c.old)
			continue
		}
		if !newOK {
			t.Errorf("%s answers nothing", c.current)
			continue
		}
		if oldPath != newPath {
			t.Errorf("%s resolves to %q and %s resolves to %q", c.old, oldPath, c.current, newPath)
		}
	}
}

// The alias must never write a file at the old place. gitsync.GitignorePatterns
// no longer names those paths, thus a file there reaches git and then
// each other device.
func TestLegacyAssetURLWritesNoOldFile(t *testing.T) {
	a := newTestApp(t)

	if _, ok := a.materializeAsset("/js/omn-go-core.js"); !ok {
		t.Fatal("the old URL answers nothing")
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "js", "omn-go-core.js")); err == nil {
		t.Error("the alias wrote the file at its old place, thus the next commit tracks it")
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "js", "OMN-Go", "omn-go-core.js")); err != nil {
		t.Errorf("the alias wrote nothing at the new place: %v", err)
	}
}

// The old name of omn-go-api.js is omn-go-sse.js. A page that a browser holds
// from an older version still asks for the old name, and a note of the user
// can name it. See storage.RenamedAssets.
func TestLegacyAssetURLKnowsTheOldNameOfTheAPIScript(t *testing.T) {
	a := newTestApp(t)
	want, err := frontend.Static.ReadFile("html/js/OMN-Go/omn-go-api.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, urlPath := range []string{"/js/OMN-Go/omn-go-sse.js", "/js/omn-go-sse.js"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, urlPath, nil)
		a.serveEmbeddableAsset(rec, req, req.URL.Path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s answers %d, want 200", urlPath, rec.Code)
			continue
		}
		if rec.Body.String() != string(want) {
			t.Errorf("%s does not answer omn-go-api.js", urlPath)
		}
	}
	for _, old := range []string{
		filepath.Join("html", "js", "OMN-Go", "omn-go-sse.js"),
		filepath.Join("html", "js", "omn-go-sse.js"),
	} {
		if _, err := os.Stat(filepath.Join(a.StorageDir, old)); err == nil {
			t.Errorf("the alias wrote %s, thus the next commit can track it", old)
		}
	}
}

// The old name of omn-go-db.js is omn-go-dbbackups.js. A browser that holds
// the Database Backups page from that version still asks for the old name.
// See storage.RenamedAssets.
func TestLegacyAssetURLKnowsTheOldNameOfTheDBScript(t *testing.T) {
	a := newTestApp(t)
	want, err := frontend.Static.ReadFile("html/js/OMN-Go/omn-go-db.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, urlPath := range []string{"/js/OMN-Go/omn-go-dbbackups.js", "/js/omn-go-dbbackups.js"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, urlPath, nil)
		a.serveEmbeddableAsset(rec, req, req.URL.Path)
		if rec.Code != http.StatusOK {
			t.Errorf("%s answers %d, want 200", urlPath, rec.Code)
			continue
		}
		if rec.Body.String() != string(want) {
			t.Errorf("%s does not answer omn-go-db.js", urlPath)
		}
	}
	old := filepath.Join(a.StorageDir, "html", "js", "OMN-Go", "omn-go-dbbackups.js")
	if _, err := os.Stat(old); err == nil {
		t.Errorf("the alias wrote %s, thus the next commit can track it", old)
	}
}

// storage.RetiredAssets must hold each old name of storage.RenamedAssets, and
// this build must ship each new name. If not, the old copy stays on the
// device, or the alias answers 404.
func TestEachRenamedAssetIsRetiredAndShipped(t *testing.T) {
	retired := map[string]bool{}
	for _, rel := range storage.RetiredAssets {
		retired[rel] = true
	}
	shipped := map[string]bool{}
	for _, rel := range storage.VersionDependentAssets {
		shipped[rel] = true
	}
	for relOld, relNew := range storage.RenamedAssets {
		if !retired[relOld] {
			t.Errorf("%s has a new name and is not in storage.RetiredAssets", relOld)
		}
		if !shipped[relNew] {
			t.Errorf("%s is the new name of %s and is not in storage.VersionDependentAssets", relNew, relOld)
		}
	}
}

// The alias covers the moved files alone. A name that nobody shipped must
// still answer 404, or a fault of a name reads as a working link.
func TestLegacyAssetURLIgnoresAUserFile(t *testing.T) {
	for _, urlPath := range []string{
		"/js/mine.js",
		"/js/omn-go-custom.js",
		"/css/omn-go-custom.css",
		"/js/OMN-Go/mine.js",
	} {
		if moved, ok := legacyAssetURL(urlPath); ok {
			t.Errorf("%s reads as a moved asset and answers %q", urlPath, moved)
		}
	}
}

// A request through the HTTP handler must answer 200 for an old URL, and
// with the correct content type. The handler is what a note reaches, and
// materializeAsset alone does not prove that.
func TestLegacyAssetURLOverHTTP(t *testing.T) {
	a := newTestApp(t)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/js/Bookmarker.js", nil)
	a.serveEmbeddableAsset(rec, req, req.URL.Path)

	if rec.Code != http.StatusOK {
		t.Fatalf("the old URL answers %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/javascript") {
		t.Errorf("the content type is %q", ct)
	}
	if body := rec.Body.String(); !strings.Contains(body, "OMNBookmarkerConfigG") {
		t.Error("the answer is not Bookmarker.js")
	}
}

// An upgrade must delete the copy of each moved file at its old place.
// See storage.RetiredAssets. A file that stays becomes a tracked file, because
// the .gitignore line for it went away in the same version.
func TestMigrationRemovesTheOldCopy(t *testing.T) {
	a := newTestApp(t)

	// The state of an install that ran an older version: the old paths
	// hold the bytes that this build ships at the new place.
	shipped, err := frontend.Static.ReadFile("html/js/OMN-Go/omn-go-core.js")
	if err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(a.StorageDir, "html", "js", "omn-go-core.js")
	if err := os.MkdirAll(filepath.Dir(oldPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(oldPath, shipped, 0o644); err != nil {
		t.Fatal(err)
	}
	// A file that the application dropped, and one old font.
	deadPath := filepath.Join(a.StorageDir, "html", "css", "markdown.css")
	os.MkdirAll(filepath.Join(a.StorageDir, "html", "css", "fonts"), 0o755)
	os.WriteFile(deadPath, []byte(".markdown-body{}"), 0o644)
	fontPath := filepath.Join(a.StorageDir, "html", "css", "fonts", "KaTeX_Main-Regular.woff2")
	fontBytes, _ := frontend.Static.ReadFile("html/css/OMN-Go/fonts/KaTeX_Main-Regular.woff2")
	os.WriteFile(fontPath, fontBytes, 0o644)

	a.refreshEmbeddedAssets()

	for _, gone := range []string{oldPath, deadPath, fontPath} {
		if _, err := os.Stat(gone); err == nil {
			t.Errorf("%s is still on disk after the upgrade", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "css", "fonts")); err == nil {
		t.Error("the empty html/css/fonts directory is still here")
	}
	// The new place holds the file of this build.
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "js", "OMN-Go", "omn-go-core.js")); err != nil {
		t.Errorf("the upgrade installed nothing at the new place: %v", err)
	}
}

// A person who edited an app file keeps that work. The migration writes
// the copy to asset_backups/ before it deletes the file, the same rule
// that the refresh uses.
func TestMigrationKeepsAChangedOldCopy(t *testing.T) {
	a := newTestApp(t)
	// A stamp that no release carries, thus the refresh always runs and
	// the name of the backup directory is known.
	const previous = "26.00.01"
	if err := os.WriteFile(filepath.Join(a.StorageDir, storage.AssetsVersionFilename), []byte(previous+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	oldPath := filepath.Join(a.StorageDir, "html", "css", "omn-go-core.css")
	os.MkdirAll(filepath.Dir(oldPath), 0o755)
	const mine = "/* the colours of this device */\n"
	if err := os.WriteFile(oldPath, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	a.refreshEmbeddedAssets()

	if _, err := os.Stat(oldPath); err == nil {
		t.Error("the changed copy is still at the old place")
	}
	backup := filepath.Join(a.StorageDir, "asset_backups", previous, "html", "css", "omn-go-core.css")
	data, err := os.ReadFile(backup)
	if err != nil {
		t.Fatalf("the work of the person is gone: %v", err)
	}
	if string(data) != mine {
		t.Errorf("the backup holds %q", string(data))
	}
}

// No template and no bundled note may load markdown.css. The build does
// not ship the file, thus a reference to it would answer 404.
func TestNoPageLoadsMarkdownCSS(t *testing.T) {
	for _, tmpl := range []string{render.IndexPageTmpl, render.EditorPageTmpl, configPageTmpl} {
		if strings.Contains(tmpl, "markdown.css") {
			t.Error("a template still loads markdown.css")
		}
	}
	entries, err := frontend.Static.ReadDir("md")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := frontend.Static.ReadFile("md/" + e.Name())
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "markdown.css") {
			t.Errorf("md/%s still names markdown.css", e.Name())
		}
	}
}
