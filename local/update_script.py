import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.5 Compiler Escaping Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.4"', 'APP_VERSION = "1.2.5"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.4";', 'const APP_VERSION = "1.2.5";'),
        ("backend/frontend/index.html", "let v = '1.2.4';", "let v = '1.2.5';"),
        ("android/app/build.gradle", "versionCode 10204", "versionCode 10205"),
        ("android/app/build.gradle", 'versionName "1.2.4"', 'versionName "1.2.5"')
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

    # 2. Repair handleSaveNote and handleGetNote using lambda-masked RegEx
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()
        
        # Pure copy of the required logic with standard Python multiline strings
        new_save_note = '''func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}

	content = strings.ReplaceAll(content, "\\r\\n", "\\n")

	var path string
	if !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))
		
		parts := strings.Split(content, "\\n\\n")
		if len(parts) > 0 && strings.Contains(parts[0], ":") {
			headerLines := strings.Split(parts[0], "\\n")
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
			parts[0] = strings.Join(headerLines, "\\n")
			content = strings.Join(parts, "\\n\\n")
		}

		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)

		htmlPath := filepath.Join(storageDir, "html", strings.TrimSuffix(cleanName, ".md")+".html")
		os.MkdirAll(filepath.Dir(htmlPath), 0755)
		compiled := compilePage(strings.TrimSuffix(cleanName, ".md"), []byte(content))
		os.WriteFile(htmlPath, compiled, 0644)

	} else {
		path = filepath.Join(storageDir, filepath.Clean(name))
		os.MkdirAll(filepath.Dir(path), 0755)
		os.WriteFile(path, []byte(content), 0644)
	}

	w.Write([]byte("Saved"))
}'''
        
        # The 'lambda _:' completely disables Python's internal backslash evaluation
        server_code = re.sub(r'func handleSaveNote\(w http\.ResponseWriter, r \*http\.Request\) \{.*?w\.Write\(\[\]byte\("Saved"\)\)\n}', lambda _: new_save_note, server_code, flags=re.DOTALL)
        
        new_get_note = '''func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}
	
	var path string
	var data []byte
	var err error

	if !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {
		cleanName := strings.TrimSuffix(name, ".html")
		if !strings.HasSuffix(cleanName, ".md") {
			cleanName += ".md"
		}
		path = filepath.Join(storageDir, "md", filepath.Clean(cleanName))
		data, err = os.ReadFile(path)
		if err != nil {
			embedPath := "frontend/md/" + cleanName
			data, err = staticFS.ReadFile(embedPath)
			if err != nil {
				title := strings.TrimSuffix(cleanName, ".md")
				timestamp := time.Now().Format("2006-01-02 15:04:05")
				newContent := fmt.Sprintf("Title: %s\\nDate: %s\\nCategory: Notes\\n\\n# %s\\n\\nStart editing this page!", title, timestamp, title)
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, []byte(newContent), 0644)
				data = []byte(newContent)
			} else {
				os.MkdirAll(filepath.Dir(path), 0755)
				os.WriteFile(path, data, 0644)
			}
		}
	} else {
		path = filepath.Join(storageDir, filepath.Clean(name))
		data, err = os.ReadFile(path)
		if err != nil {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
	}
	w.Write(data)
}'''
        
        # Same lambda shield mechanism applied here
        server_code = re.sub(r'func handleGetNote\(w http\.ResponseWriter, r \*http\.Request\) \{.*?w\.Write\(data\)\n}', lambda _: new_get_note, server_code, flags=re.DOTALL)

        with open("backend/server.go", "w", encoding="utf-8") as f:
            f.write(server_code)
        print("  [+] Cleaned and re-injected handleSaveNote and handleGetNote flawlessly.")

    commit_msg = '''fix(compiler): repair literal newlines caused by regex string escaping

- Fixed Go compilation errors ("newline in string") by neutralizing Python `re.sub` replacement string parsing using lambda functions.
- Repaired `handleSaveNote` and `handleGetNote` string payloads (`\\n` and `\\r\\n` injections) returning them to valid Go syntax.
- Bumped application to V1.2.5 (Android 10205).'''

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()