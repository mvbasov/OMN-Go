package app

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/noteheader"
	"net.basov.omngo/backend/internal/render"
	"net.basov.omngo/backend/internal/storage"
)

// ----------------------------------------------------------------------
// The single asset-serving layer
// ----------------------------------------------------------------------
//
// Two helpers serve each static asset under html/:
//
//   - serveEmbeddableAsset: /js/, /css/, /json/ and the root catch-all.
//     These files ship in the binary and reach html/ at the first request.
//   - serveStorageSubdir: /images/ and each tree of config.UserFileTrees,
//     user content only.
//
// Both take the content type from resolveContentType, the ONE MIME resolver.
// See doc/decisions/0003-use-one-table-for-each-content-type.md.

// legacyAssetPaths maps the old URL of each app asset to its place under
// OMN-Go/. It comes from storage.VersionDependentAssets, storage.RenamedAssets
// and storage.RetiredFonts, thus it follows each move and each new name. Each
// key and value starts with a slash, the same as in materializeAsset.
var legacyAssetPaths = func() map[string]string {
	out := map[string]string{}
	// add maps the URL of relOld before the OMN-Go directories to relNew.
	add := func(relOld, relNew string) {
		dir, name := path.Split(strings.TrimPrefix(relOld, "html"))
		out[strings.TrimSuffix(dir, "OMN-Go/")+name] = strings.TrimPrefix(relNew, "html")
	}
	for _, rel := range storage.VersionDependentAssets {
		if strings.HasPrefix(rel, "html/") {
			add(rel, rel)
		}
	}
	// A file with a new name answers for its old name at both places.
	for relOld, relNew := range storage.RenamedAssets {
		add(relOld, relNew)
		out[strings.TrimPrefix(relOld, "html")] = strings.TrimPrefix(relNew, "html")
	}
	for _, rel := range storage.RetiredFonts {
		name := path.Base(rel)
		out["/css/fonts/"+name] = "/css/OMN-Go/fonts/" + name
	}
	return out
}()

// legacyAssetURL gives the new URL for the old URL of an app asset.
// md/Bookmarks.md and notes of the user name the old paths, and the server
// never rewrites a note of the user. A user file such as /js/mine.js is not
// in the table, and it still answers 404. See
// doc/decisions/0007-keep-the-application-files-in-omn-go-directories.md.
func legacyAssetURL(urlPath string) (string, bool) {
	moved, ok := legacyAssetPaths[urlPath]
	return moved, ok
}

// materializeAsset answers the disk path of the html/ asset for urlPath, and
// it extracts the embedded file at its first request. ok is false for a 404
// and for a directory. This is the ONE lazy extraction.
func (a *App) materializeAsset(urlPath string) (physPath string, ok bool) {
	clean := filepath.Clean(urlPath)

	// An old URL resolves to the new place FIRST. An old copy can stay on
	// disk until removeRetiredAssets runs, and the reader must get the file
	// of this build.
	if moved, isLegacy := legacyAssetURL(filepath.ToSlash(clean)); isLegacy {
		clean = filepath.FromSlash(moved)
	}
	physPath = a.layout().HTML(clean)

	if stat, err := os.Stat(physPath); err == nil {
		if stat.IsDir() {
			return "", false
		}
		return physPath, true
	}

	embedPath := "html" + filepath.ToSlash(clean)
	if data, err := frontend.Static.ReadFile(embedPath); err == nil {
		os.MkdirAll(filepath.Dir(physPath), 0755)
		os.WriteFile(physPath, data, 0644)
		return physPath, true
	}
	return "", false
}

// serveEmbeddableAsset serves one static asset under html/, with the
// extraction of materializeAsset. ?edit=true opens the editor. For the
// catch-all, serveFrontend already handles the edit intent.
func (a *App) serveEmbeddableAsset(w http.ResponseWriter, r *http.Request, urlPath string) {
	if r.URL.Query().Get("edit") == "true" {
		a.serveEditor(w, r, urlPath)
		return
	}
	physPath, ok := a.materializeAsset(urlPath)
	if !ok {
		a.serveNotFound(w, r)
		return
	}
	if ct := a.resolveContentType(urlPath); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	http.ServeFile(w, r, physPath)
}

// serveStorageSubdir serves /images/ or a tree of config.UserFileTrees from
// html/<subDir>/. A
// forcedType pins the content type of the whole tree. Otherwise
// resolveContentType decides for each file. The binary embeds none of these
// files, thus there is no extraction.
func (a *App) serveStorageSubdir(subDir, forcedType string) http.Handler {
	dirPath := a.layout().HTML(subDir)
	os.MkdirAll(dirPath, 0755)
	fsHandler := http.StripPrefix("/"+subDir+"/", http.FileServer(http.Dir(dirPath)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// ?edit=true opens the editor here too. The documented link "[Edit
		// shared data](/user_json/inventory.json?edit=true)" needs it.
		if r.URL.Query().Get("edit") == "true" {
			a.serveEditor(w, r, r.URL.Path)
			return
		}
		if forcedType != "" {
			w.Header().Set("Content-Type", forcedType)
		} else if ct := a.resolveContentType(r.URL.Path); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		// FileServer answers a missing file itself. notFoundInterceptor
		// replaces that answer with serveNotFound.
		fsHandler.ServeHTTP(&notFoundInterceptor{ResponseWriter: w, app: a, req: r}, r)
	})
}

// ----------------------------------------------------------------------
// 404 handling
// ----------------------------------------------------------------------
//
// Each 404 goes through serveNotFound. A page navigation (Accept text/html)
// gets a themed page. A fetch, an <img> or a <script> gets the same facts as
// plain text, because the editor shows the text of a failed /api/note
// directly.

// wantsHTMLError reports a page navigation. Only an explicit text/html
// counts. Anything else gets plain text, which is safe in any context.
func wantsHTMLError(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// notFoundSuggestion answers "<name>.html" when the path names a note that
// exists: the usual cause of a 404 is [text](name) in place of
// [text](name.html). It asks hasKnownAssetExtension, thus /Report.2026 still
// gets a suggestion.
//
// It answers "" unless the resolved path stays inside StorageDir. A request
// such as "/../../etc/passwd" can thus confirm nothing outside the note tree.
func (a *App) notFoundSuggestion(urlPath string) string {
	name := strings.TrimPrefix(urlPath, "/")
	if name == "" || strings.Contains(name, "..") || a.hasKnownAssetExtension(name) {
		return ""
	}
	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)
	if !isPage || baseName == "" {
		return ""
	}
	for _, candidate := range []string{htmlPath, mdPath} {
		if candidate == "" || !a.layout().Contains(candidate) {
			continue
		}
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return "/" + baseName + ".html"
		}
	}
	return ""
}

// serveNotFound writes the detailed 404. The caller must not have written a
// body. It overwrites a Content-Type that is already set.
func (a *App) serveNotFound(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Path
	if r.URL.RawQuery != "" {
		requested += "?" + r.URL.RawQuery
	}

	// Show only a Referer of this server, and only its path. It goes into an
	// href, and it must not become a link out of the app.
	referer := ""
	if raw := r.Referer(); raw != "" {
		if ref, err := url.Parse(raw); err == nil && ref.Path != "" &&
			(ref.Host == "" || ref.Host == r.Host) && !strings.Contains(ref.Path, "..") {
			referer = ref.Path
		}
	}

	view := notFoundView{
		URL:       requested,
		Method:    r.Method,
		Time:      noteheader.Stamp(time.Now()),
		Referer:   referer,
		Suggested: a.notFoundSuggestion(r.URL.Path),
	}

	// One log line for each miss, thus a broken link shows in the console and
	// in /api/logs.
	a.log(logx.NotFound).Infof("%s %s (referer %q)", view.Method, view.URL, view.Referer)

	if !wantsHTMLError(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprintf(w, "404 Not Found\n\nRequested: %s\nMethod:    %s\nTime:      %s\n",
			view.URL, view.Method, view.Time)
		if view.Referer != "" {
			fmt.Fprintf(w, "Linked from: %s\n", view.Referer)
		}
		if view.Suggested != "" {
			fmt.Fprintf(w, "Did you mean: %s\n", view.Suggested)
		}
		return
	}

	body := renderNotFoundPage(view)
	a.renderPage(w, http.StatusNotFound, "Not found", render.PageHeader("Not found", "Error", "css/OMN-Go/omn-go-notfound.css"), body)
}

// serveNotEditable answers an editor request for a file that is not text:
// 415, with the same negotiation as serveNotFound. The server still serves
// the file normally.
func (a *App) serveNotEditable(w http.ResponseWriter, r *http.Request, relPath string) {
	urlPath := "/" + strings.TrimPrefix(relPath, "/")
	ct := a.resolveContentType(relPath)

	a.log(logx.Edit).Errf("refused %s (%s): not a text file", urlPath, ct)

	if !wantsHTMLError(r) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnsupportedMediaType)
		typ := ct
		if typ == "" {
			typ = "unknown"
		}
		fmt.Fprintf(w, "415 Not a text file\n\nFile: %s\nType: %s\n\n"+
			"OMN-Go does not open a picture, a font, an audio file or a video\n"+
			"file in an editor. Open %s to view it.\n", urlPath, typ, urlPath)
		return
	}

	body := renderNotEditablePage(notEditableView{Path: urlPath, Type: ct})
	a.renderPage(w, http.StatusUnsupportedMediaType, "Not a text file",
		render.PageHeader("Not a text file", "Error", "css/OMN-Go/omn-go-notfound.css"), body)
}

// notFoundInterceptor keeps the path handling and the range handling of
// http.FileServer, and it replaces only its 404. A test of the file before
// the call would copy the traversal defenses of FileServer.
type notFoundInterceptor struct {
	http.ResponseWriter
	app      *App
	req      *http.Request
	replaced bool
}

func (w *notFoundInterceptor) WriteHeader(code int) {
	if code == http.StatusNotFound {
		w.replaced = true
		w.app.serveNotFound(w.ResponseWriter, w.req)
		return
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *notFoundInterceptor) Write(b []byte) (int, error) {
	// Drop the 404 body of FileServer. Report the full length, or the handler
	// reads a short write.
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}

// --- The 404 page ---

var (
	notFoundTmpl    = render.LoadTemplate("not_found.html")
	notEditableTmpl = render.LoadTemplate("not_editable.html")
)

// notFoundView holds each value of the detailed 404 page. Each field is RAW,
// and renderNotFoundPage escapes it. An attacker controls the URL and the
// Referer, thus neither may reach the output without an escape.
type notFoundView struct {
	URL       string // path + query, exactly as requested
	Method    string
	Time      string
	Referer   string // "" when absent or not from this server
	Suggested string // "" when there is no plausible alternative
}

// safeLocalPath reports whether s can be an href: a path on this server, with
// no scheme and no "//host". A request header thus cannot become a live link
// out of the app.
func safeLocalPath(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")
}

// notEditableView holds the values of the "not a text file" page.
// renderNotEditablePage escapes Path and Type.
type notEditableView struct {
	Path string // "/css/OMN-Go/fonts/x.woff2"
	Type string // the resolved content type, "unknown" when there is none
}

// renderNotEditablePage makes the page that an editor route sends for a file
// that is not text. See serveEditor. The view link is the same path without
// the edit query.
func renderNotEditablePage(v notEditableView) string {
	typ := v.Type
	if typ == "" {
		typ = "unknown"
	}
	return render.Fill(notEditableTmpl, map[string]string{
		"PATH":     render.EscapeHTML(v.Path),
		"TYPE":     render.EscapeHTML(typ),
		"VIEW_URL": render.EscapeHTML(v.Path),
	})
}

func renderNotFoundPage(v notFoundView) string {
	// This block is trusted HTML that this function makes. Escape each value
	// where it goes in.
	refererRows := ""
	if v.Referer != "" {
		esc := render.EscapeHTML(v.Referer)
		if safeLocalPath(v.Referer) {
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd><a href="%s">%s</a> &middot; <a href="%s?edit=true">edit that page</a></dd>
`, esc, esc, esc)
		} else {
			// Show the Referer, but never as a link. render.EscapeHTML makes it
			// plain text. In an href, a "javascript:" value would stay live.
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd>%s</dd>
`, esc)
		}
	}

	suggestion := ""
	if v.Suggested != "" && safeLocalPath(v.Suggested) {
		esc := render.EscapeHTML(v.Suggested)
		suggestion = fmt.Sprintf(`    <div class="config-field notfound-suggest">
        <span class="notfound-suggest-label">Did you mean</span>
        <a href="%s" class="notfound-suggest-link">%s</a>
        <span class="config-hint">A note of that name exists. A link written as [text](name) asks the server for a file called "name"; note links need the .html suffix - [text](name.html).</span>
    </div>
`, esc, esc)
	}

	return render.Fill(notFoundTmpl, map[string]string{
		"URL":          render.EscapeHTML(v.URL),
		"METHOD":       render.EscapeHTML(v.Method),
		"TIME":         render.EscapeHTML(v.Time),
		"REFERER_ROWS": refererRows,
		"SUGGESTION":   suggestion,
	})
}
