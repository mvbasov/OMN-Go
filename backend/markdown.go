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

	htmlStr := string(frontendHTML)
	htmlStr = strings.Replace(htmlStr, "{{TITLE}}", title, 1)
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

	authorLine := ""
	if appConfig.Author != "" {
		authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
	}
	return fmt.Sprintf("Title: %s\nDate: %s\nModified: %s%s\n\n%s", defaultTitle, now, now, authorLine, content)
}
