package backend

import (
	"bytes"
	"fmt"
	"html/template"
	"path/filepath"
	"regexp"
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

func (a *App) renderMarkdownToHTML(mdContent []byte) string {
	contentStr := string(mdContent)
	mathBlocks := make(map[string]string)
	counter := 0

	// Protect complex KaTeX Math blocks from markdown emphasis corruption
	contentStr = regexp.MustCompile(`(?s)\$\$.*?\$\$`).ReplaceAllStringFunc(contentStr, func(m string) string {
		placeholder := fmt.Sprintf("OMN_MATH_BLOCK_%d", counter)
		mathBlocks[placeholder] = m
		counter++
		return placeholder
	})
	contentStr = regexp.MustCompile(`\$[^\$]+\$`).ReplaceAllStringFunc(contentStr, func(m string) string {
		placeholder := fmt.Sprintf("OMN_MATH_INLINE_%d", counter)
		mathBlocks[placeholder] = m
		counter++
		return placeholder
	})

	var buf bytes.Buffer
	if err := mdParser.Convert([]byte(contentStr), &buf); err != nil {
		return string(mdContent)
	}
	htmlStr := buf.String()

	// Restore math blocks natively for the offline KaTeX frontend
	for placeholder, original := range mathBlocks {
		htmlStr = strings.ReplaceAll(htmlStr, placeholder, original)
	}

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
// concrete extension (.html, .js, .css, .png, ...), any external URL
// (http(s)://, //, mailto:, tel:, javascript:, data:), and any link that is
// purely an anchor or query string is passed through unchanged.
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
		strings.HasPrefix(href, "#"):
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

func (a *App) htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func (a *App) compilePage(name string, mdContent []byte) []byte {
	return a.compilePageWithBody(name, mdContent, "", false)
}

// compilePageWithBody renders the full page shell (indexPageTmpl) for a
// single note/page/asset-edit view.
//
// customBody, when non-empty, is used as the (already-HTML) main content
// instead of rendering mdContent as markdown - this is how the "view raw
// file for external/internal editing" and Config-dashboard pages reuse the
// same page shell without being markdown themselves.
//
// isEditMode marks this render as an in-browser edit view for a
// non-markdown asset (a .js/.css/.json file opened via ?edit=true): it
// forces IsMarkdown off and asks the template to auto-switch into edit
// mode on load, matching what a separate post-render script injection used
// to bolt on after the fact.
func (a *App) compilePageWithBody(name string, mdContent []byte, customBody string, isEditMode bool) []byte {
	var headers []string
	var bodyLines []string
	inHeader := true

	lines := strings.SplitSeq(string(mdContent), "\n")
	for line := range lines {
		trimmed := strings.TrimSpace(line)
		if inHeader {
			if trimmed == "" {
				inHeader = false
				continue
			}
			if strings.Contains(line, ":") {
				headers = append(headers, line)
			} else {
				inHeader = false
				bodyLines = append(bodyLines, line)
			}
		} else {
			bodyLines = append(bodyLines, line)
		}
	}

	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = a.renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\n")))
	}

	title := "OMN-Go - " + name
	var metaTags []metaTagView
	var tags []string
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(parts[0]))
		v := strings.TrimSpace(parts[1])
		// No manual escaping here (unlike the old a.htmlEscape(v) call) -
		// indexPageTmpl's {{.Value}} is inside an HTML attribute, and
		// html/template escapes attribute contexts correctly on its own.
		metaTags = append(metaTags, metaTagView{Name: k, Value: v})
		if k == "title" {
			title = v
		}
		if k == "tags" {
			for _, tag := range strings.Split(v, ",") {
				if tag = strings.TrimSpace(tag); tag != "" {
					tags = append(tags, tag)
				}
			}
		}
	}
	metaTags = append(metaTags, metaTagView{Name: "generator", Value: "OMN-Go " + APP_VERSION})

	// Determine file extension for editor use
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if strings.Contains(name, ".") {
		// non-markdown file — keep its extension (e.g. .js, .css, .json)
		pageExt = filepath.Ext(name)
	}
	// isEditMode is only ever true for a non-markdown asset (see the
	// doc comment above), so pageExt is never ".md"/"" in that case
	// anyway - the explicit !isEditMode guard is defensive belt-and-braces
	// rather than load-bearing.
	isMarkdown := !isEditMode && (pageExt == ".md" || pageExt == "")

	view := indexPageView{
		Title:       title,
		PackageName: "net.basov.omngo",
		PageName:    name,
		PageExt:     pageExt,
		IsMarkdown:  isMarkdown,
		IsEditMode:  isEditMode,
		MetaTags:    metaTags,
		Tags:        tags,
		RawMD:       string(mdContent),
		PreviewBody: template.HTML(renderedBody),
	}

	return []byte(renderTemplate(indexPageTmpl, view, "compilePageWithBody"))
}

func (a *App) ensureHeaderModified(content string, defaultTitle string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	parts := strings.SplitN(content, "\n\n", 2)
	now := time.Now().Format("2006-01-02 15:04:05")

	isHeader := false
	if len(parts) > 0 && strings.Contains(parts[0], ":") {
		lines := strings.Split(parts[0], "\n")
		if len(lines) > 0 && strings.Contains(lines[0], ":") && !strings.HasPrefix(lines[0], " ") && !strings.HasPrefix(lines[0], "#") && !strings.HasPrefix(lines[0], "<") {
			isHeader = true
		}
	}

	if isHeader {
		headerLines := strings.Split(parts[0], "\n")
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
		parts[0] = strings.Join(headerLines, "\n")
		if len(parts) > 1 {
			return parts[0] + "\n\n" + parts[1]
		}
		return parts[0] + "\n\n"
	}

	authorLine := ""
	if author := a.GetConfig().Author; author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}
