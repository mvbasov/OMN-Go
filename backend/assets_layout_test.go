package backend

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// ----------------------------------------------------------------------
// The OMN-Go asset directory
// ----------------------------------------------------------------------
//
// 26.09.12 moved each app-owned asset below html/js/OMN-Go/ and
// html/css/OMN-Go/. Three rules keep that move safe, and each one has a
// test here:
//
//  1. Each app asset is under OMN-Go/, and each user file is not.
//  2. A request for an old URL answers with the file of the new place.
//  3. An upgrade deletes the old copy on disk. A copy that stays would
//     become a tracked file at the next commit, because gitignorePatterns
//     no longer names it.

// The rule of the directory, written as a test. A new asset that lands
// beside the user files breaks this and not something far away.
func TestEveryAppAssetIsUnderOMNGo(t *testing.T) {
	for _, rel := range versionDependentAssets {
		if !strings.HasPrefix(rel, "html/") {
			continue // an md/ note, which this rule does not cover
		}
		dir := path.Dir(rel)
		if path.Base(dir) != "OMN-Go" {
			t.Errorf("%s is app-owned and does not sit in an OMN-Go directory", rel)
		}
		if _, err := staticFS.ReadFile("frontend/" + rel); err != nil {
			t.Errorf("%s is listed and not embedded: %v", rel, err)
		}
	}
}

// The two user files must NOT move. Fixed constraint 8 says that an
// upgrade never writes over a user-owned asset, and the file index marks
// each one user-owned. A move of either would also break each link that
// the User Manual gives.
func TestUserFilesStayOutOfOMNGo(t *testing.T) {
	for _, rel := range []string{
		"frontend/html/js/omn-go-custom.js",
		"frontend/html/css/omn-go-custom.css",
		"frontend/html/js/local_counter.js",
		"frontend/html/json/bookmarker-tags.json",
	} {
		if _, err := staticFS.ReadFile(rel); err != nil {
			t.Errorf("%s moved or went away: %v", rel, err)
		}
	}
	for _, rel := range versionDependentAssets {
		if strings.Contains(rel, "omn-go-custom") {
			t.Errorf("%s is user-owned and must not be version-dependent", rel)
		}
	}
}

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

// The alias must never write a file at the old place. gitignorePatterns
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
// See retiredAssets. A file that stays becomes a tracked file, because
// the .gitignore line for it went away in the same version.
func TestMigrationRemovesTheOldCopy(t *testing.T) {
	a := newTestApp(t)

	// The state of an install that ran an older version: the old paths
	// hold the bytes that this build ships at the new place.
	shipped, err := staticFS.ReadFile("frontend/html/js/OMN-Go/omn-go-core.js")
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
	fontBytes, _ := staticFS.ReadFile("frontend/html/css/OMN-Go/fonts/KaTeX_Main-Regular.woff2")
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
	if err := os.WriteFile(filepath.Join(a.StorageDir, assetsVersionFilename), []byte(previous+"\n"), 0o644); err != nil {
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

// No path may be in both lists. An entry in both would make the refresh
// install a file and the migration delete it, at each version, forever.
func TestRetiredAssetIsNotShipped(t *testing.T) {
	shipped := map[string]bool{}
	for _, rel := range versionDependentAssets {
		shipped[rel] = true
	}
	for _, rel := range append(append([]string(nil), retiredAssets...), retiredFonts...) {
		if shipped[rel] {
			t.Errorf("%s is retired and version-dependent at the same time", rel)
		}
		for _, pattern := range gitignorePatterns {
			if pattern == "/"+rel {
				t.Errorf("%s is retired and still in gitignorePatterns", rel)
			}
		}
	}
	if len(retiredFonts) == 0 {
		t.Error("retiredFonts is empty, thus the old font files stay on each device")
	}
}

// The F-Droid build fetches each vendor asset with this script, and it
// writes the files by path. A script that writes to the old directory
// gives an APK with no KaTeX and no icon font.
//
// The Docker build does not run the script. A local build would thus look
// correct and hide the fault until the release.
func TestFdroidFetchScriptWritesUnderOMNGo(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "android", "fdroid_fetch_assets.sh"))
	if err != nil {
		t.Skipf("the script is not in this tree: %v", err)
	}
	script := string(raw)

	for _, want := range []string{
		`JS_DIR="$REPO_ROOT/backend/frontend/html/js/OMN-Go"`,
		`CSS_DIR="$REPO_ROOT/backend/frontend/html/css/OMN-Go"`,
		`FONT_DIR="$REPO_ROOT/backend/frontend/html/css/OMN-Go/fonts"`,
	} {
		if !strings.Contains(script, want) {
			t.Errorf("the F-Droid script does not hold %s", want)
		}
	}
	if strings.Contains(script, "github-markdown") {
		t.Error("the F-Droid script still fetches markdown.css, which this build dropped")
	}
}

// No template and no bundled note may load markdown.css. The file went
// away in 26.09.12, and a reference to it would answer 404.
func TestNoPageLoadsMarkdownCSS(t *testing.T) {
	for _, tmpl := range []string{indexPageTmpl, editorPageTmpl, configPageTmpl} {
		if strings.Contains(tmpl, "markdown.css") {
			t.Error("a template still loads markdown.css")
		}
	}
	entries, err := staticFS.ReadDir("frontend/md")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		data, err := staticFS.ReadFile("frontend/md/" + e.Name())
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "markdown.css") {
			t.Errorf("md/%s still names markdown.css", e.Name())
		}
	}
}
