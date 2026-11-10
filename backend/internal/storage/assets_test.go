package storage

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/logx"
)

// Each version-dependent file must be in the embed. When a file is not there,
// RefreshEmbeddedAssets only logs "not embedded", and the file reaches no
// install. A new note, for example md/AndroidIntents.md, had that fault: the
// list named it, and the build did not ship it. This test reads the whole
// list, thus the test run finds the fault before a release.
func TestVersionDependentAssetsAllEmbedded(t *testing.T) {
	for _, rel := range VersionDependentAssets {
		if _, err := frontend.Static.ReadFile(rel); err != nil {
			t.Errorf("version-dependent asset %q is not embedded in frontend.Static: %v", rel, err)
		}
	}
}

// refreshEmbeddedAssets must do the following, one time for each app version
// change.
//
//	(a) Replace a version-dependent asset whose on-disk copy no longer
//	    matches the embedded content of this build.
//	(b) Preserve the old copy under asset_backups/<prev>/<rel>.
//	(c) INSTALL a version-dependent file that is absent. This is how a note
//	    added in a new release, for example md/SQLImport.md, reaches an
//	    existing install.
//	(d) Never create a USER-OWNED file. That is anything not on the version
//	    list, such as md/Welcome.md or html/json/bookmarker-tags.json. The
//	    app caches those lazily when they are absent, and never by a version
//	    refresh.
//	(e) Do no work once the version stamp matches the app version, thus a user
//	    edit stays through an upgrade.
//	(f) On the next version change, back up a user-edited version-dependent
//	    file and replace it.
func TestRefreshEmbeddedAssets(t *testing.T) {
	dir := t.TempDir()
	a := &testApp{StorageDir: dir}

	// A version-dependent html/ asset, seeded with a stale copy on disk.
	const rel = "html/js/OMN-Go/omn-go-core.js"
	embedData, err := frontend.Static.ReadFile(rel)
	if err != nil {
		t.Fatalf("frontend.Static has no %s: %v", rel, err)
	}

	target := filepath.Join(dir, filepath.FromSlash(rel))
	os.MkdirAll(filepath.Dir(target), 0755)
	stale := []byte("// stale copy extracted by an older version\n")
	if err := os.WriteFile(target, stale, 0644); err != nil {
		t.Fatal(err)
	}

	a.refreshEmbeddedAssets()

	// (a) stale extracted copy replaced with the embedded content
	got, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(got, embedData) {
		t.Error("stale extracted asset was not refreshed to embedded content")
	}
	// (b) previous copy preserved (no stamp existed, so label "unknown");
	//     the backup mirrors the StorageDir-relative path, html/ prefix included
	bak, err := os.ReadFile(filepath.Join(dir, "asset_backups", "unknown", filepath.FromSlash(rel)))
	if err != nil || !bytes.Equal(bak, stale) {
		t.Error("previous asset copy was not preserved in asset_backups")
	}
	// (c) a version-dependent file that was ABSENT is installed from embed
	if instEmbed, embErr := frontend.Static.ReadFile("md/UserManual.md"); embErr == nil {
		got, err := os.ReadFile(filepath.Join(dir, "md", "UserManual.md"))
		if err != nil || !bytes.Equal(got, instEmbed) {
			t.Error("absent version-dependent file was not installed from embed")
		}
	}
	// (d) a refresh must NOT create a user-owned embedded file
	for _, userOwned := range []string{
		filepath.Join(dir, "md", "Welcome.md"),
		filepath.Join(dir, "html", "json", "bookmarker-tags.json"),
		filepath.Join(dir, "html", "css", "omn-go-custom.css"),
		filepath.Join(dir, "html", "js", "omn-go-custom.js"),
	} {
		if _, err := os.Stat(userOwned); !os.IsNotExist(err) {
			t.Errorf("refresh created a user-owned file that must stay lazy-only: %s", userOwned)
		}
	}
	// version stamp written
	stamp, err := os.ReadFile(filepath.Join(dir, AssetsVersionFilename))
	if err != nil || strings.TrimSpace(string(stamp)) != version {
		t.Errorf("version stamp not written correctly: %q, %v", stamp, err)
	}

	// (e) same version: a user edit to a version-dependent file survives
	edited := []byte("// user customization\n")
	if err := os.WriteFile(target, edited, 0644); err != nil {
		t.Fatal(err)
	}
	a.refreshEmbeddedAssets()
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, edited) {
		t.Error("asset overwritten although the app version did not change")
	}

	// (f) version change with a user-edited file: edit backed up under the
	// previous version's label, then replaced by the embedded content.
	if err := os.WriteFile(filepath.Join(dir, AssetsVersionFilename), []byte("0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.refreshEmbeddedAssets()
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, embedData) {
		t.Error("edited asset not refreshed after version change")
	}
	bak, err = os.ReadFile(filepath.Join(dir, "asset_backups", "0.0.1", filepath.FromSlash(rel)))
	if err != nil || !bytes.Equal(bak, edited) {
		t.Error("user edit not preserved in version-labeled backup")
	}
}

// ----------------------------------------------------------------------
// The OMN-Go asset directory
// ----------------------------------------------------------------------
//
// Each app-owned asset is below html/js/OMN-Go/ or html/css/OMN-Go/. See
// doc/decisions/0007-keep-the-application-files-in-omn-go-directories.md.
// Three rules keep that layout safe, and each one has a test here:
//
//  1. Each app asset is under OMN-Go/, and each user file is not.
//  2. A request for an old URL answers with the file of the new place.
//  3. An upgrade deletes the old copy on disk. A copy that stays would
//     become a tracked file at the next commit, because
//     gitsync.GitignorePatterns does not name it.

// The rule of the directory, written as a test. A new asset that lands
// beside the user files breaks this and not something far away.
func TestEveryAppAssetIsUnderOMNGo(t *testing.T) {
	for _, rel := range VersionDependentAssets {
		if !strings.HasPrefix(rel, "html/") {
			continue // an md/ note, which this rule does not cover
		}
		dir := path.Dir(rel)
		if path.Base(dir) != "OMN-Go" {
			t.Errorf("%s is app-owned and does not sit in an OMN-Go directory", rel)
		}
		if _, err := frontend.Static.ReadFile(rel); err != nil {
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
		"html/js/omn-go-custom.js",
		"html/css/omn-go-custom.css",
		"html/js/local_counter.js",
		"html/json/bookmarker-tags.json",
	} {
		if _, err := frontend.Static.ReadFile(rel); err != nil {
			t.Errorf("%s moved or went away: %v", rel, err)
		}
	}
	for _, rel := range VersionDependentAssets {
		if strings.Contains(rel, "omn-go-custom") {
			t.Errorf("%s is user-owned and must not be version-dependent", rel)
		}
	}
}

// Each script and each style of an OMN-Go directory is app-owned, thus it
// must be on the version list. A file that is not there reaches a device at
// its first request. After that, an upgrade never replaces it, and the device
// runs the old script with the new server. See the recipe of a new feature in
// section 3 of CLAUDE.md.
func TestEachAppScriptAndStyleIsVersionDependent(t *testing.T) {
	listed := map[string]bool{}
	for _, rel := range VersionDependentAssets {
		listed[rel] = true
	}
	for _, dir := range []string{"html/js/OMN-Go", "html/css/OMN-Go"} {
		entries, err := frontend.Static.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			rel := dir + "/" + e.Name()
			if e.IsDir() || listed[rel] {
				continue
			}
			t.Errorf("%s is not in VersionDependentAssets. Add it there and to "+
				"gitsync.GitignorePatterns.", rel)
		}
	}
}

// The start screen of Android counts the files of the list. progress gets
// each file of VersionDependentAssets one time, in sequence, also a file
// that stays the same. A second start with the same version reports nothing.
func TestRefreshEmbeddedAssetsReportsProgress(t *testing.T) {
	a := &testApp{StorageDir: t.TempDir()}
	l, log := a.layout(), a.log(logx.Assets)
	total := len(VersionDependentAssets)
	next := 1
	RefreshEmbeddedAssets(l, "1.0", log, func(done, all int) {
		if done != next || all != total {
			t.Errorf("progress(%d, %d), want (%d, %d)", done, all, next, total)
		}
		next++
	})
	if next != total+1 {
		t.Errorf("progress came %d times, want %d", next-1, total)
	}
	RefreshEmbeddedAssets(l, "1.0", log, func(done, all int) {
		t.Errorf("progress(%d, %d) on a start with the same version", done, all)
	})
}
