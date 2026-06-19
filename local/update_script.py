import os

def update_application():
    # 1. Bump Global Application Version
    versions = [
        ("backend/server.go", 'APP_VERSION = "1.3.10"', 'APP_VERSION = "1.3.11"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.3.10";', 'const APP_VERSION = "1.3.11";'),
        ("android/app/build.gradle", 'versionCode 10310', 'versionCode 10311'),
        ("android/app/build.gradle", 'versionName "1.3.10"', 'versionName "1.3.11"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content.replace(old, new))
                
    server_path = "backend/server.go"
    if not os.path.exists(server_path):
        print("File not found: backend/server.go")
        return

    with open(server_path, "r", encoding="utf-8") as f:
        content = f.read()

    # 1. Config struct
    content = content.replace(
        r"""	GuestPassword string            `json:"guest_password"`
	UseInternalEd bool              `json:"use_internal_editor"`""",
        r"""	GuestPassword string            `json:"guest_password"`
	Author        string            `json:"author"`
	UseInternalEd bool              `json:"use_internal_editor"`"""
    )

    # 2. Config init default
    content = content.replace(
        r"""			GuestPassword: "guest_secret_changeme",
			UseInternalEd: true,""",
        r"""			GuestPassword: "guest_secret_changeme",
			Author:        "Anonymous",
			UseInternalEd: true,"""
    )

    # 3. Inject Helper Function above handleSaveNote
    helper = r"""func ensureHeaderModified(content string, defaultTitle string) string {
	parts := strings.SplitN(content, "\n\n", 2)
	now := time.Now().Format("2006-01-02 15:04:05")

	isHeader := false
	if len(parts) > 0 && strings.Contains(parts[0], ":") {
		firstLine := strings.Split(parts[0], "\n")[0]
		if strings.Contains(firstLine, ":") && !strings.HasPrefix(firstLine, " ") && !strings.HasPrefix(firstLine, "#") {
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

func handleSaveNote"""
    content = content.replace("func handleSaveNote", helper)

    # 4. handleSaveNote update
    old_save_logic = r"""		parts := strings.Split(content, "\n\n")
		if len(parts) > 0 && strings.Contains(parts[0], ":") {
			headerLines := strings.Split(parts[0], "\n")
			modIdx := -1
			for i, l := range headerLines {
				if strings.HasPrefix(l, "Modified:") {
					modIdx = i
					break
				}
			}
			now := time.Now().Format("2006-01-02 15:04:05")
			if modIdx != -1 {
				headerLines[modIdx] = fmt.Sprintf("Modified: %s", now)
			} else {
				headerLines = append(headerLines, fmt.Sprintf("Modified: %s", now))
			}
			parts[0] = strings.Join(headerLines, "\n")
			content = strings.Join(parts, "\n\n")
		}

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)"""
    new_save_logic = r"""		content = ensureHeaderModified(content, strings.TrimSuffix(cleanName, ".md"))

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)"""
    content = content.replace(old_save_logic, new_save_logic)

    # 5. handleQuickNote update
    content = content.replace(
        r"""	fullMarkdown := strings.Join(newContent, "\n")
	os.WriteFile(path, []byte(fullMarkdown), 0644)""",
        r"""	fullMarkdown := strings.Join(newContent, "\n")
	fullMarkdown = ensureHeaderModified(fullMarkdown, "Quick Notes")
	os.WriteFile(path, []byte(fullMarkdown), 0644)"""
    )

    # 6. handleBookmark update
    content = content.replace(
        r"""			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			os.WriteFile(path, []byte(newContent), 0644)""",
        r"""			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			newContent = ensureHeaderModified(newContent, "Incoming bookmarks")
			os.WriteFile(path, []byte(newContent), 0644)"""
    )

    # 7. handleGetNote default update
    content = content.replace(
        r"""				timestamp := time.Now().Format("2006-01-02 15:04:05")
				newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n# %s\n\nStart editing this page!", title, timestamp, title)""",
        r"""				timestamp := time.Now().Format("2006-01-02 15:04:05")
				authorLine := ""
				if appConfig.Author != "" {
					authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
				}
				newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n# %s\n\nStart editing this page!", title, timestamp, authorLine, title)"""
    )

    # 8. serveFrontend default update
    content = content.replace(
        r"""					timestamp := time.Now().Format("2006-01-02 15:04:05")
					defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n# %s\n\nStart editing this page!", name, timestamp, name)""",
        r"""					timestamp := time.Now().Format("2006-01-02 15:04:05")
					authorLine := ""
					if appConfig.Author != "" {
						authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
					}
					defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes%s\n\n# %s\n\nStart editing this page!", name, timestamp, authorLine, name)"""
    )

    # 9. serveFrontend external edit hook
    old_ext_edit = r"""			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				compiled := compilePage(name, mdContent)
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}"""
    new_ext_edit = r"""			mdContent, err := os.ReadFile(mdPath)
			if err == nil {
				if errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime()) {
					updatedContent := ensureHeaderModified(string(mdContent), name)
					if updatedContent != string(mdContent) {
						os.WriteFile(mdPath, []byte(updatedContent), 0644)
						mdContent = []byte(updatedContent)
					}
				}
				compiled := compilePage(name, mdContent)
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}"""
    content = content.replace(old_ext_edit, new_ext_edit)

    # 10. getConfigPageBody HTML
    old_html = r"""        <div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
            <input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />"""
    new_html = r"""        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Author Name</label>
            <input type="text" id="cfgAuthor" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
        </div>
        <div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
            <input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />"""
    content = content.replace(old_html, new_html)

    # 11. getConfigPageBody JS
    old_js = r"""        params.append("guest_password", document.getElementById("cfgGuestPwd").value);
        params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");"""
    new_js = r"""        params.append("guest_password", document.getElementById("cfgGuestPwd").value);
        params.append("author", document.getElementById("cfgAuthor").value);
        params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");"""
    content = content.replace(old_js, new_js)

    # 12. getConfigPageBody Args
    old_args = '`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword,\n\t\tfunc() string {'
    new_args = '`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,\n\t\tfunc() string {'
    content = content.replace(old_args, new_args)

    # 13. handleConfig Set
    old_set = '\t\tappConfig.GuestPassword = r.FormValue("guest_password")\n\t\tappConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"'
    new_set = '\t\tappConfig.GuestPassword = r.FormValue("guest_password")\n\t\tappConfig.Author = r.FormValue("author")\n\t\tappConfig.UseInternalEd = r.FormValue("use_internal_editor") == "true"'
    content = content.replace(old_set, new_set)

    with open(server_path, "w", encoding="utf-8") as f:
        f.write(content)
    
    commit_msg = "feat(core): dynamic Author and Modified Pelican headers\n\nVersion bumped to 1.3.11"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()