package backend

import (
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
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
// mime table (so those are removed from StartServer), carries JSON Lines
// (.jsonl, used by the database backups in db_backup.go), and keeps the
// web-font types explicit for minimal containers whose stdlib mime tables
// are sparse.
var builtinMIME = map[string]string{
	".html": "text/html; charset=utf-8",
	".css":  "text/css; charset=utf-8",
	".js":   "text/javascript; charset=utf-8",
	".mjs":  "text/javascript; charset=utf-8",
	".json": "application/json",
	// JSON Lines - database backups (db_backup.go). text/plain, NOT
	// application/jsonl: a browser renders text/plain inline, and the
	// Android WebView has no download handler at all, so any type it
	// cannot render leaves the user with a link that does nothing (the
	// "view" link on the Database Backups page is exactly that link).
	// text/plain and not application/json either - a backup is JSON
	// Lines, not one JSON document, so a browser JSON viewer reports a
	// parse error on the second line instead of showing the file.
	".jsonl": "text/plain; charset=utf-8",
	".md":    "text/markdown; charset=utf-8",
	// Go's own table has no ".txt" (mime/type.go, builtinTypesLower), and a
	// phone has no /etc/mime.types for the stdlib to read at init. The
	// consequence of the missing row was not only a guessed type:
	// editableFileType asks this same table whether a file is text, so on
	// Android a .txt got no edit link and the editor refused to open it,
	// while the identical file behaved correctly on a desktop Linux with
	// mime-support installed. A file kept beside a note (note_files.go) is
	// a .txt, so this row is what makes such a file editable at all on the
	// device where it matters.
	".txt":   "text/plain; charset=utf-8",
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

// hasKnownAssetExtension reports whether the last extension of name is one
// that this install serves as a file. It is the one authority for the
// question "is this name a note, or a file under html/". resolvePageName
// asks it, and so do compilePage, rewriteInternalLink and
// notFoundSuggestion.
//
// THE LAST EXTENSION DECIDES. Before 26.08.76 the application asked whether
// the name held a dot. A note named "Report.2026" answered yes and became a
// file under html/, thus /Report.2026.html gave 404. A note name can hold a
// dot, and this function is what makes that true.
//
// The rule reads:
//
//	.md                     the source of a note
//	.html                   a compiled note. An .html always has a .md source
//	a known extension       a file under html/, for example .js or .txt
//	an unknown extension    a note, for example "Report.2026"
//	no extension            a note
//
// A note named "Draft.txt" thus has the source md/Draft.txt.md and compiles
// to html/Draft.txt.html. The file html/Draft.txt is a different thing with
// a different name. The two never collide, because each name carries each
// of its extensions.
//
// IT MUST NOT CALL mime.TypeByExtension. resolveContentType above keeps
// that fallback, because a Content-Type header can differ between two
// devices with no harm. This answer cannot. The stdlib reads
// /etc/mime.types, thus a desktop with mime-support knows ".doc" and a
// phone does not. A note named "Plan.doc" would then be a file on one
// device and a note on the other. Git sync carries that name to both.
func (a *App) hasKnownAssetExtension(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return false
	}
	if ct, ok := a.GetConfig().MimeTypes[ext]; ok && ct != "" {
		return true
	}
	_, ok := builtinMIME[ext]
	return ok
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
		a.serveNotFound(w, r)
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
		// "?edit=true opens any file that OMN-Go serves" holds for these two
		// trees as well. They have their own routes, so an edit request here
		// never reaches serveFrontend, and the documented link
		// "[Edit shared data](/user_json/inventory.json?edit=true)" used to
		// serve the raw JSON instead of opening the editor.
		if r.URL.Query().Get("edit") == "true" {
			a.serveEditor(w, r, r.URL.Path)
			return
		}
		if forcedType != "" {
			w.Header().Set("Content-Type", forcedType)
		} else if ct := a.resolveContentType(r.URL.Path); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		// FileServer answers a missing file itself; intercept that one
		// status so /images and /user_json report misses like everything
		// else (see notFoundInterceptor).
		fsHandler.ServeHTTP(&notFoundInterceptor{ResponseWriter: w, app: a, req: r}, r)
	})
}

// ----------------------------------------------------------------------
// 404 handling
// ----------------------------------------------------------------------
//
// Everything that can 404 funnels through serveNotFound so there is one
// answer instead of net/http's bare "404 page not found" in three places.
//
// It content-negotiates rather than always emitting HTML. A browser
// navigation sends "Accept: text/html,..." and gets the full themed page;
// fetch()/XHR (Accept "*/*") and <img>/<script> loads get the same facts as
// plain text. That distinction matters: /api/note's 404 is read as text by
// the editor's loadContent(), which shows it to the user directly, so
// answering that with a page of HTML would replace a readable message with
// markup.

// wantsHTMLError reports whether the caller looks like a page navigation
// rather than a programmatic fetch. Deliberately conservative: only an
// explicit text/html in Accept counts, so anything unusual degrades to the
// plain-text form, which is safe to show in any context.
func wantsHTMLError(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/html")
}

// notFoundSuggestion returns "<name>.html" when the failing path names a
// note that exists. The reader wrote [text](name) and not
// [text](name.html). That mistake is the most common way to reach a 404
// here. A missing .html does not give a 404. serveHTMLPage rebuilds it,
// see recompileMarkdownPage.
//
// The guard below asks hasKnownAssetExtension, and not whether the name has
// an extension at all. A note may hold a dot in its name since 26.08.76,
// thus /Report.2026 must still get its suggestion.
//
// Returns "" unless the lookup is provably safe and the note really exists:
// the resolved path must stay inside StorageDir, so a crafted "/../../etc/
// passwd" style request can neither confirm nor link to anything outside
// the note tree.
func (a *App) notFoundSuggestion(urlPath string) string {
	name := strings.TrimPrefix(urlPath, "/")
	if name == "" || strings.Contains(name, "..") || a.hasKnownAssetExtension(name) {
		return ""
	}
	mdPath, htmlPath, baseName, isPage := a.resolvePageName(name)
	if !isPage || baseName == "" {
		return ""
	}
	root, err := filepath.Abs(a.StorageDir)
	if err != nil {
		return ""
	}
	within := func(p string) bool {
		abs, err := filepath.Abs(p)
		if err != nil {
			return false
		}
		rel, err := filepath.Rel(root, abs)
		return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
	}
	for _, candidate := range []string{htmlPath, mdPath} {
		if candidate == "" || !within(candidate) {
			continue
		}
		if stat, err := os.Stat(candidate); err == nil && !stat.IsDir() {
			return "/" + baseName + ".html"
		}
	}
	return ""
}

// serveNotFound writes the detailed 404. Callers must not have written a
// body yet; any Content-Type already set (serveStorageSubdir pins one
// before delegating) is overwritten here.
func (a *App) serveNotFound(w http.ResponseWriter, r *http.Request) {
	requested := r.URL.Path
	if r.URL.RawQuery != "" {
		requested += "?" + r.URL.RawQuery
	}

	// Only a Referer pointing at this same server is shown, and only as its
	// path: it is echoed into an href, so an off-site or malformed value is
	// dropped rather than turned into a link out of the app.
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
		Time:      time.Now().Format("2006-01-02 15:04:05"),
		Referer:   referer,
		Suggested: a.notFoundSuggestion(r.URL.Path),
	}

	// One log line per miss, so a broken link is visible in the JS console
	// and the /api/logs stream without having to reproduce it.
	a.logInfof(log404, "%s %s (referer %q)", view.Method, view.URL, view.Referer)

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
	compiled := a.compilePageWithBody("Not found",
		[]byte("Title: Not found\nCategory: Error\n\n"), body)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write(a.injectRuntimeVars(compiled))
}

// serveNotEditable is the one answer for "you asked an editor to open a file
// that is not text". Same content negotiation as serveNotFound: a page for a
// browser navigation, plain text for a fetch - the editor's loadContent()
// shows that text to the user, so markup there would hide the reason.
//
// 415 Unsupported Media Type, not 404: the file exists and is served
// normally. Only the editor refuses it.
func (a *App) serveNotEditable(w http.ResponseWriter, r *http.Request, relPath string) {
	urlPath := "/" + strings.TrimPrefix(relPath, "/")
	ct := a.resolveContentType(relPath)

	a.logErrf(logEdit, "refused %s (%s): not a text file", urlPath, ct)

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
	compiled := a.compilePageWithBody("Not a text file",
		[]byte("Title: Not a text file\nCategory: Error\n\n"), body)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusUnsupportedMediaType)
	w.Write(a.injectRuntimeVars(compiled))
}

// notFoundInterceptor lets a handler we do not control (http.FileServer,
// behind serveStorageSubdir) keep its path-resolution and range handling
// while its 404 is replaced with ours. Wrapping the ResponseWriter rather
// than pre-checking the file avoids duplicating FileServer's traversal
// defences, which is exactly the code you do not want two copies of.
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
	// Swallow FileServer's own "404 page not found" body; serveNotFound has
	// already written a complete response. Reporting the full length keeps
	// the wrapped handler from treating this as a short write.
	if w.replaced {
		return len(b), nil
	}
	return w.ResponseWriter.Write(b)
}
