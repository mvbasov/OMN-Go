package backend

import (
	"os"
	"path/filepath"
	"strings"
)

// resolvePageName is the single place that decides, given a user- or
// URL-supplied "name", whether it refers to a markdown page and where that
// page's source (.md) and compiled (.html) files live on disk.
//
// Three shapes of "name" are accepted, matching how the frontend and the
// various handlers refer to pages:
//   - a bare page name, e.g. "Welcome" or "Report.2026"
//   - a markdown filename, e.g. "Welcome.md"
//   - a compiled HTML filename, e.g. "Welcome.html"
//
// This function treats all three as the same page. It returns mdPath and
// htmlPath together, thus a caller reads or writes the one it needs and
// derives nothing a second time.
//
// A name that ends in a known file extension, such as ".js", ".css",
// ".json", ".png" or ".txt", is not a markdown page. It is a file that only
// ever lives under html/, thus isPage is false, only htmlPath means
// anything, and mdPath stays empty.
//
// hasKnownAssetExtension (serving.go) is the one authority for that
// decision. Read its banner before you change this switch. In short: the
// LAST extension decides, and an unknown extension is a page. Before
// 26.08.76 this function asked whether the name held a dot, thus a note
// named "Report.2026" was a file and /Report.2026.html gave 404.
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
	case !a.hasKnownAssetExtension(name):
		baseName = name
		isPage = true
	default:
		// The name ends in an extension that this install serves as a
		// file. See hasKnownAssetExtension.
		return "", filepath.Join(a.StorageDir, "html", filepath.Clean(name)), name, false
	}

	// pageHTMLPath (render_cache.go) is the single formula for a page's
	// compiled-HTML path; use it here so the two never drift apart.
	mdPath = filepath.Join(a.StorageDir, "md", filepath.Clean(baseName+".md"))
	htmlPath = a.pageHTMLPath(baseName)
	return mdPath, htmlPath, baseName, true
}

// fileExists reports whether p is a file that is there to be read. A
// directory is not a file, thus a directory answers false.
//
// It lived in files_index.go until 26.08.55, where the file index used it to
// find the note behind a compiled page. That state ("compiled") went away
// when the page stopped naming the ordinary case, and the helper went with
// it - but note_exchange.go calls it three times, and the build broke. It
// belongs here instead: this file is about paths on disk, and no caller of
// this helper owns it.
func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
