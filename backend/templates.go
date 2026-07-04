package backend

import (
	"bytes"
	"fmt"
	"html/template"
	"log"
)

// This file replaces what used to be fmt.Sprintf-built HTML in handlers.go
// (getConfigPageBody / getExternalEditPageBody). That approach had two
// problems: values like a git server's Name/URL or the config's Author
// field were spliced into HTML attributes with no escaping at all (a "
// or < in any of them would break the markup, and it's a stored-XSS vector
// against the admin's own config page), and every field needed its own
// inline `style="..."` block that duplicated rules already defined in
// omn-go-core.css.
//
// The actual markup lives in frontend/templates/*.html - real files, not
// Go string literals - and is embedded via templatesFS (declared in
// server.go). Those files are deliberately embedded separately from
// staticFS (frontend/html, frontend/md): that tree is lazily extracted to
// StorageDir/html and treated as user-editable content reachable via
// ?edit=true, which is exactly what must never happen to these templates.
//
// html/template gives us auto-escaping per output context for free
// (including understanding that {{.ViewURL}} inside
// onclick="...('/{{.ViewURL}}')" is a JS string literal, not just HTML),
// so every value below is safe to interpolate as-is.

// mustParseTemplate loads a single named template file out of templatesFS.
// Called only at package init (via the package-level vars below), so a
// missing/malformed template file is a build-time/startup problem - failing
// loudly via panic (through template.Must) is correct here, unlike
// runtime rendering errors which go through renderTemplate instead.
func mustParseTemplate(filename string) *template.Template {
	return template.Must(template.ParseFS(templatesFS, "frontend/templates/"+filename))
}

// renderTemplate executes tmpl with data and returns the resulting HTML, or
// a minimal error placeholder (logged, not swallowed) if execution fails.
// Execution failures here mean a template/data mismatch - a programmer
// error, not a per-request condition - so logging plus a visible fallback
// is more useful than silently returning an empty string.
func renderTemplate(tmpl *template.Template, data any, caller string) string {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("%s: template execution failed: %v", caller, err)
		return "<p>Failed to render this page. See server logs for details.</p>"
	}
	return buf.String()
}

// --- Configuration Dashboard ---

type gitServerView struct {
	Index      int
	Slot       int
	Active     bool
	Name       string
	URL        string
	SSHKeyData string
	Password   string
}

type configPageView struct {
	ServerPort    int
	AdminPassword string
	GuestPassword string
	Author        string
	UseInternalEd bool
	DesktopExtCmd string
	GitServers    []gitServerView
}

var configPageTmpl = mustParseTemplate("config_page.html")

// --- External Editor "waiting" page ---

type externalEditView struct {
	Cmd      string
	FileName string
	ViewURL  string
}

var externalEditTmpl = mustParseTemplate("external_edit.html")

// --- Main page shell (index.html) ---
//
// This is the one template rendered for every note/page/asset-edit view.
// It replaces what used to be a raw embedded frontend/index.html with a
// dozen strings.ReplaceAll(...) calls against HTML-comment placeholders
// (<!-- OMN_GO_PAGE_TITLE -->, <!-- OMN_GO_TAGS -->, ...) in markdown.go.
// That approach had no escaping at all: a page title or tag containing a
// quote or "<" could break the surrounding HTML or JS, and RawMD relied on
// a separate manual a.htmlEscape() call that was easy to forget to pair
// with the right placeholder.
//
// html/template's contextual escaping now does all of that automatically
// and correctly for each field's actual context - see indexPageTmpl below:
// PageName is escaped once as an HTML attribute (meta script vars) and
// again as a single-quoted JS string literal (var PageName = '...'), RawMD
// is escaped for a <textarea> body, PreviewBody is trusted pre-rendered
// HTML (template.HTML) and is deliberately NOT escaped, and Tags are
// escaped appropriately for both the href fragment and the link text.

// metaTagView is one <meta name="..." content="..."> pulled from a page's
// markdown header block (plus the always-appended "generator" tag).
type metaTagView struct {
	Name  string
	Value string
}

// indexPageView holds everything indexPageTmpl needs. PreviewBody is
// template.HTML (trusted, pre-rendered markdown/edit-view HTML) - every
// other field is a plain string/bool/slice that html/template escapes
// itself, matching the context each is used in.
type indexPageView struct {
	Title       string
	PackageName string
	PageName    string
	PageExt     string
	IsMarkdown  bool
	IsEditMode  bool
	MetaTags    []metaTagView
	Tags        []string
	RawMD       string
	PreviewBody template.HTML
}

var indexPageTmpl = mustParseTemplate("index.html")

// runtimeVarsMarker is a literal (non-template-action) placeholder that
// indexPageTmpl always emits once, near the end of <head>.
const runtimeVarsMarker = "<!-- OMN_GO_RUNTIME_VARS -->"

// injectRuntimeVars splices the two globals that must reflect the
// *currently running* server - not whatever was true when a page was last
// compiled to the on-disk HTML cache - into a rendered page's
// runtimeVarsMarker.
//
// This is the one narrow spot that still does manual string substitution
// after html/template has already run, and that's intentional: pages are
// cached to disk (see precompileAllPages / serveHTMLPage's mtime check) so
// that expensive markdown rendering isn't redone on every request, but
// APP_VERSION (bumped between app releases) and UseInternalEd (toggleable
// at any time from the Config page) must always reflect *now*, not
// whatever was true when that cached copy was compiled. Recompiling every
// page on every config change or version bump just to update two booleans
// would defeat the point of the cache.
//
// fmt.Sprintf is safe here specifically because neither value is
// user/attacker-controlled: APP_VERSION is a Go build constant and
// UseInternalEd is a server-side config bool, unlike the untrusted
// git-server/page-name values that now go through indexPageTmpl/
// configPageTmpl instead.
func (a *App) injectRuntimeVars(page []byte) []byte {
	script := fmt.Sprintf(`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t;</script>`, APP_VERSION, a.GetConfig().UseInternalEd)
	return bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
}
