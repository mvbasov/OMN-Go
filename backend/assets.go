package backend

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
)

// ----------------------------------------------------------------------
// Version-stamped refresh of extracted embedded assets
// ----------------------------------------------------------------------
//
// The frontend/html tree (js/, css/, json/, ...) and the frontend/md
// starter notes are embedded in the binary. Files reach StorageDir in two
// different ways, and this file draws a hard line between them:
//
//   - USER-OWNED files are created from embedFS ONLY when absent (a lazy
//     cache): serveLazyEmbed (server.go) extracts an html/ file the first
//     time it is requested, and the initial md/ extraction seeds the
//     starter notes once. After that the on-disk copy is yours - a user
//     edits it (?edit=true), and a version change must never overwrite it.
//     md/Welcome.md and html/json/bookmarker-tags.json are the canonical
//     examples: they are meant to be edited and kept.
//
//   - VERSION-DEPENDENT files (versionDependentAssets below) ship as part
//     of the application and must match the running build: the app's own
//     JS/CSS, and the system documentation notes. Lazy extraction alone
//     cannot keep these correct across an upgrade - an already-extracted
//     copy from the previous version shadows the new one forever, and a
//     note ADDED in a new release (e.g. md/SQLImport.md) would never appear
//     at all, since a missing note is synthesized blank rather than pulled
//     from embedFS. refreshEmbeddedAssets closes both gaps.
//
// Once per APP_VERSION change, refreshEmbeddedAssets walks the
// version-dependent list and, for each entry, writes this build's embedded
// copy: it CREATES the file if it is missing (so new bundled notes land on
// existing installs), and if an on-disk copy DIFFERS it first moves that
// copy to StorageDir/asset_backups/<previous-version>/... and then
// replaces it. Nothing is ever silently lost: a user who customized a
// version-dependent file finds their copy in the backup directory (the
// path is logged) and can merge it back. While the version stamp already
// matches APP_VERSION the function is a cheap no-op, so nothing is touched
// between upgrades.

// assetsVersionFilename stores (in StorageDir, next to config.json, NOT
// under html/ where it would be served and synced) the APP_VERSION that
// most recently refreshed the extracted assets.
const assetsVersionFilename = "assets_version"

// assetsRefreshed tells if refreshEmbeddedAssets wrote a minimum of one
// file after this process started. A start that finds the same version
// stamp writes no file, thus the value stays false.
//
// The Android WebView keeps a copy of a script and of a style sheet in
// its own disk cache. After an update of the application the new pages
// can use the old scripts, and some pages then do not operate correctly.
// A user had to stop the application and clear the cache by hand. The
// Android layer reads this value instead, and clears the cache only when
// the assets changed. See AssetsRefreshed.
var assetsRefreshed atomic.Bool

// AssetsRefreshed tells if this start installed or replaced a minimum of
// one version-dependent asset. The value stays the same until the
// process stops.
//
// The function is exported for the gomobile binding. MainActivity.java
// calls it immediately before the first loadUrl. If the answer is true,
// the activity calls WebView.clearCache(true) one time. A start with no
// change of the assets thus keeps the cache.
func AssetsRefreshed() bool {
	return assetsRefreshed.Load()
}

// backupLabelSanitizer keeps backup directory names safe regardless of
// what an old/garbled version stamp contains.
var backupLabelSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// versionDependentAssets are the StorageDir-relative files that ship with
// OMN-Go and must track the running build. See the banner of this file.
// The embedded source of each entry is "frontend/" plus the path, because
// staticFS embeds frontend/html and frontend/md. A file that is NOT here
// is user-owned. Such a file is made when it is absent and never again,
// and a version change leaves it alone.
//
// EACH FILE BELOW html/ SITS UNDER AN OMN-Go DIRECTORY. That rule
// separates what the application owns from what the user owns, and
// TestEveryAppAssetIsUnderOMNGo holds it. The two user files stay at
// html/js/omn-go-custom.js and html/css/omn-go-custom.css. The demo files
// of the Test tree stay beside them, because a note of that tree names
// each one by an absolute path.
//
// The old paths are in retiredAssets below. legacyAssetURL in serving.go
// answers a request for one of them from the new place, thus a note that
// a person wrote before 26.09.12 keeps working.
var versionDependentAssets = []string{
	"html/js/OMN-Go/omn-go-compat.js",
	"html/js/OMN-Go/omn-go-core.js",
	"html/js/OMN-Go/omn-go-editor.js",
	"html/js/OMN-Go/omn-go-sse.js",
	"html/js/OMN-Go/Bookmarker.js",
	"html/js/OMN-Go/auto-render.min.js",
	"html/js/OMN-Go/katex.min.js",
	"html/js/OMN-Go/highlight.min.js",
	"html/css/OMN-Go/omn-go-core.css",
	"html/css/OMN-Go/Bookmarker.css",
	"html/css/OMN-Go/highlight.default.min.css",
	"html/css/OMN-Go/katex.min.css",
	"md/AndroidIntents.md",
	"md/BookmarksHowTo.md",
	"md/Database.md",
	"md/Editor.md",
	"md/ScriptRules.md",
	"md/SQLImport.md",
	"md/UserManual.md",
}

// retiredAssets are the StorageDir-relative paths that this build no
// longer owns. Two kinds of path are here:
//
//   - The old place of a file that moved. 26.09.12 moved each app asset
//     into an OMN-Go directory. The two are html/js/OMN-Go/ and
//     html/css/OMN-Go/.
//   - A file that the application dropped. html/css/markdown.css was
//     24 899 bytes of github-markdown-css that no template and no note
//     loaded, and the class it styles is absent from the whole tree.
//
// THE LIST IS APPEND-ONLY HISTORY. A path stays here after each later
// version, because an install can skip any number of versions.
//
// removeRetiredAssets deletes the on-disk copy of each path. That is not
// tidiness. gitignorePatterns no longer names these paths. A copy that
// stays on disk thus becomes a TRACKED file at the next commit, and it
// reaches each other device through the sync.
var retiredAssets = []string{
	"html/js/omn-go-compat.js",
	"html/js/omn-go-core.js",
	"html/js/omn-go-editor.js",
	"html/js/omn-go-sse.js",
	"html/js/Bookmarker.js",
	"html/js/auto-render.min.js",
	"html/js/katex.min.js",
	"html/js/highlight.min.js",
	"html/css/omn-go-core.css",
	"html/css/Bookmarker.css",
	"html/css/highlight.default.min.css",
	"html/css/katex.min.css",
	"html/css/markdown.css",
}

// retiredFonts adds the old place of each web font to retiredAssets.
//
// A font is not a version-dependent asset. serveLazyEmbed writes one when
// a page asks for it, thus an install holds none, some or each of them.
//
// The list is read from the embedded tree at start, and not written by
// hand. A font that a later version adds or drops thus needs no edit
// here.
var retiredFonts = func() []string {
	entries, err := staticFS.ReadDir("frontend/html/css/OMN-Go/fonts")
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, "html/css/fonts/"+e.Name())
		}
	}
	return out
}()

// retiredAssetDirs are the directories that hold nothing after the
// removal above. Each one goes away when it is empty. A directory that
// holds a file of the user stays, because os.Remove refuses it.
var retiredAssetDirs = []string{
	"html/css/fonts",
}

// removeRetiredAssets deletes each path of retiredAssets from the storage
// directory. A copy that differs from the bytes of the build that shipped
// it goes to asset_backups/<previous>/ first. That is the rule that the
// refresh below uses. A person who changed a file thus keeps that work.
//
// It runs one time for each version change, under the same version stamp
// that guards refreshEmbeddedAssets.
func (a *App) removeRetiredAssets(backupDir string) int {
	removed := 0
	for _, rel := range append(append([]string(nil), retiredAssets...), retiredFonts...) {
		diskPath := filepath.Join(a.StorageDir, filepath.FromSlash(rel))
		diskData, rerr := os.ReadFile(diskPath)
		if rerr != nil {
			continue // absent, which is the normal state after the first run
		}

		// The bytes that this build ships at the NEW place are the bytes
		// that an unchanged old copy holds. A copy that differs is the
		// work of a person, thus it goes to the backup directory.
		if !bytes.Equal(diskData, embeddedTwinOf(rel)) {
			backupPath := filepath.Join(backupDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err == nil {
				if err := os.WriteFile(backupPath, diskData, 0644); err == nil {
					a.logInfof(logAssets, "kept your copy of %s at %s", rel, backupPath)
				} else {
					a.logErrf(logAssets, "cannot back up %s: %v", rel, err)
					continue // do not delete work that has no copy
				}
			} else {
				a.logErrf(logAssets, "cannot make the backup directory for %s: %v", rel, err)
				continue
			}
		}

		if err := os.Remove(diskPath); err != nil {
			a.logErrf(logAssets, "cannot remove the old %s: %v", rel, err)
			continue
		}
		a.logInfof(logAssets, "removed the old %s", rel)
		removed++
	}

	for _, rel := range retiredAssetDirs {
		dirPath := filepath.Join(a.StorageDir, filepath.FromSlash(rel))
		if err := os.Remove(dirPath); err == nil {
			a.logInfof(logAssets, "removed the empty directory %s", rel)
		}
	}
	return removed
}

// embeddedTwinOf returns the bytes that this build ships for the file
// that once sat at rel. It is the file of the same name below OMN-Go/.
//
// The answer is nil for a file that the application dropped. Each copy of
// such a file thus counts as the work of a person, and it reaches the
// backup directory.
func embeddedTwinOf(rel string) []byte {
	dir, name := path.Split(rel)
	data, err := staticFS.ReadFile("frontend/" + dir + "OMN-Go/" + name)
	if err != nil {
		return nil
	}
	return data
}

func (a *App) refreshEmbeddedAssets() {
	// The flag reports the work of this start only.
	assetsRefreshed.Store(false)

	verFile := filepath.Join(a.StorageDir, assetsVersionFilename)
	prevRaw, _ := os.ReadFile(verFile) // missing file => "" => first run
	prev := strings.TrimSpace(string(prevRaw))
	if prev == APP_VERSION {
		return
	}

	prevLabel := prev
	if prevLabel == "" {
		// Installs that predate the version stamp (or a wiped stamp).
		prevLabel = "unknown"
	}
	prevLabel = backupLabelSanitizer.ReplaceAllString(prevLabel, "_")

	backupDir := filepath.Join(a.StorageDir, "asset_backups", prevLabel)

	// Delete the old copy of each asset that moved or went away, BEFORE
	// the install loop below. The order matters for one reason. The
	// install loop writes each file at its new place. A reader who
	// watches the storage directory must never see two copies of one
	// script. See retiredAssets.
	refreshed := a.removeRetiredAssets(backupDir)

	for _, rel := range versionDependentAssets {
		embedData, eerr := staticFS.ReadFile("frontend/" + rel)
		if eerr != nil {
			// Listed but not embedded in this build - nothing to install.
			a.logErrf(logAssets, "%s not embedded in this build: %v", rel, eerr)
			continue
		}
		diskPath := filepath.Join(a.StorageDir, filepath.FromSlash(rel))

		diskData, rerr := os.ReadFile(diskPath)
		if rerr == nil && bytes.Equal(diskData, embedData) {
			continue // already current - nothing to do
		}
		if rerr != nil && !os.IsNotExist(rerr) {
			a.logErrf(logAssets, "cannot read %s: %v", diskPath, rerr)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
			a.logErrf(logAssets, "skip %s: cannot create dir: %v", rel, err)
			continue
		}

		// A differing on-disk copy (an older version's extract, or a user
		// edit) is preserved before being overwritten; never overwrite
		// without a successful backup. A MISSING file has nothing to
		// preserve - it is simply installed.
		existed := rerr == nil
		if existed {
			bakPath := filepath.Join(backupDir, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(bakPath), 0755); err != nil {
				a.logErrf(logAssets, "skip %s: cannot create backup dir: %v", rel, err)
				continue
			}
			if err := os.WriteFile(bakPath, diskData, 0644); err != nil {
				a.logErrf(logAssets, "skip %s: backup failed: %v", rel, err)
				continue
			}
		}

		if err := os.WriteFile(diskPath, embedData, 0644); err != nil {
			a.logErrf(logAssets, "write of %s failed: %v", rel, err)
			continue
		}
		refreshed++
		if existed {
			a.logInfof(logAssets, "refreshed %s (previous copy saved to asset_backups/%s/%s)", rel, prevLabel, rel)
		} else {
			a.logInfof(logAssets, "installed %s from this build", rel)
		}
	}

	// Stamp AFTER the loop: if the process dies mid-refresh the next start
	// simply re-runs it (already-current files compare equal and are
	// skipped, so this is idempotent).
	if err := os.WriteFile(verFile, []byte(APP_VERSION+"\n"), 0644); err != nil {
		a.logErrf(logAssets, "cannot write version stamp %s: %v", verFile, err)
	}
	if refreshed > 0 {
		// The client caches must go. AssetsRefreshed tells the Android
		// layer to clear the cache of the WebView one time.
		assetsRefreshed.Store(true)
		a.logInfof(logAssets, "%d embedded asset(s) refreshed for v%s (previous: %s)", refreshed, APP_VERSION, prevLabel)
	}
}
