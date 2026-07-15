package backend

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
//     can't keep these correct across an upgrade - an already-extracted
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

// backupLabelSanitizer keeps backup directory names safe regardless of
// what an old/garbled version stamp contains.
var backupLabelSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// versionDependentAssets are the StorageDir-relative files that ship with
// OMN-Go and must track the running build (see the file-top comment).
// Each entry's embedded source is "frontend/" + the path - staticFS
// embeds both frontend/html and frontend/md. Anything NOT listed here is
// user-owned and only ever lazily created when absent; a version change
// leaves it alone.
var versionDependentAssets = []string{
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
	"md/Database.md",
	"md/Editor.md",
	"md/ScriptRules.md",
	"md/SQLImport.md",
	"md/UserManual.md",
}

func (a *App) refreshEmbeddedAssets() {
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
	refreshed := 0

	for _, rel := range versionDependentAssets {
		embedData, eerr := staticFS.ReadFile("frontend/" + rel)
		if eerr != nil {
			// Listed but not embedded in this build - nothing to install.
			log.Printf("[assets] %s not embedded in this build: %v", rel, eerr)
			continue
		}
		diskPath := filepath.Join(a.StorageDir, filepath.FromSlash(rel))

		diskData, rerr := os.ReadFile(diskPath)
		if rerr == nil && bytes.Equal(diskData, embedData) {
			continue // already current - nothing to do
		}
		if rerr != nil && !os.IsNotExist(rerr) {
			log.Printf("[assets] cannot read %s: %v", diskPath, rerr)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(diskPath), 0755); err != nil {
			log.Printf("[assets] skip %s: cannot create dir: %v", rel, err)
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
				log.Printf("[assets] skip %s: cannot create backup dir: %v", rel, err)
				continue
			}
			if err := os.WriteFile(bakPath, diskData, 0644); err != nil {
				log.Printf("[assets] skip %s: backup failed: %v", rel, err)
				continue
			}
		}

		if err := os.WriteFile(diskPath, embedData, 0644); err != nil {
			log.Printf("[assets] write of %s failed: %v", rel, err)
			continue
		}
		refreshed++
		if existed {
			log.Printf("[assets] refreshed %s (previous copy saved to asset_backups/%s/%s)", rel, prevLabel, rel)
		} else {
			log.Printf("[assets] installed %s from this build", rel)
		}
	}

	// Stamp AFTER the loop: if the process dies mid-refresh the next start
	// simply re-runs it (already-current files compare equal and are
	// skipped, so this is idempotent).
	if err := os.WriteFile(verFile, []byte(APP_VERSION+"\n"), 0644); err != nil {
		log.Printf("[assets] cannot write version stamp %s: %v", verFile, err)
	}
	if refreshed > 0 {
		log.Printf("[assets] %d embedded asset(s) refreshed for v%s (previous: %s)", refreshed, APP_VERSION, prevLabel)
	}
}
