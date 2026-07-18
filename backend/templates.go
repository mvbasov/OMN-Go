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
	editorPageTmpl    = loadTemplate("editor.html")
	// modalsHTML is the block of server-only modals (login, quick note,
	// bookmark, commit, conflict). It is kept OUT of the cached/exported
	// page (index.html carries only the modalsMarker slot) and spliced in by
	// injectRuntimeVars at serve time, so an offline/exported page - which
	// has no backend and no use for these server features - stays small.
	modalsHTML = loadTemplate("modals.html")
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
	IsAndroid   bool
	AssetPrefix string // "", "../", "../../", … or "/" — see compilePageWithBody
	MetaTags    []metaTagView
	Tags        []string
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
	if v.IsAndroid {
		condScripts += "    <script>var IS_ANDROID = true;</script>\n"
	}

	var tags strings.Builder
	for _, t := range v.Tags {
		// All pills point at the one generated Tags page (OMNGoTags), reached
		// relatively via AssetPrefix ("", "../", ...) so the link resolves from
		// any directory depth both online and offline (file://). The fragment
		// is tagSlug(t) - the same slug the generated page uses for its section
		// ids (see tags.go), so both are computed by one Go function and can't
		// drift. AssetPrefix carries only "./" characters, so it needs no
		// escaping (mirrors its ASSET_PREFIX use below).
		fmt.Fprintf(&tags, `<a href="%sOMNGoTags.html#%s" class="taglink"><span class="tagmark">%s</span></a>`,
			v.AssetPrefix, escapeHTML(tagSlug(t)), escapeHTML(t))
	}

	return fill(indexPageTmpl, map[string]string{
		"TITLE_HTML":   escapeHTML(v.Title),
		"TITLE_JS":     escapeJS(v.Title),
		"PACKAGE_JS":   escapeJS(v.PackageName),
		"PAGE_NAME_JS": escapeJS(v.PageName),
		"PAGE_EXT_JS":  escapeJS(v.PageExt),
		// Server-computed path prefix ("", "../", "/"), spliced into href/src
		// attributes; contains only "./" characters, no escaping needed.
		"ASSET_PREFIX": v.AssetPrefix,
		"META_TAGS":    metaTags.String(),
		"COND_SCRIPTS": condScripts,
		"TAGS_HTML":    tags.String(),
		"PREVIEW_BODY": v.PreviewHTML,
	})
}

// --- Standalone note editor page (editor.html) ---

// editorPageView holds everything renderEditorPage needs. All fields are
// raw values escaped here for the context each is spliced into. The note's
// text is intentionally absent: the editor fetches it from /api/note at
// editing start, so a rendered page never carries a second copy of itself.
type editorPageView struct {
	Title   string // display name (page/asset)
	Name    string // value for /api/note and /api/save
	PageExt string // e.g. ".md", ".js" (informational)
	ViewURL string // where to return after save/cancel
}

func renderEditorPage(v editorPageView) string {
	return fill(editorPageTmpl, map[string]string{
		"TITLE_HTML":  escapeHTML(v.Title),
		"NAME_JS":     escapeJS(v.Name),
		"PAGE_EXT_JS": escapeJS(v.PageExt),
		// Only consumed by JS (OMN_EDIT_VIEW) now that the redundant back
		// link is gone; the × button navigates here via omn-go-editor.js.
		"VIEW_URL_JS": escapeJS(v.ViewURL),
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
	ServerPort         int
	AdminPassword      string
	GuestPassword      string
	Author             string
	UseInternalEd      bool
	DesktopExtCmd      string
	Theme              string // "auto" | "light" | "dark" (normalized)
	ShareLAN           bool
	Hostname           string
	PruneDepth         int
	MaxUploadSizeMB    int
	EnableIntentURI    bool
	EnableTermuxIntent bool
	GitServers         []gitServerView
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
	shareLanChecked := ""
	if v.ShareLAN {
		shareLanChecked = "checked"
	}
	intentUriChecked := ""
	if v.EnableIntentURI {
		intentUriChecked = "checked"
	}
	termuxIntentChecked := ""
	if v.EnableTermuxIntent {
		termuxIntentChecked = "checked"
	}

	// Exactly one option is marked selected; normalizeTheme guarantees
	// the value is one of the three, with unknown/empty mapping to auto.
	themeSel := map[string]string{
		"THEME_AUTO_SEL":  "",
		"THEME_LIGHT_SEL": "",
		"THEME_DARK_SEL":  "",
	}
	switch normalizeTheme(v.Theme) {
	case ThemeLight:
		themeSel["THEME_LIGHT_SEL"] = "selected"
	case ThemeDark:
		themeSel["THEME_DARK_SEL"] = "selected"
	default:
		themeSel["THEME_AUTO_SEL"] = "selected"
	}

	return fill(configPageTmpl, map[string]string{
		"SERVER_PORT":           fmt.Sprintf("%d", v.ServerPort),
		"ADMIN_PWD":             escapeHTML(v.AdminPassword),
		"GUEST_PWD":             escapeHTML(v.GuestPassword),
		"AUTHOR":                escapeHTML(v.Author),
		"INTERNAL_ED_CHECKED":   internalEdChecked,
		"SHARE_LAN_CHECKED":     shareLanChecked,
		"INTENT_URI_CHECKED":    intentUriChecked,
		"TERMUX_INTENT_CHECKED": termuxIntentChecked,
		"DESKTOP_EXT_CMD":       escapeHTML(v.DesktopExtCmd),
		"HOSTNAME":              escapeHTML(displayHostname(v.Hostname)),
		"BACKUP_PRUNE_DEPTH":    fmt.Sprintf("%d", displayPruneDepth(v.PruneDepth)),
		"THEME_AUTO_SEL":        themeSel["THEME_AUTO_SEL"],
		"THEME_LIGHT_SEL":       themeSel["THEME_LIGHT_SEL"],
		"THEME_DARK_SEL":        themeSel["THEME_DARK_SEL"],
		"MAX_UPLOAD_MB":         fmt.Sprintf("%d", v.MaxUploadSizeMB),
		"GIT_SERVERS":           cards.String(),
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

// modalsMarker is the empty slot index.html emits where the server-only
// modals go. injectRuntimeVars replaces it with modalsHTML when the backend
// serves the page; on an exported/offline page (no backend) it stays as an
// empty div, so those modals simply don't exist there.
const modalsMarker = `<div id="omn-go-modals-slot"></div>`

// injectRuntimeVars splices the globals that must reflect the *currently
// running* server - not whatever was true when a page was last compiled
// to the on-disk HTML cache - into a rendered page's runtimeVarsMarker.
// Pages are cached to disk (precompileAllPages / serveHTMLPage's mtime
// check) so markdown isn't re-rendered per request, but APP_VERSION
// (bumped between releases), UseInternalEd and Theme (both toggleable at
// any time from Config) must always reflect *now*; recompiling every page
// whenever any of them changes would defeat the cache.
//
// The theme is applied by setting data-theme on <html> right here rather
// than baking a class into the markup: the marker sits inside <head>, so
// this script runs before the body is painted - no flash of the wrong
// theme - and it works identically for pages compiled long before the
// theme was changed. The CSS handles the rest: an explicit "light"/"dark"
// value pins the palette, while "auto" (or a missing attribute, e.g. an
// exported page opened via file:// where this marker is never replaced)
// falls through to the prefers-color-scheme media query.
//
// All values are server-controlled (APP_VERSION is a build constant,
// UseInternalEd a bool, Theme whitelisted through normalizeTheme), never
// user input, so splicing them with fmt is safe.
func (a *App) injectRuntimeVars(page []byte) []byte {
	cfg := a.GetConfig()
	script := fmt.Sprintf(
		`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t; var OMN_THEME = %q; document.documentElement.setAttribute('data-theme', OMN_THEME);</script>`,
		APP_VERSION, cfg.UseInternalEd, normalizeTheme(cfg.Theme))
	page = bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
	// Splice the server-only modals into the slot (a no-op on templates that
	// don't carry it, e.g. the standalone editor page).
	page = bytes.Replace(page, []byte(modalsMarker), []byte(modalsHTML), 1)
	return page
}
