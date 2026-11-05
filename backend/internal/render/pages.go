package render

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"net.basov.omngo/backend/internal/config"
)

// PageHeader is the header block of a page that the server makes.
func PageHeader(title, category string, stylesheets ...string) []byte {
	h := "Title: " + title + "\nCategory: " + category + "\n"
	for _, s := range stylesheets {
		h += "Stylesheet: " + s + "\n"
	}
	return []byte(h + "\n")
}

// --- The page shell (index.html) ---

// index.html loads omn-go-custom.css and omn-go-custom.js last, thus a user
// rule wins at the same specificity. Keep this note out of the template,
// because the template goes to each page.
var IndexPageTmpl = LoadTemplate("index.html")

// modalsHTML holds the modals that need the server. InjectRuntimeVars puts
// them into ModalsMarker when the server sends the page, thus an exported
// page stays small.
var modalsHTML = LoadTemplate("modals.html")

// MetaTagView is one <meta name="..." content="..."> from the header block of
// a page, or the "generator" tag.
type MetaTagView struct {
	Name  string
	Value string
}

// IndexPageView holds each value of RenderIndexPage. PreviewHTML is trusted
// HTML. RenderIndexPage escapes each other field.
type IndexPageView struct {
	Title       string
	PackageName string
	PageName    string
	PageExt     string
	IsMarkdown  bool
	IsAndroid   bool
	AssetPrefix string // "", "../", "../../", … or "/" — see CompilePageWithBody
	MetaTags    []MetaTagView
	Stylesheets []string // the Stylesheet: lines of the header block
	Tags        []string
	PreviewHTML string
}

func RenderIndexPage(v IndexPageView) string {
	var metaTags strings.Builder
	for _, m := range v.MetaTags {
		fmt.Fprintf(&metaTags, "    <meta name=\"%s\" content=\"%s\" />\n",
			EscapeHTML(m.Name), EscapeHTML(m.Value))
	}

	// A page stylesheet goes before omn-go-custom.css, thus a rule of the
	// user still wins. The path is below html/, and the prefix makes it work
	// at each depth and through file://.
	var styles strings.Builder
	for _, s := range v.Stylesheets {
		fmt.Fprintf(&styles, "    <link rel=\"stylesheet\" href=\"%s%s\">\n",
			v.AssetPrefix, EscapeHTML(strings.TrimPrefix(s, "/")))
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
		// Each pill links to OMNGoTags through AssetPrefix, thus it works at
		// each depth and through file://. The fragment is TagSlug(t), the
		// same as the section ids of tags.go. AssetPrefix holds only '.' and
		// '/'.
		fmt.Fprintf(&tags, `<a href="%sOMNGoTags.html#%s" class="omn-taglink"><span class="omn-tagmark">%s</span></a>`,
			v.AssetPrefix, EscapeHTML(TagSlug(t)), EscapeHTML(t))
	}

	return Fill(IndexPageTmpl, map[string]string{
		"TITLE_HTML":   EscapeHTML(v.Title),
		"TITLE_JS":     EscapeJS(v.Title),
		"PACKAGE_JS":   EscapeJS(v.PackageName),
		"PAGE_NAME_JS": EscapeJS(v.PageName),
		"PAGE_EXT_JS":  EscapeJS(v.PageExt),
		// The path prefix ("", "../" or "/") holds only '.' and '/', thus it
		// needs no escape.
		"ASSET_PREFIX": v.AssetPrefix,
		"META_TAGS":    metaTags.String(),
		"PAGE_STYLES":  styles.String(),
		"COND_SCRIPTS": condScripts,
		"TAGS_HTML":    tags.String(),
		"PREVIEW_BODY": v.PreviewHTML,
	})
}

// --- The runtime values of a cached page ---

// RuntimeVarsMarker is the placeholder that index.html writes one time, near
// the end of <head>. It stays in the cached .html files on disk.
const RuntimeVarsMarker = `<meta id="omn-go-runtime-vars-marker">`

// ModalsMarker is the empty slot of the modals that need the server.
// InjectRuntimeVars puts modalsHTML there when the server sends the page. An
// exported page keeps an empty div.
const ModalsMarker = `<div id="omn-go-modals-slot"></div>`

// Facts gives InjectRuntimeVars two values of other groups. connectGroups
// sets them, thus the page shell names no search or exchange code. A nil
// SearchGlobal gives false.
type Facts struct {
	SearchGlobal func() bool // OMN_SEARCH_GLOBAL
	IncomingPage string      // OMN_INCOMING_PAGE
}

// userFileUploadsJS is the value of OMN_USER_FILE_UPLOADS: an object that maps
// each extension of config.UserFileTrees to the upload route of its tree. The
// receive box reads it, thus omn-go-api.js holds no copy of the table.
var userFileUploadsJS = func() string {
	js := []byte{'{'}
	for _, tree := range config.UserFileTrees {
		for _, ext := range tree.Exts {
			if len(js) > 1 {
				js = append(js, ',')
			}
			js = strconv.AppendQuote(js, ext)
			js = append(js, ':')
			js = strconv.AppendQuote(js, tree.Upload)
		}
	}
	return string(append(js, '}'))
}()

// InjectRuntimeVars puts the values of NOW into the RuntimeVarsMarker of a
// page: APP_VERSION, USE_INTERNAL_ED, OMN_THEME, OMN_SEARCH_GLOBAL,
// OMN_INCOMING_PAGE, OMN_USER_FILE_UPLOADS, OMN_LOG_DEBUG, OMN_LOG_INFO and
// OMN_LOG_TAGS. The cache
// on disk keeps the marker, thus a change of a setting needs no new compile.
//
// The marker is in <head>, thus data-theme applies before the body shows. The
// server controls each value, and config.NormalizeTheme and
// config.NormalizeLogTags allow only known values, thus the script can hold
// them as they are.
func (rd *Renderer) InjectRuntimeVars(page []byte) []byte {
	cfg := rd.Config
	searchGlobal := rd.Facts.SearchGlobal != nil && rd.Facts.SearchGlobal()
	// The version is a value here, and not a constant. fmt would thus copy
	// it to the heap at each request. The append functions write each value
	// with the same quotes as %q and %t of fmt.
	script := make([]byte, 0, 768)
	script = append(script, "<script>var APP_VERSION = "...)
	script = strconv.AppendQuote(script, rd.Version)
	script = append(script, "; var USE_INTERNAL_ED = "...)
	script = strconv.AppendBool(script, cfg.UseInternalEd)
	script = append(script, "; var OMN_THEME = "...)
	script = strconv.AppendQuote(script, config.NormalizeTheme(cfg.Theme))
	script = append(script, "; var OMN_SEARCH_GLOBAL = "...)
	script = strconv.AppendBool(script, searchGlobal)
	script = append(script, "; var OMN_INCOMING_PAGE = "...)
	script = strconv.AppendQuote(script, rd.Facts.IncomingPage)
	script = append(script, "; var OMN_USER_FILE_UPLOADS = "...)
	script = append(script, userFileUploadsJS...)
	script = append(script, "; var OMN_LOG_DEBUG = "...)
	script = strconv.AppendBool(script, cfg.LogDebug)
	script = append(script, "; var OMN_LOG_INFO = "...)
	script = strconv.AppendBool(script, cfg.LogInfo)
	script = append(script, "; var OMN_LOG_TAGS = "...)
	script = strconv.AppendQuote(script, strings.Join(config.NormalizeLogTags(cfg.LogTags), ","))
	script = append(script, "; document.documentElement.setAttribute('data-theme', OMN_THEME);</script>"...)
	page = bytes.Replace(page, []byte(RuntimeVarsMarker), script, 1)
	// Put the modals into the slot. The editor page has no slot, and nothing
	// changes there.
	page = bytes.Replace(page, []byte(ModalsMarker), []byte(modalsHTML), 1)
	return page
}

// WriteHTMLHeader is the ONE place that sets the type of a page, with the
// charset: a page that the server renders has no <meta charset>.
// pageCacheWriter in backend/internal/app/middleware.go reads the prefix
// "text/html". See TestConnectionMiddlewareUsesNoStoreForAPage.
func WriteHTMLHeader(w http.ResponseWriter) {
	w.Header().Set("Content-Type", HTMLContentType)
}

const HTMLContentType = "text/html; charset=utf-8"
