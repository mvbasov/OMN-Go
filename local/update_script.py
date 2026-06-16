import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.2 Layout Engine Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.1"', 'APP_VERSION = "1.2.2"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.1";', 'const APP_VERSION = "1.2.2";'),
        ("backend/frontend/index.html", "let v = '1.2.1';", "let v = '1.2.2';"),
        ("android/app/build.gradle", 'versionCode 10201', 'versionCode 10202'),
        ("android/app/build.gradle", 'versionName "1.2.1"', 'versionName "1.2.2"')
    ]

    for filepath, old_v, new_v in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_v in content:
                content = content.replace(old_v, new_v)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")
            else:
                print(f"  [-] Version string not found in {filepath} (Already updated?)")

    # 2. Patch compilePage inside server.go
    server_go = "backend/server.go"
    if not os.path.exists(server_go):
        raise ValueError(f"Missing mandatory file: {server_go}")

    with open(server_go, "r", encoding="utf-8") as f:
        server_content = f.read()

    # Target compilePage block
    old_compile_page = """func compilePage(name string, mdContent []byte) []byte {
	var headers []string
	var bodyLines []string
	inHeader := true

	lines := strings.Split(string(mdContent), "\\n")
	for _, line := range lines {
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

	renderedBody := renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\\n")))
	metadataStr := strings.Join(headers, "\\n")

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	for _, h := range headers {
		if strings.HasPrefix(h, "Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(h, "Title:"))
			break
		}
	}

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", string(mdContent))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", metadataStr)

	return []byte(layout)
}"""

    new_compile_page = """func compilePage(name string, mdContent []byte) []byte {
	return compilePageWithBody(name, mdContent, "")
}

func compilePageWithBody(name string, mdContent []byte, customBody string) []byte {
	var headers []string
	var bodyLines []string
	inHeader := true

	lines := strings.Split(string(mdContent), "\\n")
	for _, line := range lines {
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
		renderedBody = renderMarkdownToHTML([]byte(strings.Join(bodyLines, "\\n")))
	}
	metadataStr := strings.Join(headers, "\\n")

	layout := string(frontendHTML)

	title := "OMN-Go - " + name
	for _, h := range headers {
		if strings.HasPrefix(h, "Title:") {
			title = strings.TrimSpace(strings.TrimPrefix(h, "Title:"))
			break
		}
	}

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PAGE_TITLE -->", title)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_PREVIEW_BODY -->", renderedBody)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", string(mdContent))
	layout = strings.ReplaceAll(layout, "/* OMN_GO_PAGE_NAME_JS */", fmt.Sprintf(`let currentNote = "%s";`, name))
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", metadataStr)

	return []byte(layout)
}"""

    # Target handleConfig block
    old_handle_config = """		if name == "Config" {
			w.Header().Set("Content-Type", "text/html")
			compiled := compilePage("Config", []byte("Title: Config\\nCategory: Settings\\n\\n"))
			body := getConfigPageBody()
			htmlStr := strings.Replace(string(compiled), "<!-- OMN_GO_PREVIEW_BODY -->", body, 1)
			w.Write([]byte(htmlStr))
			return
		}"""

    new_handle_config = """		if name == "Config" {
			w.Header().Set("Content-Type", "text/html")
			body := getConfigPageBody()
			compiled := compilePageWithBody("Config", []byte("Title: Config\\nCategory: Settings\\n\\n"), body)
			w.Write(compiled)
			return
		}"""

    # Target handleEditExternal block
    old_edit_external = """	w.Header().Set("Content-Type", "text/html")
	pageName := strings.TrimSuffix(cleanName, ".md")
	compiledWait := compilePage(pageName, []byte(fmt.Sprintf("Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", pageName, time.Now().Format("2006-01-02 15:04:05"))))
	
	waitBody := getExternalEditPageBody(pageName)
	htmlStr := strings.Replace(string(compiledWait), "<!-- OMN_GO_PREVIEW_BODY -->", waitBody, 1)
	w.Write([]byte(htmlStr))"""

    new_edit_external = """	w.Header().Set("Content-Type", "text/html")
	pageName := strings.TrimSuffix(cleanName, ".md")
	waitBody := getExternalEditPageBody(pageName)
	compiledWait := compilePageWithBody(pageName, []byte(fmt.Sprintf("Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", pageName, time.Now().Format("2006-01-02 15:04:05"))), waitBody)
	w.Write(compiledWait)"""

    # Apply patches
    patches = [
        (old_compile_page, new_compile_page, "compilePageWithBody infrastructure"),
        (old_handle_config, new_handle_config, "config page body generation"),
        (old_edit_external, new_edit_external, "external edit waiting page body generation")
    ]

    for old_str, new_str, desc in patches:
        if old_str in server_content:
            server_content = server_content.replace(old_str, new_str)
            print(f"  [+] Patched {desc} inside server.go")
        elif new_str in server_content:
            print(f"  [=] {desc} is already up to date inside server.go")
        else:
            raise ValueError(f"Failing to find replacement hook for: {desc}")

    with open(server_go, "w", encoding="utf-8") as f:
        f.write(server_content)

    commit_msg = """fix(engine): resolve blank admin views by adding compilePageWithBody

- Fixed blank 'Config' page on desktop by passing pre-rendered form straight to compilePageWithBody.
- Fixed blank external editor refresh interface by bypassing secondary placeholder replacements.
- Preserved Layout wrappers for both virtual system states.
- Bumped application version to 1.2.2."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()