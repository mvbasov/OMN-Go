import os
import re

def update_application():
    # 1. Update server.go to inject Pelican headers as HTML metadata
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            server_code = f.read()
            
        old_server_block = """\
	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\\n")))
	}
	metadataStr := fmt.Sprintf("File: %s.md\\n%s", name, strings.Join(headers, "\\n"))

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	for _, h := range headers {
		if after, ok := strings.CutPrefix(h, "Title:"); ok {
			title = strings.TrimSpace(after)
			break
		}
	}

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", htmlEscape(string(mdContent)))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", metadataStr)\
"""

        new_server_block = """\
	renderedBody := customBody
	if renderedBody == "" {
		renderedBody = renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\\n")))
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

	metaScript := fmt.Sprintf(`    <script>
      var PackageName = 'net.basov.omngo';
      var PageName = '%s';
      var Title = '%s';
    </script>`, name, title)

	metaBlock := strings.Join(metaTags, "\\n") + "\\n" + metaScript

	layout = strings.ReplaceAll(layout, "</head>", metaBlock+"\\n</head>")
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", htmlEscape(string(mdContent)))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")\
"""
        server_code = server_code.replace(old_server_block, new_server_block)
        server_code = re.sub(r'const APP_VERSION = "1\.3\.\d+"', 'const APP_VERSION = "1.3.6"', server_code)
        with open(server_path, "w", encoding="utf-8") as f:
            f.write(server_code)

    # 2. Update index.html to declare the KaTeX variable
    index_path = "backend/frontend/index.html"
    if os.path.exists(index_path):
        with open(index_path, "r", encoding="utf-8") as f:
            html = f.read()

        html = re.sub(
            r'const APP_VERSION = "1\.3\.\d+";',
            'const APP_VERSION = "1.3.6";\n        let OMN_GO_KATEX = false;',
            html
        )
        with open(index_path, "w", encoding="utf-8") as f:
            f.write(html)

    # 3. Update omn-go-core.js to enforce KaTeX toggle and parse Metadata Panel
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            js = f.read()

        # Enforce KaTeX checking on both initialization and mutation observer
        js = js.replace(
            "if (window.renderMathInElement) {", 
            "if (typeof OMN_GO_KATEX !== 'undefined' && OMN_GO_KATEX && window.renderMathInElement) {"
        )

        metadata_extractor = """
// --- Dynamic Metadata Panel Extractor ---
document.addEventListener("DOMContentLoaded", () => {
    const panel = document.getElementById('metadataPanel');
    if (panel) {
        let metaHtml = `<div style="margin-bottom: 8px; color: #0056b3; font-weight: bold; border-bottom: 1px solid #ccc; padding-bottom: 4px;">File: ${typeof PageName !== 'undefined' ? PageName + '.md' : ''}</div>`;
        document.querySelectorAll('meta').forEach(m => {
            const name = m.getAttribute('name');
            const content = m.getAttribute('content');
            if (name && content && !['viewport', 'charset'].includes(name.toLowerCase())) {
                metaHtml += `<div style="margin-bottom: 4px;"><strong>${name.charAt(0).toUpperCase() + name.slice(1)}:</strong> ${content}</div>`;
            }
        });
        panel.innerHTML = metaHtml;
    }
});
"""
        if "Dynamic Metadata Panel Extractor" not in js:
            js += "\n" + metadata_extractor
        
        # Bump JS version string
        js = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.6';", js)

        with open(js_path, "w", encoding="utf-8") as f:
            f.write(js)

    # 4. Bump Android build.gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            gradle_code = f.read()
        gradle_code = re.sub(r'versionCode \d+', 'versionCode 10306', gradle_code)
        gradle_code = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.6"', gradle_code)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(gradle_code)

    print("SUCCESS: Core KaTeX toggle implemented and Pelican headers mapped to native HTML metadata.")
    
    commit_msg = """feat(core): dynamic katex toggle & native html metadata mapping\n\nImplemented strict `OMN_GO_KATEX` toggle for math processing. Upgraded Pelican headers to render as native HTML `<meta>` and `<script>` blocks, with an interactive JS-driven metadata viewer panel. Version bumped to 1.3.6."""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()