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
// precompileAllPages). They agreed by luck. To hold them here means that
// the cache-write behavior is defined one time.
//
// The cached HTML is deliberately an INCOMPLETE template. It carries a
// runtimeVarsMarker (see templates.go). injectRuntimeVars fills that marker
// per request, with the values that must always show "now". Those values are
// APP_VERSION, the theme, and the internal-editor flag. The cache thus needs
// no rewrite when one of them changes, and the on-disk file correctly still
// holds the raw marker. Do not "repair" that with the values baked in at
// compile time. That would defeat the cache.

// pageHTMLPath is the single formula for the compiled-HTML path of a markdown
// page. resolvePageName returns exactly this for a page, and the two must
// agree. TestPageHTMLPath guards that. renderAndCache and precompileAllPages
// use pageHTMLPath directly, thus the path is defined in one place.
func (a *App) pageHTMLPath(name string) string {
	return filepath.Join(a.StorageDir, "html", filepath.Clean(name+".html"))
}

// renderAndCache compiles a markdown page and writes it to its on-disk HTML
// cache. That is the ONLY sanctioned way to produce html/<name>.html. See the
// cache contract above. name is the base name of the page, with no extension,
// and content is its markdown source. It creates each parent directory as
// necessary. It returns the compiled bytes, which help a caller that also
// serves them, and it returns any error. A caller that needs only the side
// effect can ignore the bytes.
func (a *App) renderAndCache(name string, content []byte) ([]byte, error) {
	compiled := a.compilePage(name, content)
	htmlPath := a.pageHTMLPath(name)
	if err := os.MkdirAll(filepath.Dir(htmlPath), 0755); err != nil {
		return compiled, fmt.Errorf("cache %q: mkdir: %w", name, err)
	}
	if err := os.WriteFile(htmlPath, compiled, 0644); err != nil {
		return compiled, fmt.Errorf("cache %q: write: %w", name, err)
	}
	// Every in-process note change goes through here. Those are the save, the
	// quick note, the bookmark, the new page, the sync and the precompile.
	// This is thus the one place that can tell the search index "something
	// moved", with no hook in each handler. It only skips the wait for the
	// next stat walk. The walk is still what decides what changed.
	a.markSearchIndexDirty()
	return compiled, nil
}
