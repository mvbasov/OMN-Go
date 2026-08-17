package backend

import (
	"bytes"
	"fmt"
	"log"
	"net/url"
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

// incomingIndexTmpl is the incoming index as first written: a header block,
// the receive box the desktop application imports through, and the marker
// that says where a new line goes. See incomingIndexStarter in
// note_exchange.go for why it lives here and not in frontend/md/.
var incomingIndexTmpl = loadTemplate("incoming_index.md")

var (
	// index.html loads css/omn-go-custom.css as the last stylesheet and
	// js/omn-go-custom.js as the last script. The position is the feature:
	// a user rule beats an application rule of the same specificity, and
	// the user script sees everything the application scripts define. Both
	// files are user-owned - they are NOT in versionDependentAssets, so
	// materializeAsset creates each one from the embedded copy on first
	// request and no upgrade replaces it. editor.html loads neither, so a
	// bad rule or a script error can never keep the user out of the editor
	// that repairs it. Do not put these notes in the template itself: it
	// ships to the browser with every page.
	indexPageTmpl     = loadTemplate("index.html")
	configPageTmpl    = loadTemplate("config_page.html")
	gitServerCardTmpl = loadTemplate("git_server_card.html")
	externalEditTmpl  = loadTemplate("external_edit.html")
	editorPageTmpl    = loadTemplate("editor.html")
	notFoundTmpl      = loadTemplate("not_found.html")
	notEditableTmpl   = loadTemplate("not_editable.html")
	statusPageTmpl    = loadTemplate("status_page.html")
	searchPageTmpl    = loadTemplate("search_page.html")
	filesPageTmpl     = loadTemplate("files_page.html")
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
	AndroidFullscreen  string // "off" | "fullscreen" | "immersive" (normalized)
	SearchEnabled      bool
	SearchKinds        []string // normalized
	SearchBundled      bool
	SearchScope        string // "all" | "page" (normalized)
	SearchIndexStatus  string // human-readable line for the Search screen
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
	searchEnabledChecked := ""
	if v.SearchEnabled {
		searchEnabledChecked = "checked"
	}
	searchBundledChecked := ""
	if v.SearchBundled {
		searchBundledChecked = "checked"
	}
	// One checkbox per kind, checked when the kind is in the normalized list.
	kindChecked := map[string]string{}
	for _, k := range v.SearchKinds {
		kindChecked[k] = "checked"
	}
	searchScopeAllSel, searchScopePageSel := "checked", ""
	if normalizeSearchScope(v.SearchScope) == SearchScopePage {
		searchScopeAllSel, searchScopePageSel = "", "checked"
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

	// Exactly one option is marked selected; normalizeFullscreen guarantees
	// the value is one of the three, with unknown/empty mapping to
	// FullscreenOn (see config.go for why that, not "off", is the default).
	fsSel := map[string]string{
		"FS_OFF_SEL":       "",
		"FS_ON_SEL":        "",
		"FS_IMMERSIVE_SEL": "",
	}
	switch normalizeFullscreen(v.AndroidFullscreen) {
	case FullscreenOff:
		fsSel["FS_OFF_SEL"] = "selected"
	case FullscreenImmersive:
		fsSel["FS_IMMERSIVE_SEL"] = "selected"
	default:
		fsSel["FS_ON_SEL"] = "selected"
	}

	return fill(configPageTmpl, map[string]string{
		"SERVER_PORT":            fmt.Sprintf("%d", v.ServerPort),
		"ADMIN_PWD":              escapeHTML(v.AdminPassword),
		"GUEST_PWD":              escapeHTML(v.GuestPassword),
		"AUTHOR":                 escapeHTML(v.Author),
		"INTERNAL_ED_CHECKED":    internalEdChecked,
		"SHARE_LAN_CHECKED":      shareLanChecked,
		"INTENT_URI_CHECKED":     intentUriChecked,
		"TERMUX_INTENT_CHECKED":  termuxIntentChecked,
		"DESKTOP_EXT_CMD":        escapeHTML(v.DesktopExtCmd),
		"HOSTNAME":               escapeHTML(displayHostname(v.Hostname)),
		"BACKUP_PRUNE_DEPTH":     fmt.Sprintf("%d", displayPruneDepth(v.PruneDepth)),
		"THEME_AUTO_SEL":         themeSel["THEME_AUTO_SEL"],
		"THEME_LIGHT_SEL":        themeSel["THEME_LIGHT_SEL"],
		"THEME_DARK_SEL":         themeSel["THEME_DARK_SEL"],
		"MAX_UPLOAD_MB":          fmt.Sprintf("%d", v.MaxUploadSizeMB),
		"FS_OFF_SEL":             fsSel["FS_OFF_SEL"],
		"FS_ON_SEL":              fsSel["FS_ON_SEL"],
		"FS_IMMERSIVE_SEL":       fsSel["FS_IMMERSIVE_SEL"],
		"SEARCH_ENABLED_CHECKED": searchEnabledChecked,
		"SEARCH_BUNDLED_CHECKED": searchBundledChecked,
		"SEARCH_KIND_MD":         kindChecked[SearchKindMD],
		"SEARCH_KIND_BOOKMARKS":  kindChecked[SearchKindBookmarks],
		"SEARCH_KIND_JS":         kindChecked[SearchKindJS],
		"SEARCH_KIND_JSON":       kindChecked[SearchKindJSON],
		"SEARCH_KIND_USER_JSON":  kindChecked[SearchKindUserJSON],
		"SEARCH_SCOPE_ALL_SEL":   searchScopeAllSel,
		"SEARCH_SCOPE_PAGE_SEL":  searchScopePageSel,
		"SEARCH_INDEX_STATUS":    escapeHTML(v.SearchIndexStatus),
		"GIT_SERVERS":            cards.String(),
	})
}

// --- 404 page ---

// notFoundView is everything the detailed 404 page shows. Every field is a
// RAW value that renderNotFoundPage escapes itself - URL and Referer in
// particular are attacker-controlled (anyone can request any path, and
// Referer is a plain request header), so they must never reach the output
// unescaped. This file deliberately does not use html/template (see the
// note at the top of this file for why), which makes that this function's
// responsibility rather than the template engine's.
type notFoundView struct {
	URL       string // path + query, exactly as requested
	Method    string
	Time      string
	Referer   string // "" when absent or not from this server
	Suggested string // "" when there is no plausible alternative
}

// safeLocalPath reports whether s may be used as an href: it must be a
// path on this server. A scheme-bearing value ("javascript:...") or a
// protocol-relative one ("//host/...") is rejected, so no caller of
// renderNotFoundPage can turn a request header into an active link off the
// app - defence in depth behind serveNotFound, which already filters the
// Referer it passes in.
func safeLocalPath(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")
}

// notEditableView holds what the "not a text file" page shows. Path and
// Type are raw values that renderNotEditablePage escapes itself.
type notEditableView struct {
	Path string // "/css/fonts/x.woff2"
	Type string // the resolved content type, "unknown" when there is none
}

// renderNotEditablePage builds the body of the page an editor route answers
// with when the file is not text (see serveEditor). The view link is the
// same path without the edit query, so the user reaches the file itself.
func renderNotEditablePage(v notEditableView) string {
	typ := v.Type
	if typ == "" {
		typ = "unknown"
	}
	return fill(notEditableTmpl, map[string]string{
		"PATH":     escapeHTML(v.Path),
		"TYPE":     escapeHTML(typ),
		"VIEW_URL": escapeHTML(v.Path),
	})
}

func renderNotFoundPage(v notFoundView) string {
	// Optional blocks are built here as trusted pre-rendered HTML (the
	// convention documented at the top of this file): the values inside are
	// escaped as they are spliced in, and the surrounding markup is ours.
	refererRows := ""
	if v.Referer != "" {
		esc := escapeHTML(v.Referer)
		if safeLocalPath(v.Referer) {
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd><a href="%s">%s</a> &middot; <a href="%s?edit=true">edit that page</a></dd>
`, esc, esc, esc)
		} else {
			// Still worth reporting, but never as a link: escapeHTML makes
			// it inert as text, whereas a "javascript:" or "//evil.example"
			// value in an href would stay live.
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd>%s</dd>
`, esc)
		}
	}

	suggestion := ""
	if v.Suggested != "" && safeLocalPath(v.Suggested) {
		esc := escapeHTML(v.Suggested)
		suggestion = fmt.Sprintf(`    <div class="config-field notfound-suggest">
        <span class="notfound-suggest-label">Did you mean</span>
        <a href="%s" class="notfound-suggest-link">%s</a>
        <span class="config-hint">A note of that name exists. A link written as [text](name) asks the server for a file called "name"; note links need the .html suffix - [text](name.html).</span>
    </div>
`, esc, esc)
	}

	return fill(notFoundTmpl, map[string]string{
		"URL":          escapeHTML(v.URL),
		"METHOD":       escapeHTML(v.Method),
		"TIME":         escapeHTML(v.Time),
		"REFERER_ROWS": refererRows,
		"SUGGESTION":   suggestion,
	})
}

// --- File index page (files_page.html, see files_index.go) ---

// filesCrumb is one step of the breadcrumb. Dir is what ?dir= should become.
type filesCrumb struct {
	Label string
	Dir   string
}

// filesTreeCard is one button of the first screen.
type filesTreeCard struct {
	Key   string
	Icon  string
	Title string
	Where string
	Count string
	Class string
}

// filesLegendItem is one line of the key under the crumb.
type filesLegendItem struct {
	Color string
	Word  string
	Text  string
}

// filesDirRow is a subdirectory of the directory being shown. Files and Bytes
// are RECURSIVE totals for that subtree, and one name counts one time even
// when it is on both sides. The four flags answer "is this whole subtree of
// one kind": see (*filesDirRow).note in files_index.go.
type filesDirRow struct {
	Name        string
	Dir         string
	Files       int
	Bytes       int64
	anyShips    bool
	anyDevice   bool
	everyShips  bool
	everyDevice bool
}

// filesFileRow is one NAME of the tree in view. Every field is a raw value
// that renderFilesPage escapes itself - names come from uploads and note
// titles, so they are user-controlled and this file assembles HTML by hand.
//
// State is the word on the first line and says what the file is. StateColor
// and OwnerColor are the classes that say what happens to it. See the block
// comment of files_index.go for the two channels.
type filesFileRow struct {
	Name       string
	Path       string
	URL        string
	EditURL    string // "" when the row offers no edit link
	Kind       string // a Material Icons ligature
	Size       string
	Mod        string // "" for a file that is not on the device
	ModFull    string
	State      string
	StateColor string
	AppOwned   bool
	OwnerColor string
	Extra      []string
}

type filesPageView struct {
	Tree       string // "" on the first screen
	Dir        string
	Crumbs     []filesCrumb
	Cards      []filesTreeCard
	Legend     []filesLegendItem
	Summary    string
	Dirs       []filesDirRow
	Files      []filesFileRow
	Total      int // files directly in this directory, before the cap
	Hidden     int // ... how many of them are not shown
	Empty      bool
	ShowingAll bool
	Denied     bool
}

// filesDeniedNotice is what a non-admin sees.
//
// A page, not the bare 401 authMiddleware would produce: this address is
// linkable, and a refusal that names neither the reason nor the remedy is the
// dead end the search page's 404 turned out to be (26.08.2). Static markup, no
// interpolation - nothing here can carry a value in from a request, and in
// particular no filename appears anywhere in this response.
const filesDeniedNotice = `<div class="files-notice">` +
	`<h2>Administrator only</h2>` +
	`<p>This page lists the files stored on the device, so it is shown only ` +
	`to an administrator.</p>` +
	`<p class="files-note">Log in from any note page - the account button in ` +
	`the page header - and come back. A connection from the device itself is ` +
	`always treated as the owner; this only applies to other machines on the ` +
	`network.</p>` +
	`</div>`

// filesOwnerHint is the tooltip of the app-owned mark.
const filesOwnerHint = "The next version of OMN-Go backs up your copy and replaces it"

func renderFilesPage(v filesPageView) string {
	if v.Denied {
		return fill(filesPageTmpl, map[string]string{
			"DENIED": " is-denied",
			"NOTICE": filesDeniedNotice,
			"BODY":   "",
		})
	}
	if v.Tree == "" {
		return fill(filesPageTmpl, map[string]string{
			"DENIED": "",
			"NOTICE": "",
			"BODY":   renderFilesCards(v),
		})
	}
	return fill(filesPageTmpl, map[string]string{
		"DENIED": "",
		"NOTICE": "",
		"BODY":   renderFilesListing(v),
	})
}

// renderFilesCards is the first screen: three buttons, one column at every
// width. A wide screen gets a narrower page rather than three columns - one
// layout to build, one to test, and the three targets stay the size of a
// thumb.
func renderFilesCards(v filesPageView) string {
	var b strings.Builder
	b.WriteString(`<div class="files-cards">`)
	for _, c := range v.Cards {
		fmt.Fprintf(&b, `<a class="files-card %s" href="%s">`+
			`<i class="material-icons files-card-icon">%s</i>`+
			`<span class="files-card-text">`+
			`<span class="files-card-title">%s</span>`+
			`<span class="files-card-where">%s</span>`+
			`<span class="files-card-count">%s</span>`+
			`</span></a>`,
			escapeHTML(c.Class), escapeHTML(filesPageURL(c.Key, "", false)),
			escapeHTML(c.Icon), escapeHTML(c.Title), escapeHTML(c.Where),
			escapeHTML(c.Count))
	}
	b.WriteString(`</div>`)
	return b.String()
}

// filesPageURL builds a link back into this page. Only three parameters exist,
// and all of them are produced here rather than anywhere in a template, so
// none can be spliced from a request value.
func filesPageURL(tree, dir string, all bool) string {
	out := "/OMNGoFiles.html"
	sep := "?"
	if tree != "" {
		out += sep + "tree=" + url.QueryEscape(tree)
		sep = "&"
	}
	if dir != "" {
		out += sep + "dir=" + url.QueryEscape(dir)
		sep = "&"
	}
	if all {
		out += sep + "all=1"
	}
	return out
}

func renderFilesListing(v filesPageView) string {
	var b strings.Builder

	// The crumb. Each label carries its own slash and nothing goes between
	// two of them, so the trail reads as the path it is: html/js/ .
	b.WriteString(`<div class="files-crumbs">`)
	for i, c := range v.Crumbs {
		if i == len(v.Crumbs)-1 {
			fmt.Fprintf(&b, `<span class="files-crumb-here">%s</span>`, escapeHTML(c.Label))
			continue
		}
		fmt.Fprintf(&b, `<a href="%s">%s</a>`,
			escapeHTML(filesPageURL(v.Tree, c.Dir, false)), escapeHTML(c.Label))
	}
	b.WriteString(`</div>`)

	fmt.Fprintf(&b, `<p class="files-summary">%s</p>`, escapeHTML(v.Summary))

	if len(v.Legend) > 0 {
		b.WriteString(`<div class="files-legend">`)
		for _, item := range v.Legend {
			fmt.Fprintf(&b, `<div><b class="%s">%s</b> — %s</div>`,
				escapeHTML(item.Color), escapeHTML(item.Word), escapeHTML(item.Text))
		}
		b.WriteString(`</div>`)
	}

	if v.Empty {
		b.WriteString(`<p class="files-empty">This directory holds nothing.</p>`)
		return b.String()
	}

	b.WriteString(`<ul class="files-list">`)
	for _, d := range v.Dirs {
		fmt.Fprintf(&b, `<li class="files-row files-dir">`+
			`<span class="files-name"><i class="material-icons files-kind">folder</i>`+
			`<a href="%s">%s</a></span>`,
			escapeHTML(filesPageURL(v.Tree, d.Dir, false)), escapeHTML(d.Name+"/"))
		if word, color := filesDirNote(v.Tree, d); word != "" {
			fmt.Fprintf(&b, `<span class="files-state %s">%s</span>`,
				escapeHTML(color), escapeHTML(word))
		}
		fmt.Fprintf(&b, `<span class="files-facts"><span class="files-size">%s · %s</span>`+
			`</span></li>`,
			escapeHTML(filesCountLabel(d.Files)), escapeHTML(filesSize(d.Bytes)))
	}
	for _, f := range v.Files {
		renderFilesRow(&b, f)
	}
	b.WriteString(`</ul>`)

	if v.Hidden > 0 {
		fmt.Fprintf(&b, `<p class="files-more">%s not shown `+
			`<a href="%s">show all %s &rarr;</a></p>`,
			escapeHTML(itoa(v.Hidden)),
			escapeHTML(filesPageURL(v.Tree, v.Dir, true)),
			escapeHTML(itoa(v.Total)))
	}
	return b.String()
}

// renderFilesRow writes one row: the name and the state on the first line, the
// facts on the second. The name owns the first line, thus no name is ever
// squeezed into a column of two characters.
func renderFilesRow(b *strings.Builder, f filesFileRow) {
	b.WriteString(`<li class="files-row">`)
	fmt.Fprintf(b, `<span class="files-name">`+
		`<i class="material-icons files-kind">%s</i><a href="%s">%s</a></span>`,
		escapeHTML(f.Kind), escapeHTML(f.URL), escapeHTML(f.Name))
	if f.State != "" {
		fmt.Fprintf(b, `<span class="files-state %s">%s</span>`,
			escapeHTML(f.StateColor), escapeHTML(f.State))
	}
	b.WriteString(`<span class="files-facts">`)
	fmt.Fprintf(b, `<span class="files-size">%s</span>`, escapeHTML(f.Size))
	if f.Mod != "" {
		// The date only. The hour and the minute made the row too wide for a
		// phone. The full time stays in the title.
		fmt.Fprintf(b, `<span class="files-meta" title="%s">%s</span>`,
			escapeHTML(f.ModFull), escapeHTML(f.Mod))
	}
	// The ownership word is on the second line of every row that has it, in
	// each of the three trees. Colour is a hint; this word is the fact.
	if f.AppOwned {
		fmt.Fprintf(b, `<span class="files-meta %s" title="%s">app-owned</span>`,
			escapeHTML(f.OwnerColor), escapeHTML(filesOwnerHint))
	}
	for _, extra := range f.Extra {
		fmt.Fprintf(b, `<span class="files-meta">%s</span>`, escapeHTML(extra))
	}
	if f.EditURL != "" {
		fmt.Fprintf(b, `<a class="files-edit" href="%s">edit</a>`, escapeHTML(f.EditURL))
	}
	b.WriteString(`</span></li>`)
}

// filesDirNote gives the one word a directory row can carry: a subtree that is
// entirely shipped, or entirely made on the device, says so. A mixed subtree
// says nothing, because the rows inside it answer better.
//
// The device-only word differs by tree, and the reason is html/Test/: in the
// Served tree a directory of compiled pages is not the user's work, so the
// word there is the neutral "not shipped". In the Source tree a directory that
// does not ship IS the user's, so it says "yours".
func filesDirNote(tree string, d filesDirRow) (word, color string) {
	if tree == filesTreeBundled {
		return "", ""
	}
	switch {
	case d.everyShips && !d.anyDevice:
		return "not extracted", filesColorPlain
	case d.everyDevice && !d.anyShips:
		if tree == filesTreeSource {
			return "yours", filesColorKeep
		}
		return "not shipped", filesColorPlain
	}
	return "", ""
}

// --- Search results page (search_page.html) ---

// searchPageView is everything renderSearchPage needs. Query is RAW - it is
// whatever someone typed into a URL, so it is escaped here for both the
// attribute and the text contexts it lands in. Results carry the same data the
// API returns; the snippet text and its spans are turned into <mark> markup by
// renderSnippetHTML, which escapes as it goes.
type searchPageView struct {
	Query        string
	Results      []searchResult
	Total        int
	Truncated    bool
	IndexedKinds []string // what the index currently covers, for the empty state
	Highlight    []string // query terms, hung off every result link as ?hl=
	Disabled     bool     // global search is switched off: explain, do not search
}

// searchKindLabel is the human name of a kind, used for the group headings.
func searchKindLabel(kind string) string {
	switch kind {
	case SearchKindMD:
		return "Notes"
	case SearchKindBookmarks:
		return "Bookmarks"
	case SearchKindJS:
		return "Scripts"
	case SearchKindJSON:
		return "JSON"
	case SearchKindUserJSON:
		return "Uploaded JSON"
	default:
		return kind
	}
}

// renderSnippetHTML splices <mark> around each span of a snippet.
//
// Spans are RUNE offsets (the Go side works in runes so Cyrillic is never cut
// in half), so the text is walked as []rune rather than sliced by byte. Every
// segment is escaped as it is emitted - the only markup in the result is the
// <mark> tags this function writes itself, which is the escaping contract at
// the top of this file applied to text that comes from the user's own notes.
func renderSnippetHTML(text string, spans [][2]int) string {
	runes := []rune(text)
	var b strings.Builder
	at := 0
	for _, sp := range spans {
		start, length := sp[0], sp[1]
		if start < at || length <= 0 || start+length > len(runes) {
			continue
		}
		if start > at {
			b.WriteString(escapeHTML(string(runes[at:start])))
		}
		b.WriteString(`<mark class="omn-search-hit">`)
		b.WriteString(escapeHTML(string(runes[start : start+length])))
		b.WriteString(`</mark>`)
		at = start + length
	}
	if at < len(runes) {
		b.WriteString(escapeHTML(string(runes[at:])))
	}
	return b.String()
}

// searchDisabledNotice is what the page says when global search is off.
//
// It used to be a 404, on the reasoning that a permanently empty results page
// is worse than an honest miss. That was wrong about who arrives here: the page
// is linkable, and people put a "Search" link on their Welcome note - so the
// address is permanent navigation and the 404 is a dead end that names neither
// the cause nor the cure.
//
// Static markup, no interpolation: everything here is fixed text and one
// internal link, so there is nothing to escape and nothing that can carry a
// value in from a request.
const searchDisabledNotice = `<div class="search-page-notice">` +
	`<h2>Global search is off</h2>` +
	`<p>Searching every note at once needs an index, and the index is held in ` +
	`memory for as long as the app runs - roughly a third of the size of the ` +
	`text it covers. That is a real cost on a small device, so it is off until ` +
	`you ask for it.</p>` +
	`<p><a class="search-page-cta" href="/Config.html#cfg-search">` +
	`Turn on global search in Settings</a></p>` +
	`<p class="search-page-note">There, <em>Enable global search</em> switches ` +
	`it on and the checkboxes under it choose what gets indexed - notes and ` +
	`bookmarks to begin with. It applies immediately; no restart.</p>` +
	`<p class="search-page-note">Searching the note you have open needs none of ` +
	`this and always works: the magnifier in the page header, or ` +
	`<kbd>Ctrl</kbd>+<kbd>K</kbd>.</p>` +
	`</div>`

func renderSearchPage(v searchPageView) string {
	if v.Disabled {
		// Every other slot stays empty: no form, because submitting it would
		// only land back here, and no results section to look mysteriously
		// blank underneath the explanation.
		return fill(searchPageTmpl, map[string]string{
			"DISABLED": " is-disabled",
			"NOTICE":   searchDisabledNotice,
			"QUERY":    "",
			"SUMMARY":  "",
			"GROUPS":   "",
			"EMPTY":    "",
		})
	}

	var groups strings.Builder

	// Grouped by kind, in a fixed order rather than by score, so the page has
	// a stable shape a reader can scan. Within a group the server's ranking is
	// preserved.
	for _, kind := range searchKindsAll {
		var inKind []searchResult
		for _, r := range v.Results {
			if r.Kind == kind {
				inKind = append(inKind, r)
			}
		}
		if len(inKind) == 0 {
			continue
		}
		fmt.Fprintf(&groups, "<h2 class=\"search-group\">%s <span class=\"search-group-count\">%d</span></h2>\n",
			escapeHTML(searchKindLabel(kind)), len(inKind))

		for _, r := range inKind {
			groups.WriteString("<div class=\"search-result\">\n")
			title := r.Title
			if title == "" {
				title = r.Name
			}
			// The link carries the query as ?hl= so the note highlights and
			// scrolls to the match on arrival, instead of dropping the reader at
			// the top of a long page to find it again by eye. The client strips
			// the parameters once applied, so the URL left in the address bar -
			// the one that gets copied or bookmarked - is the plain one.
			fmt.Fprintf(&groups, "  <a class=\"search-result-title\" href=\"%s\">%s</a>\n",
				escapeHTML(highlightURL(r.URL, v.Highlight)), escapeHTML(title))
			fmt.Fprintf(&groups, "  <div class=\"search-result-path\">%s</div>\n", escapeHTML(r.Name))

			if len(r.Tags) > 0 {
				groups.WriteString("  <div class=\"search-result-tags\">")
				for _, t := range r.Tags {
					// The same pill markup and the same anchor contract as a
					// page header (see renderIndexPage), so a tag means the
					// same thing and goes to the same place wherever it is
					// shown.
					fmt.Fprintf(&groups, "<a href=\"/OMNGoTags.html#%s\" class=\"taglink\"><span class=\"tagmark\">%s</span></a>",
						escapeHTML(tagSlug(t)), escapeHTML(t))
				}
				groups.WriteString("</div>\n")
			}

			lastSection := ""
			for _, m := range r.Matches {
				// The section heading is printed once per run of hits that
				// share it, not once per hit: several matches inside one
				// bookmark or one timestamped entry are one place, and
				// repeating the label for each would say otherwise.
				if m.Section != nil && m.Section.Label != "" && m.Section.Label != lastSection {
					lastSection = m.Section.Label
					groups.WriteString("  <div class=\"search-section\">")
					if m.Section.ID != "" {
						// r.URL already ends in the BEST hit's anchor; this
						// link wants THIS section's, so the document URL is
						// taken back apart rather than appended to.
						base := r.URL
						if at := strings.IndexByte(base, '#'); at >= 0 {
							base = base[:at]
						}
						fmt.Fprintf(&groups, "<a href=\"%s#%s\">%s</a>",
							escapeHTML(highlightURL(base, v.Highlight)),
							escapeHTML(m.Section.ID), escapeHTML(m.Section.Label))
					} else {
						groups.WriteString(escapeHTML(m.Section.Label))
					}
					groups.WriteString("</div>\n")
				} else if m.Section == nil {
					lastSection = ""
				}
				// Each line is its own link, and each one opens the note AT
				// that line. A snippet was plain text before, so the only way
				// into a note was the title above - which lands on the first
				// match, and a reader who chose the fifth line got the first.
				// snippetURL puts this line's text on the href as ?hlt=; the
				// text passes through percent-encoding there and HTML-escaping
				// here, because a note's content is attacker-controlled in the
				// LAN-sharing case.
				fmt.Fprintf(&groups, "  <a class=\"search-snippet\" href=\"%s\">",
					escapeHTML(snippetURL(r.URL, v.Highlight, m)))
				fmt.Fprintf(&groups, "<span class=\"search-snippet-line\">%d</span>", m.Line)
				if m.Context != "" {
					where := "inside a code block"
					if m.Context == "script" {
						where = "inside a <script> block"
					}
					fmt.Fprintf(&groups, "<span class=\"search-snippet-ctx\" title=\"%s\">&lsaquo;/&rsaquo;</span>",
						escapeHTML(where))
				}
				fmt.Fprintf(&groups, "<span class=\"search-snippet-text\">%s</span>",
					renderSnippetHTML(m.Text, m.Spans))
				groups.WriteString("</a>\n")
			}
			if r.Truncated {
				groups.WriteString("  <div class=\"search-result-note\">only the first 500 KiB of this file was searched</div>\n")
			}
			groups.WriteString("</div>\n")
		}
	}

	summary := ""
	empty := ""
	switch {
	case v.Query == "":
		summary = ""
	case v.Total == 0:
		// Naming what WAS searched matters: "no results" from a config the
		// reader has forgotten about is a trap, not an answer.
		var kinds []string
		for _, k := range v.IndexedKinds {
			kinds = append(kinds, searchKindLabel(k))
		}
		covered := "nothing"
		if len(kinds) > 0 {
			covered = strings.Join(kinds, ", ")
		}
		empty = fmt.Sprintf(`<div class="search-empty">`+
			`<p>No matches for <strong>%s</strong>.</p>`+
			`<p class="search-empty-hint">The index currently covers: %s. `+
			`<a href="/Config.html#cfg-search">Change what is searched</a>.</p>`+
			`</div>`, escapeHTML(v.Query), escapeHTML(covered))
	default:
		word := "results"
		if v.Total == 1 {
			word = "result"
		}
		summary = fmt.Sprintf("%d %s for <strong>%s</strong>", v.Total, word, escapeHTML(v.Query))
		if v.Truncated && len(v.Results) < v.Total {
			summary += fmt.Sprintf(" <span class=\"search-page-note\">(showing the first %d)</span>", len(v.Results))
		}
	}

	return fill(searchPageTmpl, map[string]string{
		"DISABLED": "",
		"NOTICE":   "",
		"QUERY":    escapeHTML(v.Query),
		"SUMMARY":  summary,
		"GROUPS":   groups.String(),
		"EMPTY":    empty,
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
// OMN_SEARCH_GLOBAL joins them for the same reason: whether the search dialog
// can offer the "All notes" scope depends on a setting that is toggleable at
// any time, and the header lives in every cached page. Baking it in at compile
// time would leave a stale answer on every page compiled before the toggle
// changed - exactly the problem this function exists to solve.
//
// All values are server-controlled (APP_VERSION is a build constant,
// UseInternalEd a bool, Theme whitelisted through normalizeTheme, and the
// search flag a bool), never user input, so splicing them with fmt is safe.
func (a *App) injectRuntimeVars(page []byte) []byte {
	cfg := a.GetConfig()
	script := fmt.Sprintf(
		`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t; var OMN_THEME = %q; var OMN_SEARCH_GLOBAL = %t; var OMN_INCOMING_PAGE = %q; document.documentElement.setAttribute('data-theme', OMN_THEME);</script>`,
		APP_VERSION, cfg.UseInternalEd, normalizeTheme(cfg.Theme), a.globalSearchAvailable(), incomingIndexName)
	page = bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
	// Splice the server-only modals into the slot (a no-op on templates that
	// don't carry it, e.g. the standalone editor page).
	page = bytes.Replace(page, []byte(modalsMarker), []byte(modalsHTML), 1)
	return page
}
