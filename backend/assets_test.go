package backend

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// refreshEmbeddedAssets must (a) replace an extracted asset that no longer
// matches this build's embedded content, (b) preserve the old copy in
// asset_backups/<prev>/..., (c) leave never-extracted files alone, and
// (d) become a no-op - protecting user edits - once the version stamp
// matches APP_VERSION.
func TestRefreshEmbeddedAssets(t *testing.T) {
	dir := t.TempDir()
	a := &App{StorageDir: dir}

	const relJS = "js/omn-go-core.js"
	embedData, err := staticFS.ReadFile("frontend/html/" + relJS)
	if err != nil {
		t.Fatalf("embedded %s missing from staticFS: %v", relJS, err)
	}

	target := filepath.Join(dir, "html", filepath.FromSlash(relJS))
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
	// (b) previous copy preserved (no stamp existed, so label "unknown")
	bak, err := os.ReadFile(filepath.Join(dir, "asset_backups", "unknown", filepath.FromSlash(relJS)))
	if err != nil || !bytes.Equal(bak, stale) {
		t.Error("previous asset copy was not preserved in asset_backups")
	}
	// (c) files that were never extracted must not appear on disk
	if _, err := os.Stat(filepath.Join(dir, "html", "css", "omn-go-core.css")); !os.IsNotExist(err) {
		t.Error("refresh extracted a file that was never on disk")
	}
	// version stamp written
	stamp, err := os.ReadFile(filepath.Join(dir, assetsVersionFilename))
	if err != nil || strings.TrimSpace(string(stamp)) != APP_VERSION {
		t.Errorf("version stamp not written correctly: %q, %v", stamp, err)
	}

	// (d) same version: a user edit must survive untouched
	edited := []byte("// user customization\n")
	if err := os.WriteFile(target, edited, 0644); err != nil {
		t.Fatal(err)
	}
	a.refreshEmbeddedAssets()
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, edited) {
		t.Error("asset overwritten although APP_VERSION did not change")
	}

	// Version change with a user-edited file: edit backed up under the
	// previous version's label, then replaced by the embedded content.
	if err := os.WriteFile(filepath.Join(dir, assetsVersionFilename), []byte("0.0.1\n"), 0644); err != nil {
		t.Fatal(err)
	}
	a.refreshEmbeddedAssets()
	got, _ = os.ReadFile(target)
	if !bytes.Equal(got, embedData) {
		t.Error("edited asset not refreshed after version change")
	}
	bak, err = os.ReadFile(filepath.Join(dir, "asset_backups", "0.0.1", filepath.FromSlash(relJS)))
	if err != nil || !bytes.Equal(bak, edited) {
		t.Error("user edit not preserved in version-labeled backup")
	}
}
