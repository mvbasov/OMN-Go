package backend

import (
	"path/filepath"
	"strings"
)

// resolvePageName is the single place that decides, given a user- or
// URL-supplied "name", whether it refers to a markdown page and where that
// page's source (.md) and compiled (.html) files live on disk.
//
// Three shapes of "name" are accepted, matching how the frontend and the
// various handlers have historically referred to pages:
//   - a bare page name with no extension at all, e.g. "Welcome"
//   - a markdown filename, e.g. "Welcome.md"
//   - a compiled HTML filename, e.g. "Welcome.html"
//
// All three are treated as the same page ("Welcome"), and both mdPath and
// htmlPath are returned so a caller can read/write whichever one it needs
// without re-deriving the other.
//
// Anything else - a name with a different extension such as ".js", ".css",
// ".json", ".png" - is not a markdown page at all. It's a plain static
// asset that only ever lives under html/, so isPage is false and only
// htmlPath is meaningful; mdPath is left empty.
//
// This replaces four independent (and previously slightly-diverged)
// implementations of this same decision that used to live in
// handleGetNote, handleSaveNote, handleEditExternal and serveEditor.
func (a *App) resolvePageName(name string) (mdPath, htmlPath, baseName string, isPage bool) {
	switch {
	case strings.HasSuffix(name, ".md"):
		baseName = strings.TrimSuffix(name, ".md")
		isPage = true
	case strings.HasSuffix(name, ".html"):
		baseName = strings.TrimSuffix(name, ".html")
		isPage = true
	case !strings.Contains(name, "."):
		baseName = name
		isPage = true
	default:
		// Has some other, non-page extension - treat as a static asset.
		return "", filepath.Join(a.StorageDir, "html", filepath.Clean(name)), name, false
	}

	mdPath = filepath.Join(a.StorageDir, "md", filepath.Clean(baseName+".md"))
	htmlPath = filepath.Join(a.StorageDir, "html", filepath.Clean(baseName+".html"))
	return mdPath, htmlPath, baseName, true
}
