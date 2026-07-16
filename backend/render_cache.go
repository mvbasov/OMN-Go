package backend

import (
	"fmt"
	"os"
	"path/filepath"
)

// ----------------------------------------------------------------------
// The single compiled-HTML cache pipeline
// ----------------------------------------------------------------------
//
// CACHE CONTRACT (the one place it is written down):
//
//   - md/<name>.md is the SOURCE OF TRUTH. Only the save/edit paths write it.
//   - html/<name>.html is a DERIVED CACHE. renderAndCache is its ONLY writer.
//   - serveHTMLPage's mtime check (md newer than html, or html missing, or an
//     explicit ?refresh) is its ONLY invalidator.
//
// Before this existed, six call sites each open-coded "compilePage +
// MkdirAll + WriteFile" against html/<name>.html (handleSaveNote,
// handleQuickNote, handleBookmark, handleNewPage, recompileMarkdownPage,
// precompileAllPages). They agreed by luck; consolidating them here means
// the cache-write behavior is defined once. See CODE_REVIEW.md Phase 2.
//
// Intentionally, the cached HTML is an INCOMPLETE template: it carries a
// runtimeVarsMarker (see templates.go) that injectRuntimeVars fills in per
// request with values that must always reflect "now" - APP_VERSION, the
// theme, and the internal-editor flag. That is why the cache does not need
// rewriting when those change, and why the on-disk file legitimately still
// contains the raw marker. Do not "fix" that by baking the values in at
// compile time; it would defeat the cache.

// pageHTMLPath is the single formula for a markdown page's compiled-HTML
// path. resolvePageName returns exactly this for a page (they must agree;
// TestPageHTMLPath guards it), and renderAndCache / precompileAllPages use
// it directly so the path is defined in one place.
func (a *App) pageHTMLPath(name string) string {
	return filepath.Join(a.StorageDir, "html", filepath.Clean(name+".html"))
}

// renderAndCache compiles a markdown page and writes it to its on-disk HTML
// cache - the ONLY sanctioned way to produce html/<name>.html (see the
// cache contract above). name is the page's base name (no extension),
// content its markdown source. Parent directories are created as needed.
// Returns the compiled bytes (handy for a caller that also serves them) and
// any error; callers that only need the side effect can ignore the bytes.
func (a *App) renderAndCache(name string, content []byte) ([]byte, error) {
	compiled := a.compilePage(name, content)
	htmlPath := a.pageHTMLPath(name)
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
		return compiled, fmt.Errorf("cache %q: mkdir: %w", name, err)
	}
	if err := os.WriteFile(htmlPath, compiled, 0644); err != nil {
		return compiled, fmt.Errorf("cache %q: write: %w", name, err)
	}
	return compiled, nil
}
