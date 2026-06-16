import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.4 Package Renaming & Editor Routing Fix...")

    # 1. Java Package & Directory Renaming
    old_java_dir = os.path.join("android", "app", "src", "main", "java", "net", "basov", "goomn")
    new_java_dir = os.path.join("android", "app", "src", "main", "java", "net", "basov", "omngo")
    old_java_file = os.path.join(old_java_dir, "MainActivity.java")
    new_java_file = os.path.join(new_java_dir, "MainActivity.java")

    if os.path.exists(old_java_file):
        os.makedirs(new_java_dir, exist_ok=True)
        with open(old_java_file, "r", encoding="utf-8") as f:
            java_code = f.read()
        
        # Replace package name, gomobile library import, and storage path cleanly
        java_code = java_code.replace("net.basov.goomn", "net.basov.omngo")
        
        with open(new_java_file, "w", encoding="utf-8") as f:
            f.write(java_code)
        
        os.remove(old_java_file)
        try: os.rmdir(old_java_dir)
        except: pass
        print("  [+] Relocated and refactored MainActivity.java to net.basov.omngo")
    elif os.path.exists(new_java_file):
        print("  [=] MainActivity.java already relocated.")
    else:
        print("  [!] WARNING: Could not find MainActivity.java in either directory.")

    # 2. Update File Contents globally for the rename and versions
    replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.3"', 'APP_VERSION = "1.2.4"'),
        ("backend/server.go", '"/storage/emulated/0/Android/media/net.basov.goomn"', '"/storage/emulated/0/Android/media/net.basov.omngo"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.3";', 'const APP_VERSION = "1.2.4";'),
        ("backend/frontend/index.html", "let v = '1.2.3';", "let v = '1.2.4';"),
        ("android/app/build.gradle", "versionCode 10203", "versionCode 10204"),
        ("android/app/build.gradle", 'versionName "1.2.3"', 'versionName "1.2.4"'),
        ("android/app/build.gradle", "namespace 'net.basov.goomn'", "namespace 'net.basov.omngo'"),
        ("android/app/build.gradle", 'applicationId "net.basov.goomn"', 'applicationId "net.basov.omngo"'),
        ("android/app/src/main/AndroidManifest.xml", 'package="net.basov.goomn"', 'package="net.basov.omngo"'),
        ("Dockerfile", "-javapkg net.basov.goomn", "-javapkg net.basov.omngo"),
        ("go.mod", "module net.basov.goomn", "module net.basov.omngo"),
        ("main_desktop.go", '"net.basov.goomn/backend"', '"net.basov.omngo/backend"'),
    ]

    for filepath, old_val, new_val in replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Patched {filepath}: replaced string successfully.")
            elif new_val in content:
                pass # Already patched

    # 3. Complex Regex Replacements for server.go logic and frontend Config logic
    
    # Patch backend/server.go (handleSaveNote + handleGetNote to firmly handle ext-less files)
    if os.path.exists("backend/server.go"):
        with open("backend/server.go", "r", encoding="utf-8") as f:
            server_code = f.read()
        
        # Robust replacement for handleSaveNote
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
        server_code = re.sub(r'func handleSaveNote\(w http\.ResponseWriter, r \*http\.Request\) \{.*?w\.Write\(\[\]byte\("Saved"\)\)\n}', new_save_note, server_code, flags=re.DOTALL)
        
        # Robust replacement for handleGetNote
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
        server_code = re.sub(r'func handleGetNote\(w http\.ResponseWriter, r \*http\.Request\) \{.*?w\.Write\(data\)\n}', new_get_note, server_code, flags=re.DOTALL)

        with open("backend/server.go", "w", encoding="utf-8") as f:
            f.write(server_code)
        print("  [+] Rebuilt handleSaveNote and handleGetNote perfectly in server.go")

    # Patch backend/frontend/index.html (Hide Config Edit button)
    if os.path.exists("backend/frontend/index.html"):
        with open("backend/frontend/index.html", "r", encoding="utf-8") as f:
            html_code = f.read()

        old_onload = '''window.onload = () => {
            checkSession();
            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            let hash = window.location.hash;
            if (hash) {
                let el = document.getElementById(hash.substring(1));
                if (el) el.scrollIntoView();
            }
        };'''

        new_onload = '''window.onload = () => {
            checkSession();
            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            let hash = window.location.hash;
            if (hash) {
                let el = document.getElementById(hash.substring(1));
                if (el) el.scrollIntoView();
            }
        };'''
        if old_onload in html_code:
            html_code = html_code.replace(old_onload, new_onload)
            with open("backend/frontend/index.html", "w", encoding="utf-8") as f:
                f.write(html_code)
            print("  [+] Patched window.onload in index.html to hide Config Edit button")

    commit_msg = '''fix(core): finalize android packaging and editor routing rules

- Corrected native Android Java package layout from net.basov.goomn to net.basov.omngo.
- Altered module path, storage directives, and Dockerfile bindings to use new namespace.
- Completely refactored server.go handleSaveNote and handleGetNote using full-block regex to guarantee extensionless files (QuickNotes) save strictly to the /md/ directory.
- Hidden the 'Edit Mode' button when interacting with the dynamic Config dashboard.
- Bumped application to V1.2.4 (Android 10204).'''

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()