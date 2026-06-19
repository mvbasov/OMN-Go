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
        
    print("  [-] WARNING: Target string NOT FOUND! (Might be handled by fallback or regex)")
    return False

def bump_versions():
    print("\n[VERSION BUMP] Upgrading to 1.3.15")
    versions = [
        ("backend/server.go", 'APP_VERSION = "1.3.14"', 'APP_VERSION = "1.3.15"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.3.14";', 'const APP_VERSION = "1.3.15";'),
        ("android/app/build.gradle", 'versionCode 10314', 'versionCode 10315'),
        ("android/app/build.gradle", 'versionName "1.3.14"', 'versionName "1.3.15"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            if old not in content:
                print(f"  [~] {fp}: Exact old version string not found. Trying dynamic Regex bump...")
                if "build.gradle" in fp:
                    content = re.sub(r'versionCode\s+\d+', 'versionCode 10315', content)
                    content = re.sub(r'versionName\s+"1\.3\.\d+"', 'versionName "1.3.15"', content)
                else:
                    content = re.sub(r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.15"', content)
                    content = re.sub(r'APP_VERSION = \'1\.3\.\d+\'', 'APP_VERSION = "1.3.15"', content)
            else:
                content = content.replace(old, new)
                
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"  [+] Bumped version in {fp}")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.3.15)")
    print("==================================================")
    
    bump_versions()

    # 1. Strip redundant texts from default page generators
    apply_patch("backend/server.go",
        'defaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nModified: %s\\nCategory: Notes%s\\n\\n# %s\\n\\nStart editing this page!", title, now, now, authorLine, title)',
        'defaultContent := fmt.Sprintf("Title: %s\\nDate: %s\\nModified: %s\\nCategory: Notes%s\\n\\n", title, now, now, authorLine)',
        "Strip # Title body from handleNewPage"
    )
    apply_patch("backend/server.go",
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n# %s\\n\\nStart editing this page!", title, timestamp, authorLine, title)',
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n", title, timestamp, authorLine)',
        "Strip # Title body from handleGetNote"
    )
    apply_patch("backend/server.go",
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n# %s\\n\\nStart editing this page!", name, timestamp, authorLine, name)',
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes%s\\n\\n", name, timestamp, authorLine)',
        "Strip # Title body from serveFrontend"
    )
    # Fallbacks in case previous authorLine patches were missed
    apply_patch("backend/server.go",
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n# %s\\n\\nStart editing this page!", title, timestamp, title)',
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n", title, timestamp)',
        "Strip # Title body from handleGetNote (fallback)"
    )
    apply_patch("backend/server.go",
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n# %s\\n\\nStart editing this page!", name, timestamp, name)',
        'fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n", name, timestamp)',
        "Strip # Title body from serveFrontend (fallback)"
    )

    # 2. Exclude External Wait Screen and Config from Backstack natively
    apply_patch("backend/server.go",
        'onclick="window.location.href=\'/%s.html\'"',
        'onclick="window.location.replace(\'/%s.html\')"',
        "Use location.replace() for external editor return button"
    )
    apply_patch("backend/frontend/index.html",
        '<a href="/Config.html" style="background: #444; border-color: #666;">Config</a>',
        '<a href="#" onclick="window.location.replace(\'/Config.html\'); return false;" style="background: #444; border-color: #666;">Config</a>',
        "Use location.replace() for Config menu link"
    )
    apply_patch("backend/frontend/html/js/omn-go-core.js",
        "window.location.href = '/' + fileName + '.html?edit=true';",
        "window.location.replace('/' + fileName + '.html?edit=true');",
        "Use location.replace() for createNewPage redirect"
    )
    apply_patch("backend/frontend/html/js/omn-go-core.js",
        "window.location.href = '/api/edit-external?name=' + encodeURIComponent(currentNote);",
        "window.location.replace('/api/edit-external?name=' + encodeURIComponent(currentNote));",
        "Use location.replace() for external editor API trigger"
    )

    # 3. Completely Rewrite ensureHeaderModified and handleSaveNote using Regex for Bulletproof logic
    print("\n[PATCH] Upgrading core logic functions for bulletproof Modified tracking")
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            content = f.read()

        new_ensure_func = r"""func ensureHeaderModified(content string, defaultTitle string) string {
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
}"""

        new_save_func = r"""func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}

	content = strings.ReplaceAll(content, "\r\n", "\n")

	var path string
	if !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))

		content = ensureHeaderModified(content, strings.TrimSuffix(cleanName, ".md"))

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)

		htmlPath := filepath.Join(storageDir, "html", strings.TrimSuffix(cleanName, ".md")+".html")
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		compiled := compilePage(strings.TrimSuffix(cleanName, ".md"), []byte(content))
		os.WriteFile(htmlPath, compiled, 0644)

	} else {
		path = filepath.Join(storageDir, "html", filepath.Clean(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	w.Write([]byte("Saved"))
}"""

        if "func ensureHeaderModified" in content:
            content = re.sub(r'func ensureHeaderModified\(content string, defaultTitle string\) string \{.*?^\}\r?\n', new_ensure_func + '\n', content, flags=re.MULTILINE | re.DOTALL)
        else:
            content = content.replace("func handleSaveNote", new_ensure_func + "\n\nfunc handleSaveNote")

        content = re.sub(r'func handleSaveNote\(w http\.ResponseWriter, r \*http\.Request\) \{.*?w\.Write\(\[\]byte\("Saved"\)\)\r?\n\}', new_save_func, content, flags=re.DOTALL)

        with open(server_path, "w", encoding="utf-8") as f:
            f.write(content)
        print("  [+] SUCCESS: Core logic functions fully updated via RegEx.")
    else:
        print(f"  [-] ERROR: File {server_path} not found!")

    # 4. Append BFCache listener to dynamically refresh on Android/Desktop browser back buttons
    print("\n[PATCH] Appending BFCache refresh listener to omn-go-core.js")
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            js_content = f.read()

        listener_code = "\nwindow.addEventListener('pageshow', function(event) {\n    if (event.persisted) {\n        window.location.reload();\n    }\n});\n"
        if "pageshow" not in js_content:
            with open(js_path, "a", encoding="utf-8") as f:
                f.write(listener_code)
            print("  [+] SUCCESS: pageshow reload listener added.")
        else:
            print("  [+] SUCCESS: Listener already exists.")
    else:
        print(f"  [-] ERROR: File {js_path} not found!")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "feat(core): fix missing modified logic, backstack exclusions, BFCache refreshing, and clean pelican injection\n\nVersion bumped to 1.3.15"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()