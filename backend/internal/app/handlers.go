package app

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/noteheader"
	"net.basov.omngo/backend/internal/render"
)

func (a *App) serveFrontend(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if path == "/" || path == "/index.html" {
		http.Redirect(w, r, "/Welcome.html", http.StatusSeeOther)
		return
	}

	// Edit intent comes first, for a page and for an asset. One editor page
	// handles each editable file.
	if r.URL.Query().Get("edit") == "true" {
		a.serveEditor(w, r, path)
		return
	}

	if strings.HasSuffix(path, ".html") {
		a.serveHTMLPage(w, r, path)
		return
	}

	a.serveStaticAsset(w, r, path)
}

func (a *App) serveHTMLPage(w http.ResponseWriter, r *http.Request, path string) {
	// requested keeps ".html" for resolvePageName, which reads the LAST
	// extension: "Draft.txt.html" is the note md/Draft.txt.md, and
	// "Draft.txt" is a file. Do not strip ".md", because "Welcome.md" is a
	// valid note name.
	requested := strings.TrimPrefix(path, "/")
	mdPath, htmlPath, name, _ := a.resolvePageName(requested)

	htmlStat, errHtml := os.Stat(htmlPath)
	mdStat, errMd := os.Stat(mdPath)

	forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
	if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
		a.recompileMarkdownPage(name, mdPath, errMd)
	}

	render.WriteHTMLHeader(w)
	data, err := os.ReadFile(htmlPath)
	if err == nil {
		w.Write(a.injectRuntimeVars(data))
	} else {
		http.ServeFile(w, r, htmlPath)
	}
}

func (a *App) recompileMarkdownPage(name, mdPath string, errMd error) {
	if os.IsNotExist(errMd) {
		data, err := frontend.Static.ReadFile("md/" + name + ".md")
		if err != nil {
			data = []byte(noteheader.New(name, a.config.Get().Author, time.Now()))
		}
		if err := a.writeNoteFile(name, data); err != nil {
			a.log(logx.Page).Errf("recompileMarkdownPage: %v", err)
		}
	}

	mdContent, err := os.ReadFile(mdPath)
	if err == nil {
		// Rebuild the cache from the .md on disk. This runs on a plain VIEW,
		// thus it must not rewrite the .md. ensureHeaderModified belongs to
		// handleSaveNote alone.
		if _, err := a.renderAndCache(name, mdContent); err != nil {
			a.log(logx.Precompile).Errf("recompileMarkdownPage: %v", err)
		}
	}
}

// serveEditor handles each ?edit=true request: the internal editor page, or
// the external editor when the internal one is off.
func (a *App) serveEditor(w http.ResponseWriter, r *http.Request, path string) {
	relPath := strings.TrimPrefix(path, "/")

	if _, _, _, isPage := a.resolvePageName(relPath); !isPage {
		// No editor for a picture, a font, an audio file or a video file.
		// Refuse before the extraction below. A page skips this test: its
		// edit opens the markdown source.
		if !a.editableFileType(relPath) {
			a.serveNotEditable(w, r, relPath)
			return
		}
		// Put a shipped html/ asset on disk. The external editor and the
		// Android intent open the file path directly, and a missing file
		// gives them nothing to show.
		a.materializeAsset("/" + filepath.ToSlash(relPath))
	}

	if !a.config.Get().UseInternalEd {
		http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
		return
	}

	a.renderInternalEditor(w, relPath)
}

// renderInternalEditor writes the editor page for relPath. The page fetches
// the text from /api/note, thus no view page carries a second copy.
func (a *App) renderInternalEditor(w http.ResponseWriter, relPath string) {
	_, _, baseName, isPage := a.resolvePageName(relPath)

	// name goes to /api/note and /api/save. A page sends baseName + ".md",
	// because both endpoints read the LAST extension.
	name := relPath
	viewURL := "/" + relPath
	pageExt := filepath.Ext(relPath)
	title := relPath
	if isPage {
		name = baseName + ".md"
		viewURL = "/" + baseName + ".html"
		pageExt = ".md"
		title = baseName
	}

	page := render.RenderEditorPage(render.EditorPageView{
		Title:   title,
		Name:    name,
		PageExt: pageExt,
		ViewURL: viewURL,
	})

	render.WriteHTMLHeader(w)
	w.Write(a.injectRuntimeVars([]byte(page)))
}

// serveStaticAsset is the root catch-all for an embedded asset outside /js,
// /css and /json, for example favicon.ico. It shares serveEmbeddableAsset
// with those trees.
func (a *App) serveStaticAsset(w http.ResponseWriter, r *http.Request, path string) {
	a.serveEmbeddableAsset(w, r, path)
}
