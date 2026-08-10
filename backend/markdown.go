package backend

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

// hrefRe pulls out the raw href attribute value so we can decide, per link,
// how (or whether) to rewrite it.
var hrefRe = regexp.MustCompile(`href="([^"]*)"`)

// extRe matches a trailing filename extension (e.g. ".html", ".png", ".js")
// so we only append ".html" to links that don't already point at a
// concrete file.
var extRe = regexp.MustCompile(`\.[a-zA-Z0-9]+$`)

// uriSchemeRe matches a URI scheme at the start of a link, as RFC 3986
// defines one: ALPHA *( ALPHA / DIGIT / "+" / "-" / "." ) ":". A link that
// has one is not a page reference and must reach the browser exactly as the
// note author wrote it - see rewriteInternalLink.
//
// This is the same expression the click interceptor uses in
// omn-go-core.js (setupPreviewLinkInterceptor). Keep the two identical.
var uriSchemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

var mdParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		html.WithUnsafe(), // CRITICAL: Allows raw Bookmarks.md scripts to execute
	),
)

// Regexes used by renderMarkdownToHTML to shield content from the markdown /
// math passes. Compiled once.
var (
	// Raw/verbatim regions whose contents must never be treated as markdown or
	// KaTeX math: their text routinely contains '$', '*', '_', backticks and JS
	// `${...}` template literals.
	//
	// This is ONE combined, leftmost-first alternation rather than five
	// sequential passes, and that matters for correctness. Documentation
	// pages (e.g. Database.md) legitimately mention "<script>" inside inline
	// code and inside ``` fenced blocks. Run as separate passes, the
	// <script>...</script> regex matched the FIRST literal "<script>" (inside
	// a code span) and paired it with a real "</script>" far away in a later
	// fenced example - swallowing everything between and producing
	// placeholders whose stored text contained OTHER placeholders. Restoring
	// those nested placeholders in a single map-iteration pass then left some
	// unrestored (Go randomizes map order, so it surfaced on some
	// runs/devices and not others): exactly the leaked "OMN_RAW_n_END" tokens
	// this fixes. A single combined scan consumes each raw region whole, so a
	// "<script>" mentioned inside a code span or fence is part of that
	// span's/fence's match and can never start its own - no nesting, and
	// restore order is genuinely irrelevant.
	//
	// Alternation order is significant: the fenced ``` alternative must
	// precede the inline ` one, or a triple-backtick fence would first match
	// as an empty `` inline span.
	reRaw = regexp.MustCompile("(?is)<script\\b[^>]*>.*?</script>|<style\\b[^>]*>.*?</style>|<pre\\b[^>]*>.*?</pre>|```.*?```|`[^`]*`")

	// KaTeX math delimiters, protected from goldmark's emphasis handling.
	reMathBlock  = regexp.MustCompile(`(?s)\$\$.*?\$\$`)
	reMathInline = regexp.MustCompile(`\$[^\$]+\$`)
)

func (a *App) renderMarkdownToHTML(mdContent []byte) string {
	contentStr := string(mdContent)

	rawBlocks := make(map[string]string)
	mathBlocks := make(map[string]string)
	counter := 0
	// Placeholders are alphanumeric and "_END"-terminated so goldmark passes
	// them through verbatim and no placeholder is ever a substring of another
	// (OMN_MATH_1_END is not contained in OMN_MATH_10_END). The previous
	// scheme (OMN_MATH_INLINE_%d) collided on restore — "_1" matched inside
	// "_10" and, because a Go map iterates in random order, fragments of
	// unrelated math/code were spliced into each other.
	stash := func(store map[string]string, tag, m string) string {
		placeholder := fmt.Sprintf("OMN_%s_%d_END", tag, counter)
		store[placeholder] = m
		counter++
		return placeholder
	}

	// 1. Shield raw/verbatim regions BEFORE the math pass. Without this the
	//    inline-math regex below pairs up the '$' signs in JS `${...}` template
	//    literals (and any '$' inside code), which tears apart <script> notes
	//    like the SVG editor. Restored just before goldmark so <script>/<style>/
	//    <pre> pass through via html.WithUnsafe() and code renders as before.
	//    A single combined scan (reRaw) consumes each region whole, so raw
	//    regions never nest inside one another's placeholders.
	contentStr = reRaw.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(rawBlocks, "RAW", m)
	})

	// 2. Protect genuine KaTeX math (now only in prose) from emphasis corruption.
	contentStr = reMathBlock.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(mathBlocks, "MATH", m)
	})
	contentStr = reMathInline.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(mathBlocks, "MATH", m)
	})

	// 3. Restore the raw regions before rendering so goldmark parses them as
	//    it always has. The combined scan above guarantees no placeholder's
	//    stored text contains another, so order is irrelevant; the fixed-point
	//    helper is cheap insurance against any future change reintroducing
	//    nesting (a silent, order-dependent leak otherwise).
	contentStr = restorePlaceholders(contentStr, rawBlocks)

	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(contentStr), &buf); err != nil {
		return string(mdContent)
	}
	htmlStr := buf.String()

	// Restore math blocks natively for the offline KaTeX frontend.
	htmlStr = restorePlaceholders(htmlStr, mathBlocks)

	// Remap static browsing links natively
	htmlStr = hrefRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		match := hrefRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return `href="` + a.rewriteInternalLink(match[1]) + `"`
	})
	return htmlStr
}

// restorePlaceholders substitutes every placeholder in store back into s,
// repeating until the string stops changing so a placeholder whose stored
// text itself contains another placeholder is still fully restored
// regardless of Go's randomized map-iteration order. With the current
// single-pass stashing no nesting occurs, so this converges in one pass; the
// loop (bounded by the number of placeholders, since restoration forms a DAG
// and can never cycle) makes a stray, order-dependent leak - like the
// historical "OMN_RAW_n_END" one - structurally impossible.
func restorePlaceholders(s string, store map[string]string) string {
	for i := 0; i <= len(store); i++ {
		before := s
		for placeholder, original := range store {
			s = strings.ReplaceAll(s, placeholder, original)
		}
		if s == before {
			break
		}
	}
	return s
}

// rewriteInternalLink normalizes a raw markdown-authored href the way a
// browser would resolve it, so that:
//   - "./page", "../page", and bare "page" stay relative to the current page
//   - "/page" stays an absolute path for the site root
//   - "#anchor" and "?query" suffixes (and page#anchor / page?query
//     combinations) are left untouched rather than having ".html" appended
//     after them
//
// The only thing this function actually changes is normalizing an internal
// page reference's extension: ".md" becomes ".html", and a bare page name
// with no extension gets ".html" appended. Anything that already has a
// concrete extension (.html, .js, .css, .png, ...), any link that carries a
// URI scheme, and any link that is purely an anchor or query string is passed
// through unchanged.
//
// A LINK WITH A SCHEME IS NOT A PAGE. This test was an allowlist - http,
// https, mailto, tel, javascript, data, intent - and every scheme absent from
// it was read as a bare page name and given ".html":
//
//	sms:+15551234               ->  sms:+15551234.html
//	sms:+1555?body=Hi           ->  sms:+1555.html?body=Hi
//	whatsapp://send?phone=1555  ->  whatsapp://send.html?phone=1555
//	geo:59,30                   ->  geo:59,30.html
//
// Each of those reaches Android as a URI that names nothing, and the
// Messaging or Maps app it was written for never opens. A list can only ever
// be short of some scheme; "geo:59.9,30.3" even survived by accident, because
// its last "." looked like a file extension.
//
// The test is the scheme itself now (uriSchemeRe), which is what the click
// interceptor in omn-go-core.js already used. The two have to agree: this
// function decides what the page SAYS and the interceptor decides what a tap
// DOES, and a link works only when both leave it alone.
// MainActivity.shouldOverrideUrlLoading hands every scheme it does not serve
// itself to the OS, so the app that owns it (Messaging, Dialer, Maps, Termux)
// opens - provided what arrives is what the note author wrote.
//
// The cost is a page name that holds a ":" before any "/". "Notes:Draft" is
// not distinguishable from a scheme and is now left alone instead of becoming
// "Notes:Draft.html". The interceptor reads such a name the same way, so it
// does not work on the client side either.
//
// The raw-HTML button form (onclick="window.location='sms:...'") is untouched
// regardless, since the href-rewrite regex only rewrites href="..." values.
func (a *App) rewriteInternalLink(href string) string {
	if href == "" {
		return href
	}

	switch {
	// "//host/path" is protocol-relative: no scheme of its own, external all
	// the same. "#anchor" is this page.
	case strings.HasPrefix(href, "//"),
		strings.HasPrefix(href, "#"),
		uriSchemeRe.MatchString(href):
		return href
	}

	// Split off the query/fragment suffix so it's never touched by the
	// extension rewrite below (e.g. "Page?x=1" must not become
	// "Page?x=1.html", and "Page#section" must not become
	// "Page#section.html").
	path := href
	suffix := ""
	if idx := strings.IndexAny(href, "?#"); idx >= 0 {
		path = href[:idx]
		suffix = href[idx:]
	}

	// A bare "?query" or the (already-handled) "#anchor" case with nothing
	// before it — nothing to rewrite, it's relative to the current page.
	if path == "" {
		return href
	}

	// Only touch the final path segment; preserve any "./", "../", nested
	// directories, or a leading "/" exactly as written so relative and
	// absolute semantics are unaffected.
	dir := ""
	base := path
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		dir = path[:slash+1]
		base = path[slash+1:]
	}

	// Directory-only reference (".", "..", "", trailing slash) - leave as-is.
	if base == "" || base == "." || base == ".." {
		return href
	}

	switch {
	case strings.HasSuffix(base, ".md"):
		base = strings.TrimSuffix(base, ".md") + ".html"
	case extRe.MatchString(base):
		// Already has a concrete extension (.html, .js, .css, .png, ...) -
		// leave it alone.
	default:
		base += ".html"
	}

	return dir + base + suffix
}

// htmlEscape is kept as a method for its existing call sites; the single
// escaping implementation lives in templates.go (escapeHTML).
func (a *App) htmlEscape(s string) string {
	return escapeHTML(s)
}

func (a *App) compilePage(name string, mdContent []byte) []byte {
	return a.compilePageWithBody(name, mdContent, "")
}

// compilePageWithBody renders the full page shell (indexPageTmpl) for a
// single note/page/asset-edit view.
//
// customBody, when non-empty, is used as the (already-HTML) main content
// instead of rendering mdContent as markdown - this is how the
// Config-dashboard and "editing externally" wait pages reuse the same page
// shell without being markdown themselves.
//
// Editing is no longer an in-page mode: ?edit=true is served by the
// dedicated editor page (renderEditorPage), so this function only ever
// produces read/view shells.
func (a *App) compilePageWithBody(name string, mdContent []byte, customBody string) []byte {
	// One header-block split for the whole backend (see header_block.go).
	// Previously this function had its own line-by-line header scan that
	// classified any colon-bearing line as header - swallowing e.g. a
	// "# Head: x" Markdown heading. parseHeaderBlock uses the same
	// first-line rule as ensureHeaderModified and handleNewPage.
	hb := parseHeaderBlock(string(mdContent))
	var headers []string
	if hb.HasHeader {
		headers = strings.Split(hb.Header, "\n")
	}

	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = a.renderMarkdownToHTML([]byte(hb.Body))
	}

	// Title and Tags come from the shared extractTitleTags (also used by the
	// Tags-page generator, so the two parse notes identically). The loop below
	// only builds metaTags now.
	title := "OMN-Go - " + name
	rawTitle, tags := extractTitleTags(string(mdContent))
	if rawTitle != "" {
		title = rawTitle
	}
	var metaTags []metaTagView
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		// No escaping here - renderIndexPage escapes every meta name/value
		// for the HTML-attribute context itself.
		metaTags = append(metaTags, metaTagView{Name: k, Value: v})
	}
	metaTags = append(metaTags, metaTagView{Name: "generator", Value: "OMN-Go " + APP_VERSION})

	// Determine file extension (used by the view page's edit link).
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if strings.Contains(name, ".") {
		// non-markdown file — keep its extension (e.g. .js, .css, .json)
		pageExt = filepath.Ext(name)
	}
	isMarkdown := pageExt == ".md" || pageExt == ""

	// Chrome-asset (CSS/JS/Home) path prefix. A normal markdown note
	// (customBody == "") is cached to html/<name>.html and may be opened
	// directly from disk (file://), where an absolute "/js/..." path doesn't
	// resolve - so use a prefix relative to the page's own directory depth
	// (see relPrefix), which resolves correctly both offline and online.
	// Custom-body pages (Config, DB backups, the external-edit wait page) are
	// dynamic, served at URLs whose depth doesn't track the page name and
	// never opened from disk, so they keep absolute "/" paths.
	assetPrefix := "/"
	if customBody == "" {
		assetPrefix = relPrefix(name)
	}

	view := indexPageView{
		Title:       title,
		PackageName: "net.basov.omngo",
		PageName:    name,
		PageExt:     pageExt,
		IsMarkdown:  isMarkdown,
		IsAndroid:   runtime.GOOS == "android",
		AssetPrefix: assetPrefix,
		MetaTags:    metaTags,
		Tags:        tags,
		PreviewHTML: renderedBody,
	}

	return []byte(renderIndexPage(view))
}

// relPrefix returns the "../"-per-directory-level prefix that makes a cached
// page's chrome-asset URLs (CSS/JS/Home) resolve to the storage root both
// when served over HTTP and when the compiled .html is opened directly from
// disk (file://). A root-level page yields "", a page one directory deep
// "../", two deep "../../", and so on.
func relPrefix(name string) string {
	return strings.Repeat("../", strings.Count(name, "/"))
}

func (a *App) ensureHeaderModified(content string, defaultTitle string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	now := time.Now().Format("2006-01-02 15:04:05")

	// Same header decision as everywhere else (see header_block.go).
	hb := parseHeaderBlock(content)

	if hb.HasHeader {
		headerLines := strings.Split(hb.Header, "\n")
		modIdx := -1
		for i, l := range headerLines {
			if strings.HasPrefix(strings.ToLower(l), "modified:") {
				modIdx = i
				break
			}
		}
		if modIdx != -1 {
			headerLines[modIdx] = fmt.Sprintf("Modified: %s", now)
		} else {
			headerLines = append(headerLines, fmt.Sprintf("Modified: %s", now))
		}
		// Body is "" for a header-only note; the trailing "\n\n" preserves
		// the previous behavior (a header always ends with a blank line).
		return strings.Join(headerLines, "\n") + "\n\n" + hb.Body
	}

	authorLine := ""
	if author := a.GetConfig().Author; author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}
