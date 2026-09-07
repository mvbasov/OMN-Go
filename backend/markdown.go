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
	// This is ONE combined, leftmost-first alternation, and not five
	// sequential passes. That matters for correctness. A documentation page
	// such as Database.md legitimately mentions "<script>" inside inline
	// code, and inside a ``` fenced block.
	//
	// Run as separate passes, the <script>...</script> regular expression
	// matched the FIRST literal "<script>", inside a code span. It paired
	// that with a real "</script>" far away in a later fenced example. It
	// swallowed everything between, and it produced placeholders whose
	// stored text held OTHER placeholders.
	//
	// A restore of those nested placeholders in one map-iteration pass then
	// left some of them unrestored. Go randomizes map order, thus the fault
	// surfaced on some runs and devices and not on others. Those are exactly
	// the leaked "OMN_RAW_n_END" tokens that this fixes.
	//
	// One combined scan consumes each raw region whole. A "<script>"
	// mentioned inside a code span or a fence is thus part of the match of
	// that span or fence. It can never start its own. There is no nesting,
	// and the restore order is irrelevant.
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
	// A placeholder is alphanumeric and ends with "_END". goldmark thus
	// passes it through verbatim, and no placeholder is ever a substring of
	// another. OMN_MATH_10_END does not contain OMN_MATH_1_END.
	//
	// The previous scheme was OMN_MATH_INLINE_%d, and it collided on
	// restore. "_1" matched inside "_10". A Go map iterates in random order,
	// thus fragments of unrelated math and code were spliced into each
	// other.
	stash := func(store map[string]string, tag, m string) string {
		placeholder := fmt.Sprintf("OMN_%s_%d_END", tag, counter)
		store[placeholder] = m
		counter++
		return placeholder
	}

	// 1. Shield each raw and verbatim region BEFORE the math pass. Without
	//    this, the inline-math regular expression below pairs up the '$'
	//    signs in a JS `${...}` template literal, and any '$' inside code.
	//    That tears apart a <script> note such as the SVG editor. The
	//    regions are restored right before goldmark, thus <script>, <style>
	//    and <pre> pass through by html.WithUnsafe(), and code renders as
	//    before. One combined scan, reRaw, consumes each region whole, thus
	//    raw regions never nest inside the placeholders of one another.
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

	// 3. Restore the raw regions before the render, thus goldmark parses
	//    them as it always has. The combined scan above guarantees that the
	//    stored text of a placeholder contains no other placeholder, thus
	//    order is irrelevant. The fixed-point helper is cheap insurance
	//    against a future change that brings nesting back, which would
	//    otherwise be a silent, order-dependent leak.
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

// restorePlaceholders substitutes every placeholder of store back into s. It
// repeats until the string stops changing. A placeholder whose stored text
// itself holds another placeholder is thus fully restored, whatever the
// randomized map-iteration order of Go is.
//
// With the current single-pass stashing there is no nesting, thus this
// converges in one pass. The loop is bounded by the number of placeholders,
// because a restore forms a DAG and can never cycle. It makes a stray,
// order-dependent leak structurally impossible, and the historical
// "OMN_RAW_n_END" leak was one of those.
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
// This function changes one thing only. It normalizes the extension of an
// internal page reference. ".md" becomes ".html", and a bare page name with
// no extension gets ".html" appended.
//
// Three kinds of link pass through unchanged. The first already has a
// concrete extension, such as .html, .js, .css or .png. The second carries a
// URI scheme. The third is purely an anchor or a query string.
//
// A LINK WITH A SCHEME IS NOT A PAGE. This test was an allowlist of http,
// https, mailto, tel, javascript, data and intent. Every scheme absent from
// it was read as a bare page name and given ".html":
//
//	sms:+15551234               ->  sms:+15551234.html
//	sms:+1555?body=Hi           ->  sms:+1555.html?body=Hi
//	whatsapp://send?phone=1555  ->  whatsapp://send.html?phone=1555
//	geo:59,30                   ->  geo:59,30.html
//
// Each of those reaches Android as a URI that names nothing. The Messaging
// or Maps app that it was written for never opens. A list can only ever be
// short of some scheme. "geo:59.9,30.3" even survived by accident, because
// its last "." looked like a file extension.
//
// The test is now the scheme itself, uriSchemeRe, which is what the click
// interceptor in omn-go-core.js already used. The two have to agree. This
// function decides what the page SAYS, and the interceptor decides what a
// tap DOES. A link works only when both leave it alone.
//
// MainActivity.shouldOverrideUrlLoading hands every scheme that it does not
// serve itself to the OS. The app that owns it then opens, which is
// Messaging, Dialer, Maps or Termux. What arrives has to be what the note
// author wrote.
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

	// Split off the query/fragment suffix so it is never touched by the
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
	// before it — nothing to rewrite, it is relative to the current page.
	if path == "" {
		return href
	}

	// Only touch the final path segment. Preserve a "./", a "../", a nested
	// directory and a leading "/" exactly as written, thus the relative and
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

	// hasKnownAssetExtension (serving.go) is the one authority for the
	// question below. It reads the LAST extension. A link to a note named
	// "Report.2026" thus becomes "Report.2026.html", and a link to the file
	// "draft.txt" stays as it is. Until 26.08.76 a regular expression
	// matched any extension-shaped tail here, thus a link to a note with a
	// dot in its name went nowhere.
	switch {
	case strings.HasSuffix(base, ".md"):
		base = strings.TrimSuffix(base, ".md") + ".html"
	case a.hasKnownAssetExtension(base):
		// A file that this install serves, for example .html, .js, .css or
		// .png. Leave it alone.
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
// customBody, when non-empty, is used as the main content, and it is already
// HTML. mdContent is then not rendered as markdown. That is how the Config
// dashboard and the "editing externally" wait page reuse the same page
// shell, although neither is markdown itself.
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

	// The file extension, which the view page gives to its edit link.
	//
	// An empty customBody means a note. renderAndCache is the only caller
	// that reaches this function that way, and it only ever compiles a
	// note. The asset prefix below reads customBody for the same reason.
	//
	// A NAME ALONE CANNOT ANSWER THIS. A note named "Draft.txt" and the
	// file html/Draft.txt carry the same name here. Before 26.08.76 this
	// block asked whether the name held a dot, thus a note named
	// "Report.2026" got pageExt ".2026" and IsMarkdown false. The page then
	// lost each control that belongs to a note.
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if customBody != "" && a.hasKnownAssetExtension(name) {
		// A server-built view of a file, for example the wait page of the
		// external editor. Keep the extension of that file.
		pageExt = filepath.Ext(name)
	}
	isMarkdown := pageExt == ".md" || pageExt == ""

	// The path prefix of a chrome asset, which is CSS, JS or Home. A normal
	// markdown note has customBody == "". It is cached to html/<name>.html,
	// and it may be opened directly from disk through file://. An absolute
	// "/js/..." path does not resolve there. Use a prefix relative to the own
	// directory depth of the page, see relPrefix, which resolves correctly
	// both offline and online.
	//
	// A custom-body page is Config, DB backups or the external-edit wait
	// page. Each is dynamic and is served at a URL whose depth does not
	// track the page name. None is ever opened from disk, thus they keep
	// absolute "/" paths.
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

// relPrefix answers the prefix of one "../" for each directory level. That
// prefix makes the chrome-asset URLs of a cached page resolve to the storage
// root. Those URLs are CSS, JS and Home. It works when the page is served
// over HTTP, and when the compiled .html is opened directly from disk
// through file://.
//
// A root-level page yields "". A page one directory deep yields "../", two
// deep yields "../../", and so on.
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
