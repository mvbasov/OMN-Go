package app

import (
	"net/http"
	"os"

	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/render"
)

// The App side of the render package. renderer gives package render the
// storage directory, a copy of the settings, the version and the hooks of
// the App. Each other method makes one renderer for its call.

// renderer answers the values of the App that a page compile needs. It
// copies the settings, thus one compile sees one set of values.
func (a *App) renderer() render.Renderer {
	return render.Renderer{
		Layout:        a.layout(),
		Config:        a.config.Get(),
		Version:       version,
		Generator:     generator,
		Facts:         a.pages,
		OnPageWritten: a.onPageWritten,
	}
}

// renderPage writes one page in the shell of a note page: the content type,
// the status code, the compiled page and the runtime values. name is the page
// name of render.Renderer.CompilePageWithBody.
func (a *App) renderPage(w http.ResponseWriter, code int, name string, header []byte, body string) {
	rd := a.renderer()
	compiled := rd.CompilePageWithBody(name, header, body)
	render.WriteHTMLHeader(w)
	w.WriteHeader(code)
	w.Write(rd.InjectRuntimeVars(compiled))
}

// injectRuntimeVars puts the values of now into a page. See
// render.Renderer.InjectRuntimeVars.
func (a *App) injectRuntimeVars(page []byte) []byte {
	rd := a.renderer()
	return rd.InjectRuntimeVars(page)
}

// renderAndCache compiles a note and writes html/<name>.html. See
// render.Renderer.RenderAndCache.
func (a *App) renderAndCache(name string, content []byte) ([]byte, error) {
	rd := a.renderer()
	return rd.RenderAndCache(name, content)
}

// ensureHeaderModified sets the Modified: line of a note. The author of a
// new header comes from the settings. See render.EnsureHeaderModified.
func (a *App) ensureHeaderModified(content string, defaultTitle string) string {
	return render.EnsureHeaderModified(content, defaultTitle, a.config.Get().Author)
}

// generateTagsPage writes the Tags page again. See
// render.Renderer.GenerateTagsPage.
func (a *App) generateTagsPage() error {
	rd := a.renderer()
	return rd.GenerateTagsPage(a.log(logx.Tags))
}

// serveTagsPage sends the Tags page, and it rebuilds the page first when it is
// stale. It uses the test of render.Renderer.TagsPageStale, and not the mtime
// test of serveHTMLPage. The rest is the same as the end of serveHTMLPage.
func (a *App) serveTagsPage(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	rd := a.renderer()
	if rd.TagsPageStale(forceRefresh) {
		if err := rd.GenerateTagsPage(a.log(logx.Tags)); err != nil {
			a.log(logx.Tags).Errf("serveTagsPage: %v", err)
		}
	}
	htmlPath := rd.Layout.PageHTML("OMNGoTags")
	render.WriteHTMLHeader(w)
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		w.Write(rd.InjectRuntimeVars(data))
	} else {
		http.ServeFile(w, r, htmlPath)
	}
}

// writeJSON answers with one JSON value and the HTTP status code. See
// render.WriteJSON.
func (a *App) writeJSON(w http.ResponseWriter, code int, v any) {
	render.WriteJSON(w, code, v, a.log(logx.Server))
}
