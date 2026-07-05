package backend

import (
	"bytes"
	"fmt"
	"log"
	"strings"
)

// ----------------------------------------------------------------------
// Why this file does NOT use html/template
// ----------------------------------------------------------------------
//
// An earlier revision rendered these pages through html/template. That was
// correct security-wise, but it carried a hidden, binary-wide cost:
// html/template (via text/template) calls reflect.Value.MethodByName,
// which forces the Go linker to DISABLE dead-code elimination for methods
// across the entire program - the linker can no longer prove any method
// unreachable, so it keeps all of them. The largest method surface in this
// binary by far is go-git (every transport, storage backend and plumbing
// type), most of which is normally pruned. With html/template linked, none
// of it was, which is what blew the binary up.
//
// What html/template actually bought us was context-correct escaping of a
// handful of known fields into a handful of known positions. This file
// keeps exactly that guarantee, but explicitly: each render function
// escapes each value with the escape function matching the context it is
// spliced into (HTML text/attribute vs. JS string literal), using a plain
// string Replacer - no reflection anywhere.
//
// The rules, applied below and worth keeping in mind when editing:
//   - escapeHTML(v)         for values inside HTML text or a quoted
//                           HTML attribute
//   - escapeJS(v)           for values inside a '...' or "..." JS string
//                           literal in an inline <script>
//   - escapeHTML(escapeJS(v)) for a JS string literal that itself lives
//                           inside an HTML attribute (e.g. onclick="...")
//   - pre-rendered trusted HTML (the markdown preview body, or fragments
//                           built by the render functions here) is spliced
//                           in as-is, never escaped twice
// ----------------------------------------------------------------------

// escapeHTML escapes a value for HTML text content or a double-quoted
// HTML attribute. (Same rules as the old a.htmlEscape; kept as a free
// function so this file has no receiver dependencies.)
func escapeHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&#39;")
	return s
}

// escapeJS escapes a value for use inside a single- or double-quoted
// JavaScript string literal in an inline <script> block. "<" and ">" are
// hex-escaped so no value can ever form a closing "</script>" and break
// out of the block.
func escapeJS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\'':
			b.WriteString(`\'`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '<':
			b.WriteString(`\x3c`)
		case '>':
			b.WriteString(`\x3e`)
		case '&':
			b.WriteString(`\x26`)
		case '\u2028':
			b.WriteString(`\u2028`)
		case '\u2029':
			b.WriteString(`\u2029`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// loadTemplate reads one page-fragment file out of templatesFS (declared
// in server.go - deliberately a separate embed from staticFS, whose
// frontend/html tree is lazily extracted to disk as user-editable content;
// these files must never be). A missing file is a packaging error, caught
// at first render rather than crashing startup.
func loadTemplate(filename string) string {
	data, err := templatesFS.ReadFile("frontend/templates/" + filename)
	if err != nil {
		log.Printf("templates: failed to read embedded %s: %v", filename, err)
		return "<p>Missing embedded template: " + escapeHTML(filename) + "</p>"
	}
	return string(data)
}

var (
	indexPageTmpl     = loadTemplate("index.html")
	configPageTmpl    = loadTemplate("config_page.html")
	gitServerCardTmpl = loadTemplate("git_server_card.html")
	externalEditTmpl  = loadTemplate("external_edit.html")
)

// fill replaces %%NAME%% placeholders in tmpl. Every value passed in MUST
// already be escaped for the context its placeholder sits in (see the
// rules at the top of this file); fill itself is escaping-agnostic on
// purpose, so trusted pre-rendered HTML fragments can pass through it too.
func fill(tmpl string, pairs map[string]string) string {
	oldnew := make([]string, 0, len(pairs)*2)
	for k, v := range pairs {
		oldnew = append(oldnew, "%%"+k+"%%", v)
	}
	return strings.NewReplacer(oldnew...).Replace(tmpl)
}

// --- Main page shell (index.html) ---

// metaTagView is one <meta name="..." content="..."> pulled from a page's
// markdown header block (plus the always-appended "generator" tag).
type metaTagView struct {
	Name  string
	Value string
}

// indexPageView holds everything renderIndexPage needs. PreviewHTML is
// trusted pre-rendered HTML (markdown output, or a fragment built by the
// other render functions in this file); every other field is a raw value
// that renderIndexPage escapes itself.
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
	PreviewHTML string
}

func renderIndexPage(v indexPageView) string {
	var metaTags strings.Builder
	for _, m := range v.MetaTags {
		fmt.Fprintf(&metaTags, "    <meta name=\"%s\" content=\"%s\" />\n",
			escapeHTML(m.Name), escapeHTML(m.Value))
	}

	condScripts := ""
	if v.IsMarkdown {
		condScripts += "    <script>var IS_MARKDOWN = true;</script>\n"
	}
	if v.IsEditMode {
		condScripts += "    <script>setTimeout(function(){ if(typeof toggleMode==='function') toggleMode(); }, 120);</script>\n"
	}

	var tags strings.Builder
	for _, t := range v.Tags {
		e := escapeHTML(t)
		fmt.Fprintf(&tags, `<a href="Tags.html#%s" class="taglink"><span class="tagmark">%s</span></a>`, e, e)
	}

	return fill(indexPageTmpl, map[string]string{
		"TITLE_HTML":   escapeHTML(v.Title),
		"TITLE_JS":     escapeJS(v.Title),
		"PACKAGE_JS":   escapeJS(v.PackageName),
		"PAGE_NAME_JS": escapeJS(v.PageName),
		"PAGE_EXT_JS":  escapeJS(v.PageExt),
		"META_TAGS":    metaTags.String(),
		"COND_SCRIPTS": condScripts,
		"TAGS_HTML":    tags.String(),
		"RAW_MD_HTML":  escapeHTML(v.RawMD),
		"PREVIEW_BODY": v.PreviewHTML,
	})
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

func renderConfigPage(v configPageView) string {
	var cards strings.Builder
	for _, gs := range v.GitServers {
		checked := ""
		if gs.Active {
			checked = "checked"
		}
		cards.WriteString(fill(gitServerCardTmpl, map[string]string{
			"INDEX":          fmt.Sprintf("%d", gs.Index),
			"SLOT":           fmt.Sprintf("%d", gs.Slot),
			"ACTIVE_CHECKED": checked,
			"NAME":           escapeHTML(gs.Name),
			"URL":            escapeHTML(gs.URL),
			"SSH_KEY":        escapeHTML(gs.SSHKeyData),
			"PASSWORD":       escapeHTML(gs.Password),
		}))
	}

	internalEdChecked := ""
	if v.UseInternalEd {
		internalEdChecked = "checked"
	}

	return fill(configPageTmpl, map[string]string{
		"SERVER_PORT":         fmt.Sprintf("%d", v.ServerPort),
		"ADMIN_PWD":           escapeHTML(v.AdminPassword),
		"GUEST_PWD":           escapeHTML(v.GuestPassword),
		"AUTHOR":              escapeHTML(v.Author),
		"INTERNAL_ED_CHECKED": internalEdChecked,
		"DESKTOP_EXT_CMD":     escapeHTML(v.DesktopExtCmd),
		"GIT_SERVERS":         cards.String(),
	})
}

// --- External Editor "waiting" page ---

type externalEditView struct {
	Cmd      string
	FileName string
	ViewURL  string
}

func renderExternalEditPage(v externalEditView) string {
	return fill(externalEditTmpl, map[string]string{
		"CMD":       escapeHTML(v.Cmd),
		"FILE_NAME": escapeHTML(v.FileName),
		// ViewURL sits inside a JS string literal which itself sits inside
		// an HTML onclick attribute: JS-escape first, then HTML-escape the
		// result (inner context first, outer second).
		"VIEW_URL_ATTR_JS": escapeHTML(escapeJS(v.ViewURL)),
	})
}

// --- Runtime variable injection into cached pages ---

// runtimeVarsMarker is a literal placeholder that index.html always emits
// once, near the end of <head>. It survives into the compiled .html files
// cached on disk.
const runtimeVarsMarker = `<meta id="omn-go-runtime-vars-marker">`

// injectRuntimeVars splices the two globals that must reflect the
// *currently running* server - not whatever was true when a page was last
// compiled to the on-disk HTML cache - into a rendered page's
// runtimeVarsMarker. Pages are cached to disk (precompileAllPages /
// serveHTMLPage's mtime check) so markdown isn't re-rendered per request,
// but APP_VERSION (bumped between releases) and UseInternalEd (toggleable
// at any time from Config) must always reflect *now*; recompiling every
// page whenever either changes would defeat the cache. Both values are
// server-controlled constants/booleans, never user input, so splicing them
// with fmt is safe.
func (a *App) injectRuntimeVars(page []byte) []byte {
	script := fmt.Sprintf(`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t;</script>`, APP_VERSION, a.GetConfig().UseInternalEd)
	return bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
}
