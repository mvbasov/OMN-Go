package backend

import (
	"bytes"
	"path/filepath"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

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

func renderMarkdownToHTML(mdContent []byte) string {
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
	htmlStr = regexp.MustCompile(`href="([^"http#:]+)\.md"`).ReplaceAllString(htmlStr, `href="$1.html"`)
	htmlStr = regexp.MustCompile(`href="([^"\.#:]+)"`).ReplaceAllString(htmlStr, `href="$1.html"`)
	return htmlStr
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

func compilePage(name string, mdContent []byte) []byte {
	return compilePageWithBody(name, mdContent, "")
}

func compilePageWithBody(name string, mdContent []byte, customBody string) []byte {
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
		renderedBody = renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\n")))
	}

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	var metaTags []string
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			k := strings.ToLower(strings.TrimSpace(parts[0]))
			v := htmlEscape(strings.TrimSpace(parts[1]))
			metaTags = append(metaTags, fmt.Sprintf(`    <meta name="%s" content="%s" />`, k, v))
			if k == "title" {
				title = strings.TrimSpace(parts[1])
			}
		}
	}
	metaTags = append(metaTags, fmt.Sprintf(`    <meta name="generator" content="OMN-Go %s" />`, APP_VERSION))

	// Determine file extension for editor use
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if strings.Contains(name, ".") {
		// non-markdown file — keep its extension (e.g. .js, .css, .json)
		pageExt = filepath.Ext(name)
	}
	metaScript := fmt.Sprintf(`    <script>
      var PackageName = 'net.basov.omngo';
      var PageName = '%s';
      var Title = '%s';
      var PAGE_EXT = '%s';
    </script>`, name, title, pageExt)

	// Build tag links for the header
	var tagLinks []string
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "tags") {
			for _, tag := range strings.Split(parts[1], ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tagLinks = append(tagLinks, fmt.Sprintf(`<a href="Tags.html#%s" class="taglink"><span class="tagmark">%s</span></a>`, htmlEscape(tag), htmlEscape(tag)))
				}
			}
		}
	}
	tagsHTML := strings.Join(tagLinks, "\n")

	metaBlock := strings.Join(metaTags, "\n") + "\n" + metaScript

	// Explicitly set IS_MARKDOWN = true for markdown pages (overrides any previous false)
	if pageExt == ".md" || pageExt == "" {
		metaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"
	}
	layout = strings.ReplaceAll(layout, "</head>", metaBlock+"\n</head>")
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", htmlEscape(string(mdContent)))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_TAGS -->", tagsHTML)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")

	return []byte(layout)
}

func ensureHeaderModified(content string, defaultTitle string) string {
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
	if appConfig.Author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}
