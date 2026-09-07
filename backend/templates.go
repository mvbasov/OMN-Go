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
// correct for security, and it carried a hidden cost across the whole
// binary. html/template, through text/template, calls
// reflect.Value.MethodByName. That call forces the Go linker to DISABLE
// dead-code elimination for methods across the entire program. The linker
// can no longer prove any method unreachable, thus it keeps all of them.
//
// The largest method surface in this binary by far is go-git, with every
// transport, storage backend and plumbing type. Most of it is normally
// pruned. With html/template linked, none of it was, and that is what blew
// the binary up.
//
// What html/template gave us was context-correct escaping of a few known
// fields into a few known positions. This file keeps exactly that
// guarantee, and it does so explicitly. Each render function escapes each
// value with the escape function that matches the context it is spliced
// into. Those contexts are HTML text, an HTML attribute, and a JS string
// literal. It uses a plain string Replacer, and no reflection anywhere.
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

// loadTemplate reads one page-fragment file out of templatesFS, which
// server.go declares. That embed is deliberately separate from staticFS.
// The frontend/html tree of staticFS is lazily extracted to disk as
// user-editable content, and these files must never be. A missing file is
// a packaging error. It is caught at the first render, and it does not
// crash the startup.
func loadTemplate(filename string) string {
	data, err := templatesFS.ReadFile("frontend/templates/" + filename)
	if err != nil {
		log.Printf("[templates] (error) failed to read embedded %s: %v", filename, err)
		return "<p>Missing embedded template: " + escapeHTML(filename) + "</p>"
	}
	return string(data)
}

// incomingIndexTmpl is the incoming index as first written. It holds a
// header block, the receive box that the desktop application imports
// through, and the marker that says where a new line goes. See
// incomingIndexStarter in note_exchange.go for why it lives here, and not
// in frontend/md/.
var incomingIndexTmpl = loadTemplate("incoming_index.md")

var (
	// index.html loads css/omn-go-custom.css as the last stylesheet and
	// js/omn-go-custom.js as the last script. The position is the feature.
	// A user rule beats an application rule of the same specificity, and
	// the user script sees everything that the application scripts define.
	//
	// Both files are user-owned. They are NOT in versionDependentAssets,
	// thus materializeAsset creates each one from the embedded copy on the
	// first request, and no upgrade replaces it.
	//
	// editor.html loads neither. A bad rule or a script error can then
	// never keep the user out of the editor that repairs it.
	//
	// Do not put these notes in the template itself. The template ships to
	// the browser with every page.
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
	// modalsHTML is the block of server-only modals. Those are login,
	// quick note, bookmark, commit and conflict. It is kept OUT of the
	// cached and exported page, and index.html carries only the
	// modalsMarker slot. injectRuntimeVars splices it in at serve time.
	// An offline or exported page thus stays small. Such a page has no
	// backend, and no use for these server features.
	modalsHTML = loadTemplate("modals.html")
)

// fill replaces %%NAME%% placeholders in tmpl. Every value passed in MUST
// already be escaped for the context that its placeholder sits in. See the
// rules at the top of this file. fill itself is escaping-agnostic on
// purpose, thus a trusted pre-rendered HTML fragment can pass through it
// as well.
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

// indexPageView holds everything that renderIndexPage needs. PreviewHTML
// is trusted pre-rendered HTML, either markdown output or a fragment built
// by the other render functions in this file. Every other field is a raw
// value that renderIndexPage escapes itself.
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
		// All pills point at the one generated Tags page, OMNGoTags. They
		// reach it relatively through AssetPrefix, which is "", "../" and
		// so on. The link thus resolves from any directory depth, online
		// and offline through file://.
		//
		// The fragment is tagSlug(t). The generated page uses that same
		// slug for its section ids, see tags.go. One Go function computes
		// both, thus they cannot drift.
		//
		// AssetPrefix carries only "./" characters, thus it needs no
		// escaping. That mirrors its ASSET_PREFIX use below.
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

// editorPageView holds everything that renderEditorPage needs. All fields
// are raw values, escaped here for the context that each one is spliced
// into. The text of the note is intentionally absent. The editor fetches
// it from /api/note at the start of the edit, thus a rendered page never
// carries a second copy of itself.
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

// gitServerView is one git-server slot as the Config page shows it.
//
// It carries NO SSH key and NO key password, and configPageView carries no
// admin password and no guest password. A secret in a view reaches the HTML
// of the page. The page is a file on disk, and each reader of the device
// can open it. Since 26.09.7 the boxes are empty, and the reader
// presses "Show passwords" to read the values from GET /api/config. See
// revealSecrets in omn-go-sse.js.
type gitServerView struct {
	Index  int
	Slot   int
	Active bool
	Name   string
	URL    string
}

type configPageView struct {
	ServerPort         int
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
	LogDebug           bool
	LogInfo            bool
	LogTags            []string // normalized
	GitServers         []gitServerView
}

// logTagLabels gives each tag the words the Config page shows beside its
// checkbox. A tag with no entry here falls back to its own text. A new tag
// thus reaches the page with no template edit and no missing box. The map is
// a label table and never the tag set: allLogTags in log_levels.go is the
// authority for that.
var logTagLabels = map[logTag]string{
	log404:         "Requests for a page that does not exist",
	logAssets:      "Bundled asset refresh at startup",
	logConfig:      "Reading and writing config.json",
	logDB:          "SQLite handles behind /api/sql",
	logDBBackup:    "Database backup and pruning",
	logDBBootstrap: "First-run restore on a new device",
	logDBRestore:   "Database restore from a backup",
	logEdit:        "The external editor",
	logExchange:    "Note import and export",
	logNoteFiles:   "Files carried between md/ and html/",
	logPage:        "Reading and writing a note",
	logPrecompile:  "Compiling notes to HTML",
	logRestart:     "Restarting the server process",
	logSearch:      "The global search index",
	logServer:      "Startup, the listener and crashes",
	logSession:     "The login and the session key",
	logStatus:      "The Status page",
	logStorage:     "The storage directory",
	logSync:        "Git sync, the loudest subsystem",
	logTags:        "The tags index",
	logTemplates:   "The embedded page templates",
	logUpload:      "File uploads",
}

// renderLogTagBoxes builds one checkbox for each tag in allLogTags. The list
// is built here rather than written into config_page.html, so that a new tag
// needs one line in log_levels.go and nothing else.
func renderLogTagBoxes(checked map[string]string) string {
	var b strings.Builder
	for _, tag := range allLogTags {
		label, ok := logTagLabels[tag]
		if !ok {
			label = string(tag)
		}
		b.WriteString(`                <div class="config-checkbox-row">` + "\n")
		b.WriteString(`                    <input type="checkbox" name="log_tags" value="` +
			escapeHTML(string(tag)) + `" ` + checked[string(tag)] + ` />` + "\n")
		b.WriteString(`                    <label class="config-label"><code>` +
			escapeHTML(string(tag)) + `</code> - ` + escapeHTML(label) + `</label>` + "\n")
		b.WriteString("                </div>\n")
	}
	return b.String()
}

func renderConfigPage(v configPageView) string {
	var cards strings.Builder
	for _, gs := range v.GitServers {
		checked := ""
		if gs.Active {
			checked = "checked"
		}
		// No SSH_KEY and no PASSWORD. See the banner of gitServerView.
		cards.WriteString(fill(gitServerCardTmpl, map[string]string{
			"INDEX":          fmt.Sprintf("%d", gs.Index),
			"SLOT":           fmt.Sprintf("%d", gs.Slot),
			"ACTIVE_CHECKED": checked,
			"NAME":           escapeHTML(gs.Name),
			"URL":            escapeHTML(gs.URL),
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
	logDebugChecked := ""
	if v.LogDebug {
		logDebugChecked = "checked"
	}
	logInfoChecked := ""
	if v.LogInfo {
		logInfoChecked = "checked"
	}
	// One checkbox per tag, checked when the tag is in the normalized list.
	logTagChecked := map[string]string{}
	for _, t := range v.LogTags {
		logTagChecked[t] = "checked"
	}

	searchScopeAllSel, searchScopePageSel := "checked", ""
	if normalizeSearchScope(v.SearchScope) == SearchScopePage {
		searchScopeAllSel, searchScopePageSel = "", "checked"
	}

	// Exactly one option is marked selected. normalizeTheme guarantees
	// that the value is one of the three, and it maps an unknown or empty
	// value to auto.
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

	// Exactly one option is marked selected. normalizeFullscreen
	// guarantees that the value is one of the three, and it maps an
	// unknown or empty value to FullscreenOn. See config.go for why that
	// is the default, and not "off".
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

	// No ADMIN_PWD and no GUEST_PWD. See the banner of gitServerView.
	return fill(configPageTmpl, map[string]string{
		// The names of the checkboxes of this page, from the table in
		// config_fields.go. See configCheckboxFields for why the page
		// must declare them.
		"CONFIG_FIELDS":          configCheckboxFields(),
		"SERVER_PORT":            fmt.Sprintf("%d", v.ServerPort),
		"AUTHOR":                 escapeHTML(v.Author),
		"INTERNAL_ED_CHECKED":    internalEdChecked,
		"SHARE_LAN_CHECKED":      shareLanChecked,
		"INTENT_URI_CHECKED":     intentUriChecked,
		"TERMUX_INTENT_CHECKED":  termuxIntentChecked,
		"DESKTOP_EXT_CMD":        escapeHTML(v.DesktopExtCmd),
		"HOSTNAME":               escapeHTML(normalizeHostname(v.Hostname)),
		"BACKUP_PRUNE_DEPTH":     fmt.Sprintf("%d", normalizePruneDepth(v.PruneDepth)),
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
		"LOG_DEBUG_CHECKED":      logDebugChecked,
		"LOG_INFO_CHECKED":       logInfoChecked,
		"LOG_TAG_BOXES":          renderLogTagBoxes(logTagChecked),
		"GIT_SERVERS":            cards.String(),
	})
}

// --- 404 page ---

// notFoundView is everything that the detailed 404 page shows. Every field
// is a RAW value, and renderNotFoundPage escapes it itself. URL and
// Referer above all are attacker-controlled. Anyone can request any path,
// and Referer is a plain request header. Neither must ever reach the
// output unescaped.
//
// This file deliberately does not use html/template. See the note at the
// top of this file for why. The escaping is thus the responsibility of
// this function, and not of a template engine.
type notFoundView struct {
	URL       string // path + query, exactly as requested
	Method    string
	Time      string
	Referer   string // "" when absent or not from this server
	Suggested string // "" when there is no plausible alternative
}

// safeLocalPath reports whether s may be used as an href. It must be a
// path on this server. A scheme-bearing value such as "javascript:..." is
// rejected, and so is a protocol-relative one such as "//host/...". No
// caller of renderNotFoundPage can thus turn a request header into an
// active link out of the app. This is defense in depth behind
// serveNotFound, which already filters the Referer that it passes in.
func safeLocalPath(s string) bool {
	return strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")
}

// notEditableView holds what the "not a text file" page shows. Path and
// Type are raw values that renderNotEditablePage escapes itself.
type notEditableView struct {
	Path string // "/css/OMN-Go/fonts/x.woff2"
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
	// An optional block is built here as trusted pre-rendered HTML. That
	// is the convention documented at the top of this file. Each value
	// inside is escaped as it is spliced in, and the surrounding markup is
	// ours.
	refererRows := ""
	if v.Referer != "" {
		esc := escapeHTML(v.Referer)
		if safeLocalPath(v.Referer) {
			refererRows = fmt.Sprintf(`        <dt>Linked from</dt>
        <dd><a href="%s">%s</a> &middot; <a href="%s?edit=true">edit that page</a></dd>
`, esc, esc, esc)
		} else {
			// Still worth reporting, and never as a link. escapeHTML makes
			// it inert as text. A "javascript:" or "//evil.example" value
			// in an href would stay live.
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
	shipCount   int
	anyDevice   bool
	everyShips  bool
	everyDevice bool
}

// filesFileRow is one NAME of the tree in view. Every field is a raw value,
// and renderFilesPage escapes it itself. A name comes from an upload or a
// note title, thus it is user-controlled, and this file assembles the HTML
// by hand.
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
// A page, and not the bare 401 that authMiddleware would produce. This
// address is linkable. A refusal that names neither the reason nor the
// remedy is a dead end. The 404 of the search page turned out to be one
// in 26.08.2.
//
// The markup is static, with no interpolation. Nothing here can carry a
// value in from a request, and no filename appears anywhere in this
// response.
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

// renderFilesCards is the first screen. It holds three buttons, in one
// column at every width. A wide screen gets a narrower page, and not three
// columns. There is one layout to build and one to test, and the three
// targets stay the size of a thumb.
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

// filesPageURL builds a link back into this page. Only three parameters
// exist. All of them are produced here, and not anywhere in a template,
// thus none can be spliced in from a request value.
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

	// Folded by default, and absent when this directory uses no word at
	// all. <details> is the own control of the browser. It needs no script,
	// it keeps its state while the page lives, and a reader who knows the
	// words never opens it.
	if len(v.Legend) > 0 {
		b.WriteString(`<details class="files-legend">` +
			`<summary>What the words mean</summary>`)
		for _, item := range v.Legend {
			fmt.Fprintf(&b, `<div><b class="%s">%s</b> — %s</div>`,
				escapeHTML(item.Color), escapeHTML(item.Word), escapeHTML(item.Text))
		}
		b.WriteString(`</details>`)
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
	// each of the three trees. Color is a hint. This word is the fact.
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

// filesDirNote gives the one word that a directory row can carry. It is
// the same rule as the rows use. Speak only when the application is
// involved.
//
// 26.08.54 had this the other way round, and it marked what was NOT
// shipped. On a real installation that is nearly every directory, such as
// the note tree, the compiled pages and the images. The page thus carried
// a column of words that said "ordinary". Now a directory speaks when
// OMN-Go delivered files into it, and the count says how many.
func filesDirNote(tree string, d filesDirRow) (word, color string) {
	if tree == filesTreeBundled || !d.anyShips {
		return "", ""
	}
	if d.everyShips && !d.anyDevice {
		return itoa(d.Files) + " " + filesFromTheApp + ", none extracted", filesColorPlain
	}
	return itoa(d.shipCount) + " " + filesFromTheApp, filesColorApp
}

// --- Search results page (search_page.html) ---

// searchPageView is everything that renderSearchPage needs. Query is RAW.
// It is whatever someone typed into a URL. It is thus escaped here for
// both the attribute context and the text context that it lands in. Results
// carry the same data that the API returns. renderSnippetHTML turns the
// snippet text and its spans into <mark> markup, and it escapes as it goes.
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
// Spans are RUNE offsets. The Go side works in runes, thus Cyrillic is
// never cut in half. The text is therefore walked as []rune, and not sliced
// by byte.
//
// Every segment is escaped as it is emitted. The only markup in the result
// is the <mark> tags that this function writes itself. That is the escaping
// contract at the top of this file, applied to text that comes from the own
// notes of the user.
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
// It used to be a 404. The reasoning was that a permanently empty results
// page is worse than an honest miss. That was wrong about who arrives here.
// The page is linkable, and people put a "Search" link on their Welcome
// note. The address is thus permanent navigation, and the 404 is a dead end
// that names neither the cause nor the cure.
//
// The markup is static, with no interpolation. Everything here is fixed
// text and one internal link. There is nothing to escape, and nothing that
// can carry a value in from a request.
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
		// Every other slot stays empty. There is no form, because a submit
		// would only land back here. There is no results section either,
		// which would look blank underneath the explanation.
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
			// The link carries the query as ?hl=. The note thus highlights
			// and scrolls to the match on arrival. Otherwise the reader
			// lands at the top of a long page and has to find it again by
			// eye. The client strips the parameters once it has applied
			// them. The URL left in the address bar is thus the plain one,
			// and that is the URL that gets copied or bookmarked.
			fmt.Fprintf(&groups, "  <a class=\"search-result-title\" href=\"%s\">%s</a>\n",
				escapeHTML(highlightURL(r.URL, v.Highlight)), escapeHTML(title))
			fmt.Fprintf(&groups, "  <div class=\"search-result-path\">%s</div>\n", escapeHTML(r.Name))

			if len(r.Tags) > 0 {
				groups.WriteString("  <div class=\"search-result-tags\">")
				for _, t := range r.Tags {
					// The same pill markup and the same anchor contract as
					// a page header, see renderIndexPage. A tag thus means
					// the same thing and goes to the same place, wherever
					// it is shown.
					fmt.Fprintf(&groups, "<a href=\"/OMNGoTags.html#%s\" class=\"taglink\"><span class=\"tagmark\">%s</span></a>",
						escapeHTML(tagSlug(t)), escapeHTML(t))
				}
				groups.WriteString("</div>\n")
			}

			lastSection := ""
			for _, m := range r.Matches {
				// The section heading is printed one time for each run of
				// hits that share it, and not one time for each hit.
				// Several matches inside one bookmark, or inside one
				// timestamped entry, are one place. A repeat of the label
				// for each would say otherwise.
				if m.Section != nil && m.Section.Label != "" && m.Section.Label != lastSection {
					lastSection = m.Section.Label
					groups.WriteString("  <div class=\"search-section\">")
					if m.Section.ID != "" {
						// r.URL already ends in the anchor of the BEST hit.
						// This link wants the anchor of THIS section, thus
						// the document URL is taken back apart, and not
						// appended to.
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
				// that line. A snippet was plain text before, thus the only
				// way into a note was the title above. That title lands on
				// the first match, and a reader who chose the fifth line
				// got the first.
				//
				// snippetURL puts the text of this line on the href as
				// ?hlt=. The text passes through percent-encoding there and
				// HTML-escaping here. The content of a note is
				// attacker-controlled in the LAN-sharing case.
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
		// ViewURL sits inside a JS string literal, and that literal sits
		// inside an HTML onclick attribute. JS-escape first, and then
		// HTML-escape the result. The inner context comes first, and the
		// outer context second.
		"VIEW_URL_ATTR_JS": escapeHTML(escapeJS(v.ViewURL)),
	})
}

// --- Runtime variable injection into cached pages ---

// runtimeVarsMarker is a literal placeholder that index.html always emits
// once, near the end of <head>. It survives into the compiled .html files
// cached on disk.
const runtimeVarsMarker = `<meta id="omn-go-runtime-vars-marker">`

// modalsMarker is the empty slot that index.html emits where the
// server-only modals go. injectRuntimeVars replaces it with modalsHTML when
// the backend serves the page. On an exported or offline page, which has no
// backend, it stays an empty div, thus those modals do not exist there.
const modalsMarker = `<div id="omn-go-modals-slot"></div>`

// injectRuntimeVars splices globals into the runtimeVarsMarker of a
// rendered page. Those globals must reflect the *currently running*
// server, and not whatever was true when a page was last compiled to the
// on-disk HTML cache.
//
// Pages are cached to disk, thus markdown is not re-rendered for each
// request. See precompileAllPages and the mtime check of serveHTMLPage.
// APP_VERSION is bumped between releases. UseInternalEd and Theme are both
// toggleable at any time from Config. All three must always reflect *now*.
// A recompile of every page whenever one of them changes would defeat the
// cache.
//
// The theme is applied by a data-theme attribute on <html> right here, and
// not by a class baked into the markup. The marker sits inside <head>, thus
// this script runs before the body is painted, and no flash of the wrong
// theme happens. It works the same way for a page compiled long before the
// theme changed.
//
// The CSS handles the rest. An explicit "light" or "dark" value pins the
// palette. "auto" falls through to the prefers-color-scheme media query,
// and so does a missing attribute. An exported page opened through file://
// has a missing attribute, because this marker is never replaced there.
//
// OMN_SEARCH_GLOBAL joins them for the same reason. Whether the search
// dialog can offer the "All notes" scope depends on a setting that is
// toggleable at any time. The header lives in every cached page. To
// bake it in at compile time would leave a stale answer on every page
// compiled before the toggle changed. That is exactly the problem this
// function exists to solve.
//
// OMN_LOG_DEBUG, OMN_LOG_INFO and OMN_LOG_TAGS join them for the same
// reason. omn-go-sse.js decides here what the browser console prints, the
// three values come from the Config page, and every page carries the
// EventSource that reads them. A page compiled before the switches changed
// would otherwise keep the old answer forever.
//
// The server controls every value, and none of them is user input, thus
// fmt can splice them safely. APP_VERSION is a build constant. UseInternalEd
// and the search flag are booleans. normalizeTheme whitelists Theme, and
// normalizeLogTags whitelists the log tags.
func (a *App) injectRuntimeVars(page []byte) []byte {
	cfg := a.GetConfig()
	script := fmt.Sprintf(
		`<script>var APP_VERSION = %q; var USE_INTERNAL_ED = %t; var OMN_THEME = %q; var OMN_SEARCH_GLOBAL = %t; var OMN_INCOMING_PAGE = %q; var OMN_LOG_DEBUG = %t; var OMN_LOG_INFO = %t; var OMN_LOG_TAGS = %q; document.documentElement.setAttribute('data-theme', OMN_THEME);</script>`,
		APP_VERSION, cfg.UseInternalEd, normalizeTheme(cfg.Theme), a.globalSearchAvailable(), incomingIndexName,
		cfg.LogDebug, cfg.LogInfo, strings.Join(normalizeLogTags(cfg.LogTags), ","))
	page = bytes.Replace(page, []byte(runtimeVarsMarker), []byte(script), 1)
	// Splice the server-only modals into the slot (a no-op on templates that
	// do not carry it, e.g. the standalone editor page).
	page = bytes.Replace(page, []byte(modalsMarker), []byte(modalsHTML), 1)
	return page
}
