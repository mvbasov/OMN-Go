import os
import re

def apply_patch(filepath, old_str, new_str, description):
    print(f"\n[PATCH] {description}")
    print(f"  Target: {filepath}")
    
    if not os.path.exists(filepath):
        print(f"  [-] ERROR: File {filepath} not found!")
        return False
        
    with open(filepath, "r", encoding="utf-8") as f:
        content = f.read()
        
    if new_str in content:
        print("  [+] SUCCESS: Patch appears to be already applied from a previous run.")
        return True
        
    if old_str in content:
        content = content.replace(old_str, new_str)
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)
        print("  [+] SUCCESS: Exact string match replaced.")
        return True
        
    old_normalized = old_str.replace('\r\n', '\n')
    content_normalized = content.replace('\r\n', '\n')
    
    if old_normalized in content_normalized:
        content_normalized = content_normalized.replace(old_normalized, new_str.replace('\r\n', '\n'))
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content_normalized)
        print("  [+] SUCCESS: Normalized match replaced.")
        return True
        
    print("  [-] ERROR: Target string NOT FOUND!")
    return False

def bump_versions():
    print("\n[VERSION BUMP] Upgrading to 1.4.4")
    
    versions = [
        ("backend/config.go", 'APP_VERSION = "1.4.2"', 'APP_VERSION = "1.4.4"'),
        ("backend/config.go", 'APP_VERSION = "1.4.3"', 'APP_VERSION = "1.4.4"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.4.2";', 'const APP_VERSION = "1.4.4";'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.4.3";', 'const APP_VERSION = "1.4.4";'),
        ("android/app/build.gradle", 'versionCode 10402', 'versionCode 10404'),
        ("android/app/build.gradle", 'versionCode 10403', 'versionCode 10404'),
        ("android/app/build.gradle", 'versionName "1.4.2"', 'versionName "1.4.4"'),
        ("android/app/build.gradle", 'versionName "1.4.3"', 'versionName "1.4.4"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            if old in content:
                content = content.replace(old, new)
                with open(fp, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {fp}")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.4.4)")
    print("==================================================")
    
    bump_versions()

    # 1. API Handlers: Replace Pelican with Raw MD Tag (handleNewPage)
    old_newpage = (
        '\tnow := time.Now().Format("2006-01-02 15:04:05")\n\n'
        '\ttargetMdPath := filepath.Join(storageDir, "md", target+".md")\n'
        '\tif _, err := os.Stat(targetMdPath); os.IsNotExist(err) {\n'
        '\t\tauthorLine := ""\n'
        '\t\tif appConfig.Author != "" {\n'
        '\t\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t\t}\n'
        '\t\tdefaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nModified: %s\\nCategory: Notes%s\\n\\n", title, now, now, authorLine)'
    )
    new_newpage = (
        '\ttargetMdPath := filepath.Join(storageDir, "md", target+".md")\n'
        '\tif _, err := os.Stat(targetMdPath); os.IsNotExist(err) {\n'
        '\t\tdefaultContent := "<!-- OMN_GO_RAW_MD -->\\n\\n"'
    )
    apply_patch("backend/handlers_api.go", old_newpage, new_newpage, "[1.4.3] Strip Pelican headers in handleNewPage")

    # 2. API Handlers: Replace Pelican with Raw MD Tag (handleGetNote)
    old_getnote = (
        '\t\thumanTitle := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")\n'
        '\t\ttimestamp := time.Now().Format("2006-01-02 15:04:05")\n'
        '\t\tauthorLine := ""\n'
        '\t\tif appConfig.Author != "" {\n'
        '\t\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t\t}\n'
        '\t\tdefaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n", humanTitle, timestamp, authorLine)'
    )
    new_getnote = '\t\tdefaultContent := "<!-- OMN_GO_RAW_MD -->\\n\\n"'
    apply_patch("backend/handlers_api.go", old_getnote, new_getnote, "[1.4.3] Strip Pelican headers in handleGetNote")

    # 3. Web Handlers: Replace Pelican with Raw MD Tag (serveFrontend)
    old_serve1 = (
        '\t\t\t\ttimestamp := time.Now().Format("2006-01-02 15:04:05")\n'
        '\t\t\t\tauthorLine := ""\n'
        '\t\t\t\tif appConfig.Author != "" {\n'
        '\t\t\t\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t\t\t\t}\n'
        '\t\t\t\thumanName := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")\n'
        '\t\t\t\tdefaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n", humanName, timestamp, authorLine)'
    )
    new_serve1 = '\t\t\t\tdefaultContent := "<!-- OMN_GO_RAW_MD -->\\n\\n"'
    apply_patch("backend/handlers_web.go", old_serve1, new_serve1, "[1.4.3] Strip Pelican headers in serveFrontend fallback")

    # 4. Markdown Logic: ensureHeaderModified fallback -> Raw MD Tag
    old_markdown1 = (
        '\tauthorLine := ""\n'
        '\tif appConfig.Author != "" {\n'
        '\t\tauthorLine = fmt.Sprintf("\\nAuthor: %s", appConfig.Author)\n'
        '\t}\n'
        '\treturn fmt.Sprintf("Title: %s\\nDate: %s\\nModified: %s%s\\n\\n%s", defaultTitle, now, now, authorLine, content)'
    )
    new_markdown1 = (
        '\tif strings.HasPrefix(strings.TrimSpace(content), "<!-- OMN_GO_RAW_MD -->") {\n'
        '\t\treturn content\n'
        '\t}\n'
        '\treturn fmt.Sprintf("<!-- OMN_GO_RAW_MD -->\\n\\n%s", content)'
    )
    apply_patch("backend/markdown.go", old_markdown1, new_markdown1, "[1.4.3] Update ensureHeaderModified fallback to use Raw MD Tag")

    # 5. Markdown Logic: Isolate output to Static HTML Format
    old_markdown2 = r"""	htmlStr := string(frontendHTML)
	htmlStr = strings.Replace(htmlStr, "{{TITLE}}", title, 1)
	htmlStr = strings.Replace(htmlStr, "{{CONTENT}}", buf.String(), 1)
	return []byte(htmlStr)"""
    new_markdown2 = r"""	const staticHTMLTemplate = `<!DOCTYPE html>
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
	return []byte(htmlStr)"""
    apply_patch("backend/markdown.go", old_markdown2, new_markdown2, "[1.4.4] Decouple UI Shell; compile files as Static HTML Sites")

    # 6. Web Handlers: Dynamically inject Static Output back into App Shell
    old_serve2 = r"""		content, err := os.ReadFile(htmlPath)
		if err == nil {
			if !appConfig.UseInternalEd {
				content = bytes.Replace(content, []byte(`id="toggleBtn"`), []byte(`id="toggleBtn" style="display:none;"`), 1)
			}
			w.Write(content)
			return
		}"""
    
    new_serve2 = r"""		content, err := os.ReadFile(htmlPath)
		if err == nil {
			contentStr := string(content)
			startMarker := "<!-- OMN_CONTENT_START -->\n"
			endMarker := "\n<!-- OMN_CONTENT_END -->"
			
			startIdx := strings.Index(contentStr, startMarker)
			endIdx := strings.Index(contentStr, endMarker)
			
			var payload string
			if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
				payload = contentStr[startIdx+len(startMarker) : endIdx]
			} else {
				if mdData, errMd := os.ReadFile(mdPath); errMd == nil {
					newCompiled := compilePage(name, mdData)
					os.WriteFile(htmlPath, newCompiled, 0644)
					contentStr = string(newCompiled)
					startIdx = strings.Index(contentStr, startMarker)
					endIdx = strings.Index(contentStr, endMarker)
					if startIdx != -1 && endIdx != -1 && endIdx > startIdx {
						payload = contentStr[startIdx+len(startMarker) : endIdx]
					} else {
						payload = contentStr
					}
				} else {
					payload = contentStr
				}
			}

			appShell := string(frontendHTML)
			humanName := strings.ReplaceAll(strings.ReplaceAll(name, "-", " "), "_", " ")
			appShell = strings.Replace(appShell, "{{TITLE}}", humanName, 1)
			appShell = strings.Replace(appShell, "{{CONTENT}}", payload, 1)
			
			if !appConfig.UseInternalEd {
				appShell = strings.Replace(appShell, `id="toggleBtn"`, `id="toggleBtn" style="display:none;"`, 1)
			}
			
			w.Write([]byte(appShell))
			return
		}"""
    apply_patch("backend/handlers_web.go", old_serve2, new_serve2, "[1.4.4] Dynamically read and inject static HTML back into RAM App Shell")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "feat(core): switch to static HTML file storage and dynamic memory-based UI shell injection\n\nVersion bumped to 1.4.4"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()