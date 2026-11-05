package render

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
	"net.basov.omngo/backend/internal/config"
	"net.basov.omngo/backend/internal/noteheader"
)

// hrefRe finds the raw value of an href attribute, thus the code can decide
// for each link.
var hrefRe = regexp.MustCompile(`href="([^"]*)"`)

// URISchemeRe matches a URI scheme at the start of a link, as RFC 3986
// defines it. A link with a scheme is not a page, and it must reach the
// browser as the author wrote it. setupPreviewLinkInterceptor in
// omn-go-nav.js uses the same expression. Keep the two equal.
var URISchemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)

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

// These are the regular expressions that protect text from the markdown and
// math passes.
var (
	// reRaw finds the raw regions that are never markdown or math: <script>,
	// <style>, <pre>, a fenced block and a code span. Their text often holds
	// '$', '*', '_' and backticks.
	//
	// It is ONE alternation, and the leftmost match wins. Database.md
	// mentions "<script>" inside inline code and inside a fence. Separate
	// passes would pair that text with a real "</script>" much later, and the
	// placeholders would nest. TestRenderMarkdownRawNoPlaceholderLeak holds
	// the rule.
	//
	// The fence must come before the inline code span, or "```" matches as an
	// empty `` span.
	reRaw = regexp.MustCompile("(?is)<script\\b[^>]*>.*?</script>|<style\\b[^>]*>.*?</style>|<pre\\b[^>]*>.*?</pre>|```.*?```|`[^`]*`")

	// These are the KaTeX delimiters. The code protects them from the
	// emphasis rules of goldmark.
	reMathBlock  = regexp.MustCompile(`(?s)\$\$.*?\$\$`)
	reMathInline = regexp.MustCompile(`\$[^\$]+\$`)
)

// RenderMarkdown makes the HTML body of a note from its markdown.
func (rd *Renderer) RenderMarkdown(mdContent []byte) string {
	contentStr := string(mdContent)

	rawBlocks := make(map[string]string)
	mathBlocks := make(map[string]string)
	counter := 0
	// A placeholder has only letters, digits and '_', and it ends with
	// "_END". goldmark thus keeps it as it is, and no placeholder is part of
	// another: OMN_MATH_10_END does not hold OMN_MATH_1_END.
	stash := func(store map[string]string, tag, m string) string {
		placeholder := fmt.Sprintf("OMN_%s_%d_END", tag, counter)
		store[placeholder] = m
		counter++
		return placeholder
	}

	// 1. Protect each raw region BEFORE the math pass. Without this, the
	// inline math pattern pairs the '$' signs of a JS template literal, and
	// it breaks the script of a note. reRaw takes each region whole, thus the
	// regions never nest.
	contentStr = reRaw.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(rawBlocks, "RAW", m)
	})

	// 2. Protect the KaTeX math, now only in prose, from the emphasis rules.
	contentStr = reMathBlock.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(mathBlocks, "MATH", m)
	})
	contentStr = reMathInline.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(mathBlocks, "MATH", m)
	})

	// 3. Restore the raw regions before goldmark, thus goldmark reads them as
	// before. No stored text holds a placeholder, thus the order does not
	// matter. restorePlaceholders still repeats until nothing changes.
	contentStr = restorePlaceholders(contentStr, rawBlocks)

	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(contentStr), &buf); err != nil {
		return string(mdContent)
	}
	htmlStr := buf.String()

	// Restore the math for KaTeX in the page.
	htmlStr = restorePlaceholders(htmlStr, mathBlocks)

	// Rewrite each internal link. See RewriteInternalLink.
	htmlStr = hrefRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		match := hrefRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return `href="` + rd.RewriteInternalLink(match[1]) + `"`
	})
	return htmlStr
}

// restorePlaceholders puts each stored text of store back into s. It repeats
// until s stops changing, thus a nested placeholder also comes back, in any
// map order. A restore cannot loop, thus len(store) passes are sufficient.
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

// RewriteInternalLink changes one thing in an href: the extension of a link
// to a page. ".md" becomes ".html", and a page name with no extension gets
// ".html". "./page", "../page", "page" and "/page" keep their meaning. A
// "#anchor" or "?query" suffix stays after the new extension.
//
// Three kinds of link stay as they are. They are a link with a file
// extension, a link with a URI scheme, and an anchor or query alone.
//
// A LINK WITH A SCHEME IS NOT A PAGE. A list of known schemes is always short
// of one, and ".html" then breaks it:
//
//	sms:+15551234               ->  sms:+15551234.html
//	whatsapp://send?phone=1555  ->  whatsapp://send.html?phone=1555
//
// The test is thus the scheme itself, URISchemeRe, the same as the click
// interceptor of omn-go-nav.js. This function decides what the page SAYS,
// and the interceptor decides what a tap DOES.
// WebViewSetup.shouldOverrideUrlLoading gives each unknown scheme to the OS.
// TestRenderMarkdownToHTMLSchemeLinksUntouched holds the rule.
//
// This rule has a cost. A page name with ":" before each "/", for example
// "Notes:Draft", looks like a scheme and gets no ".html".
func (rd *Renderer) RewriteInternalLink(href string) string {
	if href == "" {
		return href
	}

	switch {
	// "//host/path" has no scheme, but it is external. "#anchor" is this
	// page.
	case strings.HasPrefix(href, "//"),
		strings.HasPrefix(href, "#"),
		URISchemeRe.MatchString(href):
		return href
	}

	// Split off the query or the fragment, thus the new extension goes before
	// it: "Page?x=1" becomes "Page.html?x=1".
	path := href
	suffix := ""
	if idx := strings.IndexAny(href, "?#"); idx >= 0 {
		path = href[:idx]
		suffix = href[idx:]
	}

	// A "?query" alone refers to the current page. Change nothing.
	if path == "" {
		return href
	}

	// Change only the last path segment. Keep "./", "../", the directories
	// and a leading "/" as written.
	dir := ""
	base := path
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		dir = path[:slash+1]
		base = path[slash+1:]
	}

	// This is a reference to a directory. Change nothing.
	if base == "" || base == "." || base == ".." {
		return href
	}

	// config.HasKnownAssetExtension in internal/config/content_types.go is the one
	// authority here. It reads the LAST extension, thus "Report.2026" becomes
	// "Report.2026.html" and "draft.txt" stays.
	switch {
	case strings.HasSuffix(base, ".md"):
		base = strings.TrimSuffix(base, ".md") + ".html"
	case config.HasKnownAssetExtension(rd.Config.MimeTypes, base):
		// This is a file that this install serves, for example .js, .css or
		// .png.
	default:
		base += ".html"
	}

	return dir + base + suffix
}

// CompilePage compiles a note. See CompilePageWithBody.
func (rd *Renderer) CompilePage(name string, mdContent []byte) []byte {
	return rd.CompilePageWithBody(name, mdContent, "")
}

// CompilePageWithBody renders the page shell (IndexPageTmpl) for one view.
// When customBody is not empty, it is the HTML of the page, and mdContent is
// not rendered. The Config page and the wait page of the external editor use
// that. ?edit=true goes to RenderEditorPage, and not here.
func (rd *Renderer) CompilePageWithBody(name string, mdContent []byte, customBody string) []byte {
	// noteheader.Parse is the one header split. See package noteheader.
	hb := noteheader.Parse(string(mdContent))
	var headers []string
	if hb.HasHeader {
		headers = strings.Split(hb.Header, "\n")
	}

	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = rd.RenderMarkdown([]byte(hb.Body))
	}

	// ExtractTitleTags reads the title and the tags, the same as the Tags
	// page. The loop below only makes metaTags.
	title := "OMN-Go - " + name
	rawTitle, tags := ExtractTitleTags(string(mdContent))
	if rawTitle != "" {
		title = rawTitle
	}
	var metaTags []MetaTagView
	var stylesheets []string
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		if k == "stylesheet" {
			stylesheets = append(stylesheets, v)
			continue
		}
		// RenderIndexPage escapes each meta name and value for the attribute.
		metaTags = append(metaTags, MetaTagView{Name: k, Value: v})
	}
	metaTags = append(metaTags, MetaTagView{Name: "generator", Value: rd.Generator})

	// Find the file extension for the edit link of the view page. An empty
	// customBody means a note, because RenderAndCache is the only caller that
	// passes none. A NAME ALONE CANNOT ANSWER THIS. The note "Draft.txt" and
	// the file html/Draft.txt have the same name here. The note "Report.2026"
	// must still get IsMarkdown.
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if customBody != "" && config.HasKnownAssetExtension(rd.Config.MimeTypes, name) {
		// This is a view that the server makes for a file, for example the
		// wait page of the external editor. Keep the extension of the file.
		pageExt = filepath.Ext(name)
	}
	isMarkdown := pageExt == ".md" || pageExt == ""

	// Find the path prefix of the page assets: CSS, JS and Home. A note goes
	// to the cache html/<name>.html, and a person can open it from disk
	// through file://. There, "/js/..." does not resolve. RelPrefix gives a
	// relative prefix that works online and offline. A page with customBody
	// is always dynamic, thus it keeps "/".
	assetPrefix := "/"
	if customBody == "" {
		assetPrefix = RelPrefix(name)
	}

	view := IndexPageView{
		Title:       title,
		PackageName: "net.basov.omngo",
		PageName:    name,
		PageExt:     pageExt,
		IsMarkdown:  isMarkdown,
		IsAndroid:   runtime.GOOS == "android",
		AssetPrefix: assetPrefix,
		MetaTags:    metaTags,
		Stylesheets: stylesheets,
		Tags:        tags,
		PreviewHTML: renderedBody,
	}

	return []byte(RenderIndexPage(view))
}

// RelPrefix answers one "../" for each directory level of name. With it, the
// asset URLs of a cached page reach the storage root over HTTP and through
// file://. A page at the root gets "".
func RelPrefix(name string) string {
	return strings.Repeat("../", strings.Count(name, "/"))
}

// EnsureHeaderModified sets the Modified: line of a note to now. A note
// with no header block gets a new header with the title, the dates and the
// author.
func EnsureHeaderModified(content, defaultTitle, author string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	now := noteheader.Stamp(time.Now())

	// Use the one header split. See package noteheader.
	hb := noteheader.Parse(content)

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
		// This rebuild uses a fixed "\n\n". A header that ended at a body
		// line thus gets an empty line after it. The noteheader package says
		// why noteheader.SetKey does not do this.
		return strings.Join(headerLines, "\n") + "\n\n" + hb.Body
	}

	authorLine := ""
	if author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}
