package backend

import (
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// ----------------------------------------------------------------------
// The single asset-serving layer
// ----------------------------------------------------------------------
//
// Static assets under html/ reach the browser through exactly two shared
// helpers here, so "which code serves URL X, and what content-type does it
// get" is answerable in one place:
//
//   - serveEmbeddableAsset backs the /js/, /css/, /json/ trees and the root
//     catch-all (favicon.ico, robots.txt, ...). These ship embedded in the
//     binary and are lazily extracted to html/ on first request.
//   - serveStorageSubdir backs the /images/ and /user_json/ trees, which are
//     pure user content (never embedded), served straight from html/<sub>/.
//
// Both resolve content-type through resolveContentType, the ONE MIME
// resolver - it folds together what used to be three separate sources: the
// per-install Config.MimeTypes map, the startup mime.AddExtensionType(...)
// registrations, and http.FileServer's implicit stdlib lookup. See
// CODE_REVIEW.md Phase 3.

// builtinMIME is OMN-Go's canonical content-type table. It supersedes the
// startup mime.AddExtensionType(...) calls that used to seed the process
// mime table (so those are removed from StartServer), adds JSON Lines
// (.jsonl, used by the database backups in db_backup.go), and keeps the
// web-font types explicit for minimal containers whose stdlib mime tables
// are sparse.
var builtinMIME = map[string]string{
	".html":  "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".mjs":   "text/javascript; charset=utf-8",
	".json":  "application/json",
	".jsonl": "application/jsonl", // JSON Lines - database backups (db_backup.go)
	".md":    "text/markdown; charset=utf-8",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".gif":   "image/gif",
	".webp":  "image/webp",
	".ico":   "image/x-icon",
	".woff":  "font/woff",
	".woff2": "font/woff2",
	".ttf":   "font/ttf",
}

// resolveContentType is the single MIME resolver. Precedence:
//  1. Config.MimeTypes  - the per-install override (config.json).
//  2. builtinMIME       - OMN-Go's canonical table (also covers what the old
//     startup mime.AddExtensionType calls registered, plus .jsonl).
//  3. mime.TypeByExtension - the Go stdlib fallback.
//
// Returns "" when nothing knows the extension, in which case the caller
// leaves the header unset and lets net/http sniff the content.
func (a *App) resolveContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ct, ok := a.GetConfig().MimeTypes[ext]; ok && ct != "" {
		return ct
	}
	if ct, ok := builtinMIME[ext]; ok {
		return ct
	}
	return mime.TypeByExtension(ext)
}

// materializeAsset returns the on-disk path of the html/ asset for urlPath,
// extracting it from the embedded frontend on first request if it is not on
// disk yet. ok is false for a genuine 404 (present neither on disk nor
// embedded) or when the path resolves to a directory. This is the ONE
// implementation of the lazy embed-extraction that used to be copied between
// the /js|/css|/json handler (server.go) and the root catch-all
// (serveStaticAsset).
func (a *App) materializeAsset(urlPath string) (physPath string, ok bool) {
	clean := filepath.Clean(urlPath)
	physPath = filepath.Join(a.StorageDir, "html", clean)

	if stat, err := os.Stat(physPath); err == nil {
		if stat.IsDir() {
			return "", false
		}
		return physPath, true
	}

	embedPath := "frontend/html" + filepath.ToSlash(clean)
	if data, err := staticFS.ReadFile(embedPath); err == nil {
		os.MkdirAll(filepath.Dir(physPath), 0755)
		os.WriteFile(physPath, data, 0644)
		return physPath, true
	}
	return "", false
}

// serveEmbeddableAsset serves one static asset under html/ that may need
// first-request extraction from the embedded frontend (see materializeAsset).
// It backs the /js/, /css/, /json/ trees and the root catch-all. An
// ?edit=true request is handed to the dedicated editor page, exactly as
// before: the /js|/css|/json routes reach this with edit intent, while the
// catch-all's edit intent is already peeled off in serveFrontend, so the
// check here is a harmless no-op on that path.
func (a *App) serveEmbeddableAsset(w http.ResponseWriter, r *http.Request, urlPath string) {
	if r.URL.Query().Get("edit") == "true" {
		a.serveEditor(w, r, urlPath)
		return
	}
	physPath, ok := a.materializeAsset(urlPath)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if ct := a.resolveContentType(urlPath); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, physPath)
}

// serveStorageSubdir serves the pure-user-content trees /images/ and
// /user_json/ from html/<subDir>/. forcedType, when non-empty, pins the
// content-type for the whole tree (keeps /user_json/ as application/json
// regardless of extension); otherwise resolveContentType decides per file.
// These files are never embedded, so - unlike serveEmbeddableAsset - there
// is no extraction step.
func (a *App) serveStorageSubdir(subDir, forcedType string) http.Handler {
	dirPath := filepath.Join(a.StorageDir, "html", subDir)
	os.MkdirAll(dirPath, 0755)
	fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if forcedType != "" {
			w.Header().Set("Content-Type", forcedType)
		} else if ct := a.resolveContentType(r.URL.Path); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		fsHandler.ServeHTTP(w, r)
	})
}
