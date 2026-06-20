package backend

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

func compilePage(title string, mdContent []byte) []byte {
	content := string(mdContent)
	parts := strings.SplitN(content, "\n\n", 2)
	body := content
	if len(parts) > 1 && strings.Contains(parts[0], ":") {
		firstLine := strings.Split(parts[0], "\n")[0]
		if strings.Contains(firstLine, ":") && !strings.HasPrefix(firstLine, " ") && !strings.HasPrefix(firstLine, "#") && !strings.HasPrefix(firstLine, "<") {
			body = parts[1]
		}
	}

	re := regexp.MustCompile(`(?s)\$\$.*?\$\$`)
	body = re.ReplaceAllStringFunc(body, func(m string) string {
		return strings.ReplaceAll(m, "_", "\\_")
	})

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Typographer),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithHardWraps(), html.WithUnsafe()),
	)
	var buf bytes.Buffer
	if err := md.Convert([]byte(body), &buf); err != nil {
		return []byte(err.Error())
	}

	const staticHTMLTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{TITLE}}</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Helvetica, Arial, sans-serif; line-height: 1.6; padding: 20px; max-width: 900px; margin: 0 auto; color: #333; }
pre { background: #f4f4f4; padding: 10px; border-radius: 5px; overflow-x: auto; }
code { font-family: monospace; padding: 2px 4px; background: #f4f4f4; border-radius: 3px; }
img { max-width: 100%; height: auto; }
blockquote { border-left: 4px solid #ccc; margin: 0; padding-left: 10px; color: #666; }
</style>
</head>
<body>
<!-- OMN_CONTENT_START -->
{{CONTENT}}
<!-- OMN_CONTENT_END -->
</body>
</html>`

	htmlStr := strings.Replace(staticHTMLTemplate, "{{TITLE}}", title, 1)
	htmlStr = strings.Replace(htmlStr, "{{CONTENT}}", buf.String(), 1)
	return []byte(htmlStr)
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

	if strings.HasPrefix(strings.TrimSpace(content), "<!-- OMN_GO_RAW_MD -->") {
		return content
	}
	return fmt.Sprintf("<!-- OMN_GO_RAW_MD -->\n\n%s", content)
}
