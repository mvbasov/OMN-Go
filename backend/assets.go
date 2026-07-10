package backend

import (
	"bytes"
	"io/fs"
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
// The frontend/html tree (js/, css/, json/, ...) is embedded in the binary
// and lazily extracted to StorageDir/html on first request (see
// serveLazyEmbed in server.go). Extraction only happens when a file is
// MISSING on disk, because the extracted copies are user-editable
// (?edit=true). The flip side: after an app upgrade, an already-extracted
// file from the previous version keeps shadowing the new embedded one
// forever - a fix shipped in e.g. omn-go-core.js would never reach an
// existing installation.
//
// refreshEmbeddedAssets closes that gap. Once per APP_VERSION change it
// walks the embedded tree and, for every embedded file whose on-disk copy
// differs from the current embedded content, moves the old copy to
// StorageDir/asset_backups/<previous-version>/... and writes the new one.
// Nothing is ever silently lost: a user who customized an asset finds the
// customized copy in the backup directory (the path is logged) and can
// merge their changes back. Files that were never extracted are left
// alone - lazy extraction will serve the new embedded content anyway.
// While the version stamp matches APP_VERSION the function is a cheap
// no-op, so user edits are never touched between upgrades.

// assetsVersionFilename stores (in StorageDir, next to config.json, NOT
// under html/ where it would be served and synced) the APP_VERSION that
// most recently refreshed the extracted assets.
const assetsVersionFilename = "assets_version"

// backupLabelSanitizer keeps backup directory names safe regardless of
// what an old/garbled version stamp contains.
var backupLabelSanitizer = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

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

	htmlDir := filepath.Join(a.StorageDir, "html")
	backupDir := filepath.Join(a.StorageDir, "asset_backups", prevLabel)
	refreshed := 0

	fs.WalkDir(staticFS, "frontend/html", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(p, "frontend/html/")
		diskPath := filepath.Join(htmlDir, filepath.FromSlash(rel))

		diskData, rerr := os.ReadFile(diskPath)
		if os.IsNotExist(rerr) {
			// Never extracted: nothing stale to fix. serveLazyEmbed will
			// extract the current embedded content on first request.
			return nil
		}
		if rerr != nil {
			log.Printf("[assets] cannot read %s: %v", diskPath, rerr)
			return nil
		}
		embedData, eerr := staticFS.ReadFile(p)
		if eerr != nil {
			return nil
		}
		if bytes.Equal(diskData, embedData) {
			return nil
		}

		// On-disk copy differs from this build's embedded content: either
		// it was extracted by an older version, or the user edited it.
		// Preserve it first; never overwrite without a successful backup.
		bakPath := filepath.Join(backupDir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(bakPath), 0755); err != nil {
			log.Printf("[assets] skip %s: cannot create backup dir: %v", rel, err)
			return nil
		}
		if err := os.WriteFile(bakPath, diskData, 0644); err != nil {
			log.Printf("[assets] skip %s: backup failed: %v", rel, err)
			return nil
		}
		if err := os.WriteFile(diskPath, embedData, 0644); err != nil {
			log.Printf("[assets] refresh of %s failed: %v", rel, err)
			return nil
		}
		refreshed++
		log.Printf("[assets] refreshed %s (previous copy saved to asset_backups/%s/%s)", rel, prevLabel, rel)
		return nil
	})

	// Stamp AFTER the walk: if the process dies mid-refresh the next start
	// simply re-runs it (already-refreshed files compare equal and are
	// skipped, so this is idempotent).
	if err := os.WriteFile(verFile, []byte(APP_VERSION+"\n"), 0644); err != nil {
		log.Printf("[assets] cannot write version stamp %s: %v", verFile, err)
	}
	if refreshed > 0 {
		log.Printf("[assets] %d embedded asset(s) refreshed for v%s (previous: %s)", refreshed, APP_VERSION, prevLabel)
	}
}
