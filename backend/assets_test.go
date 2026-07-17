package backend

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refreshEmbeddedAssets must, once per APP_VERSION change:
//
//	(a) replace a version-dependent asset whose on-disk copy no longer
//	    matches this build's embedded content,
//	(b) preserve the old copy under asset_backups/<prev>/<rel>,
//	(c) INSTALL a version-dependent file that is absent (this is how a note
//	    added in a new release, e.g. md/SQLImport.md, reaches existing
//	    installs),
//	(d) never create a USER-OWNED file (anything not on the version list -
//	    md/Welcome.md, html/json/bookmarker-tags.json, ...): those are
//	    lazily cached only when absent, never by a version refresh,
//	(e) be a no-op once the version stamp matches APP_VERSION, so user
//	    edits survive between upgrades, and
//	(f) on the next version change, back up a user-edited version-dependent
//	    file and replace it.
func TestRefreshEmbeddedAssets(t *testing.T) {
	dir := t.TempDir()
	a := &App{StorageDir: dir}

	// A version-dependent html/ asset, seeded with a stale copy on disk.
	const rel = "html/js/omn-go-core.js"
	embedData, err := staticFS.ReadFile("frontend/" + rel)
	if err != nil {
		t.Fatalf("embedded %s missing from staticFS: %v", rel, err)
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
	if instEmbed, embErr := staticFS.ReadFile("frontend/md/UserManual.md"); embErr == nil {
		got, err := os.ReadFile(filepath.Join(dir, "md", "UserManual.md"))
		if err != nil || !bytes.Equal(got, instEmbed) {
			t.Error("absent version-dependent file was not installed from embed")
		}
	}
	// (d) user-owned embedded files must NOT be created by a refresh
	for _, userOwned := range []string{
		filepath.Join(dir, "md", "Welcome.md"),
		filepath.Join(dir, "html", "json", "bookmarker-tags.json"),
	} {
		if _, err := os.Stat(userOwned); !os.IsNotExist(err) {
			t.Errorf("refresh created a user-owned file that must stay lazy-only: %s", userOwned)
		}
	}
	// version stamp written
	stamp, err := os.ReadFile(filepath.Join(dir, assetsVersionFilename))
	if err != nil || strings.TrimSpace(string(stamp)) != APP_VERSION {
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
		t.Error("asset overwritten although APP_VERSION did not change")
	}

	// (f) version change with a user-edited file: edit backed up under the
	// previous version's label, then replaced by the embedded content.
	if err := os.WriteFile(filepath.Join(dir, assetsVersionFilename), []byte("0.0.1\n"), 0644); err != nil {
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

// Every version-dependent asset must actually be embedded, otherwise
// refreshEmbeddedAssets silently logs "not embedded" and the file never
// reaches an install - exactly the failure mode of listing a new doc note
// (e.g. md/AndroidIntents.md) but forgetting to ship it. This guards the
// whole list, so any future addition is caught at test time rather than in
// the field.
func TestVersionDependentAssetsAllEmbedded(t *testing.T) {
	for _, rel := range versionDependentAssets {
		if _, err := staticFS.ReadFile("frontend/" + rel); err != nil {
			t.Errorf("version-dependent asset %q is not embedded in staticFS: %v", rel, err)
		}
	}
}
