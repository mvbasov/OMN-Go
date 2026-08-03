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

// hrefRe pulls out the raw href value for per-link rewriting.
var hrefRe = regexp.MustCompile(`href="([^"]*)"`)

// extRe matches a trailing filename extension, so rewriteInternalLink appends
// ".html" only to a link that has none.
var extRe = regexp.MustCompile(`\.[a-zA-Z0-9]+$`)

var mdParser = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithParserOptions(
		// search_sections.go predicts these ids. The two rules must agree.
		parser.WithAutoHeadingID(),
	),
	goldmark.WithRendererOptions(
		html.WithHardWraps(),
		html.WithUnsafe(), // Required so note scripts run
	),
)

// Regexes that shield content from the markdown and math passes.
var (
	// Raw regions whose text must never be parsed as markdown or math.
	// One combined alternation, not one pass per form: a "<script>" written
	// inside a code span must not start a region of its own, or placeholders
	// nest. The fenced form must precede the inline one in the alternation.
	reRaw = regexp.MustCompile("(?is)<script\\b[^>]*>.*?</script>|<style\\b[^>]*>.*?</style>|<pre\\b[^>]*>.*?</pre>|```.*?```|`[^`]*`")

	// Math spans are stashed so goldmark does not eat '_' or '*' inside them.
	reMathBlock  = regexp.MustCompile(`(?s)\$\$.*?\$\$`)
	reMathInline = regexp.MustCompile(`\$[^\$]+\$`)
)

func (a *App) renderMarkdownToHTML(mdContent []byte) string {
	contentStr := string(mdContent)

	rawBlocks := make(map[string]string)
	mathBlocks := make(map[string]string)
	counter := 0
	// Placeholders pass through goldmark verbatim. The "_END" terminator keeps
	// one placeholder out of another (OMN_MATH_1_END inside OMN_MATH_10_END),
	// which would splice unrelated text on restore.
	stash := func(store map[string]string, tag, m string) string {
		placeholder := fmt.Sprintf("OMN_%s_%d_END", tag, counter)
		store[placeholder] = m
		counter++
		return placeholder
	}

	// 1. Shield raw regions BEFORE the math pass. If not, the inline-math
	//    regex pairs the '$' signs in a JS template literal and tears a note
	//    script apart.
	contentStr = reRaw.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(rawBlocks, "RAW", m)
	})

	// 2. Stash math left in prose. KaTeX runs only on a page that sets
	//    OMN_GO_KATEX, so "$5 and $10" stays text.
	contentStr = reMathBlock.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(mathBlocks, "MATH", m)
	})
	contentStr = reMathInline.ReplaceAllStringFunc(contentStr, func(m string) string {
		return stash(mathBlocks, "MATH", m)
	})

	// 3. Restore the raw regions before rendering so goldmark parses them.
	contentStr = restorePlaceholders(contentStr, rawBlocks)

	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(contentStr), &buf); err != nil {
		return string(mdContent)
	}
	htmlStr := buf.String()

	htmlStr = restorePlaceholders(htmlStr, mathBlocks)

	htmlStr = hrefRe.ReplaceAllStringFunc(htmlStr, func(m string) string {
		match := hrefRe.FindStringSubmatch(m)
		if len(match) < 2 {
			return m
		}
		return `href="` + a.rewriteInternalLink(match[1]) + `"`
	})
	return htmlStr
}

// restorePlaceholders substitutes every placeholder in store back into s. It
// repeats until the string stops changing, so a nested placeholder restores
// whatever order the Go map iterates in.
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

// rewriteInternalLink normalizes an internal page reference: ".md" becomes
// ".html", and a bare page name gets ".html". An external URL, a link that
// already has an extension, and a pure "#anchor" or "?query" link pass
// through. An "intent:" URI must stay byte-identical for Android dispatch.
func (a *App) rewriteInternalLink(href string) string {
	if href == "" {
		return href
	}

	lower := strings.ToLower(href)
	switch {
	case strings.HasPrefix(lower, "http://"),
		strings.HasPrefix(lower, "https://"),
		strings.HasPrefix(href, "//"),
		strings.HasPrefix(lower, "mailto:"),
		strings.HasPrefix(lower, "tel:"),
		strings.HasPrefix(lower, "javascript:"),
		strings.HasPrefix(lower, "data:"),
		strings.HasPrefix(lower, "intent:"),
		strings.HasPrefix(href, "#"):
		return href
	}

	// Split off any query or fragment so the extension rewrite misses it.
	path := href
	suffix := ""
	if idx := strings.IndexAny(href, "?#"); idx >= 0 {
		path = href[:idx]
		suffix = href[idx:]
	}

	// Nothing before the "?" - the link is relative to the current page.
	if path == "" {
		return href
	}

	// Rewrite the last segment only, so "./", "../" and "/" keep their meaning.
	dir := ""
	base := path
	if slash := strings.LastIndex(path, "/"); slash >= 0 {
		dir = path[:slash+1]
		base = path[slash+1:]
	}

	// Directory-only reference - leave as-is.
	if base == "" || base == "." || base == ".." {
		return href
	}

	switch {
	case strings.HasSuffix(base, ".md"):
		base = strings.TrimSuffix(base, ".md") + ".html"
	case extRe.MatchString(base):
		// Already a concrete extension - leave it alone.
	default:
		base += ".html"
	}

	return dir + base + suffix
}

// htmlEscape delegates to escapeHTML in templates.go.
func (a *App) htmlEscape(s string) string {
	return escapeHTML(s)
}

func (a *App) compilePage(name string, mdContent []byte) []byte {
	return a.compilePageWithBody(name, mdContent, "")
}

// compilePageWithBody renders the page shell for one note or asset-edit view.
// A non-empty customBody must be HTML. It replaces the rendered mdContent, so
// the Config page and the external-edit wait page reuse this shell.
func (a *App) compilePageWithBody(name string, mdContent []byte, customBody string) []byte {
	// One header-block split for the whole backend (see frontmatter.go).
	fm := splitFrontMatter(string(mdContent))
	var headers []string
	if fm.HasHeader {
		headers = strings.Split(fm.Header, "\n")
	}

	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = a.renderMarkdownToHTML([]byte(fm.Body))
	}

	// extractTitleTags is shared with the Tags page, so both parse notes alike.
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
		// No escaping here - renderIndexPage escapes each name and value.
		metaTags = append(metaTags, metaTagView{Name: k, Value: v})
	}
	metaTags = append(metaTags, metaTagView{Name: "generator", Value: "OMN-Go " + APP_VERSION})

	// The view page's edit link needs the extension.
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if strings.Contains(name, ".") {
		pageExt = filepath.Ext(name)
	}
	isMarkdown := pageExt == ".md" || pageExt == ""

	// A cached page can be opened from disk (file://), where "/js/..." does
	// not resolve, so its asset paths are relative to its own depth. A
	// custom-body page is served dynamically and keeps absolute paths.
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

// relPrefix returns the "../"-per-level prefix that makes a cached page's
// asset URLs resolve to the storage root over HTTP and from disk (file://).
func relPrefix(name string) string {
	return strings.Repeat("../", strings.Count(name, "/"))
}

func (a *App) ensureHeaderModified(content string, defaultTitle string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	now := time.Now().Format("2006-01-02 15:04:05")

	// Same header-block rule as the rest of the backend (see frontmatter.go).
	fm := splitFrontMatter(content)

	if fm.HasHeader {
		headerLines := strings.Split(fm.Header, "\n")
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
		// A header block always ends with a blank line, empty body or not.
		return strings.Join(headerLines, "\n") + "\n\n" + fm.Body
	}

	authorLine := ""
	if author := a.GetConfig().Author; author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}
