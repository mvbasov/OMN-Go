import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.6 Goldmark Engine Overhaul...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.5"', 'APP_VERSION = "1.2.6"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.5";', 'const APP_VERSION = "1.2.6";'),
        ("backend/frontend/index.html", "let v = '1.2.5';", "let v = '1.2.6';"),
        ("android/app/build.gradle", "versionCode 10205", "versionCode 10206"),
        ("android/app/build.gradle", 'versionName "1.2.5"', 'versionName "1.2.6"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")
            else:
                print(f"  [-] Version string not found in {filepath} (Already updated?)")

    # 2. Patch Dockerfile to fetch Goldmark
    dockerfile = "Dockerfile"
    if os.path.exists(dockerfile):
        with open(dockerfile, "r", encoding="utf-8") as f:
            d_content = f.read()
        
        old_go_get = "RUN go get golang.org/x/mobile@latest && go mod tidy"
        new_go_get = "RUN go get github.com/yuin/goldmark@latest && go get golang.org/x/mobile@latest && go mod tidy"
        
        if old_go_get in d_content:
            d_content = d_content.replace(old_go_get, new_go_get)
            with open(dockerfile, "w", encoding="utf-8") as f:
                f.write(d_content)
            print("  [+] Appended Goldmark fetch to Dockerfile compilation chain.")

    # 3. Patch server.go to replace manual parsing with Goldmark Engine
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # A. Replace Import Block
        new_imports = r"""import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)"""
        server_code = re.sub(r'import \((.*?)\)', lambda _: new_imports, server_code, count=1, flags=re.DOTALL)

        # B. Replace Render Logic Block (from renderMarkdownToHTML down to compilePageWithBody definition)
        new_engine = r"""var mdParser = goldmark.New(
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

\g<1>"""

        # Rip out the old manual engine functions securely
        server_code = re.sub(r'func renderMarkdownToHTML\(mdContent \[\]byte\) string \{.*?(func compilePage\(name string, mdContent \[\]byte\) \[\]byte \{)', new_engine, server_code, flags=re.DOTALL)

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)
        print("  [+] Injected robust Goldmark engine and math isolation into server.go.")

    commit_msg = """feat(parser): replace custom markdown engine with Goldmark

- Integrated github.com/yuin/goldmark replacing manual line-parsing engine.
- Configured html.WithUnsafe() allowing raw Bookmarks.md script arrays to execute seamlessly.
- Wrote regex pre-processor to securely isolate KaTeX $$ blocks, preventing goldmark from accidentally rendering math underscores as italics.
- Preserved native href mapping for completely serverless .html browsing.
- Bumped application to V1.2.6 (Android 10206)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()