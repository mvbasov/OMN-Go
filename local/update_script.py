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
    print("\n[VERSION BUMP] Upgrading to 1.3.14")
    versions = [
        ("backend/server.go", 'APP_VERSION = "1.3.13"', 'APP_VERSION = "1.3.14"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.3.13";', 'const APP_VERSION = "1.3.14";'),
        ("android/app/build.gradle", 'versionCode 10313', 'versionCode 10314'),
        ("android/app/build.gradle", 'versionName "1.3.13"', 'versionName "1.3.14"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            if old not in content:
                print(f"  [~] {fp}: Exact old version string not found. Trying dynamic Regex bump...")
                if "build.gradle" in fp:
                    content = re.sub(r'versionCode\s+\d+', 'versionCode 10314', content)
                    content = re.sub(r'versionName\s+"1\.3\.\d+"', 'versionName "1.3.14"', content)
                else:
                    content = re.sub(r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.14"', content)
                    content = re.sub(r'APP_VERSION = \'1\.3\.\d+\'', 'APP_VERSION = "1.3.14"', content)
            else:
                content = content.replace(old, new)
                
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"  [+] Bumped version in {fp}")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.3.14)")
    print("==================================================")
    
    bump_versions()

    # 1. Inject /api/newpage Handler Logic into Go Backend
    old_func = 'func handleSaveNote(w http.ResponseWriter, r *http.Request) {'
    new_func = r"""func handleNewPage(w http.ResponseWriter, r *http.Request) {
	source := r.FormValue("source")
	target := r.FormValue("target")
	title := r.FormValue("title")

	if target == "" || title == "" {
		http.Error(w, "Missing fields", http.StatusBadRequest)
		return
	}

	now := time.Now().Format("2006-01-02 15:04:05")

	targetMdPath := filepath.Join(storageDir, "md", target+".md")
	if _, err := os.Stat(targetMdPath); os.IsNotExist(err) {
		authorLine := ""
		if appConfig.Author != "" {
			authorLine = fmt.Sprintf("\nAuthor: %s", appConfig.Author)
		}
		defaultContent := fmt.Sprintf("Title: %s\nDate: %s\nModified: %s\nCategory: Notes%s\n\n# %s\n\nStart editing this page!", title, now, now, authorLine, title)
		os.MkdirAll(filepath.Dir(targetMdPath), 0755)
		os.WriteFile(targetMdPath, []byte(defaultContent), 0644)
	}

	if source != "" {
		sourceMdPath := filepath.Join(storageDir, "md", source+".md")
		sourceData, err := os.ReadFile(sourceMdPath)
		if err == nil {
			content := string(sourceData)
			linkStr := fmt.Sprintf("* [%s](%s)", title, target)
			parts := strings.SplitN(content, "\n\n", 2)

			isHeader := false
			if len(parts) > 0 && strings.Contains(parts[0], ":") {
				firstLine := strings.Split(parts[0], "\n")[0]
				if strings.Contains(firstLine, ":") && !strings.HasPrefix(firstLine, " ") && !strings.HasPrefix(firstLine, "#") {
					isHeader = true
				}
			}

			if isHeader {
				if len(parts) > 1 {
					content = parts[0] + "\n\n" + linkStr + "\n" + parts[1]
				} else {
					content = parts[0] + "\n\n" + linkStr + "\n"
				}
			} else {
				content = linkStr + "\n\n" + content
			}

			content = ensureHeaderModified(content, source)
			os.WriteFile(sourceMdPath, []byte(content), 0644)
			
			// Recompile Source HTML immediately to prevent caching delays
			htmlPath := filepath.Join(storageDir, "html", source+".html")
			compiled := compilePage(source, []byte(content))
			os.MkdirAll(filepath.Dir(htmlPath), 0755)
			os.WriteFile(htmlPath, compiled, 0644)
		}
	}

	w.Write([]byte("Created"))
}

func handleSaveNote(w http.ResponseWriter, r *http.Request) {"""
    apply_patch("backend/server.go", old_func, new_func, "Inject handleNewPage backend logic")

    # 2. Register /api/newpage Router
    old_router = 'mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))'
    new_router = 'mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))\n\t\tmux.HandleFunc("/api/newpage", authMiddleware(handleNewPage, true))'
    apply_patch("backend/server.go", old_router, new_router, "Register /api/newpage API endpoint")

    # 3. Rewrite Frontend JS Workflow using Robust Regex
    print("\n[PATCH] Rewrite Frontend JS createNewPage workflow")
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            js_content = f.read()

        js_pattern = re.compile(r"        function toCamelCase\(str\).*?async function saveNote\(\).*?alert\('Failed to save!'\);\s*\}\s*\}", re.DOTALL)
        new_js = r"""        function toCamelCase(str) {
            let words = str.split(/[-_\s]+/);
            return words.map(w => w ? w.charAt(0).toUpperCase() + w.slice(1) : '').join('');
        }

        async function createNewPage() {
            let title = prompt("Enter New Page Title:");
            if (!title) return;
            let camel = toCamelCase(title);
            let safeName = camel.replace(/[^a-zA-Z0-9-]/g, '-');
            let fileName = prompt("Confirm File Name:", safeName);
            if (!fileName) return;

            let src = typeof currentNote !== 'undefined' ? currentNote : 'Welcome';
            const fd = new URLSearchParams();
            fd.append('source', src);
            fd.append('target', fileName);
            fd.append('title', title);

            const res = await fetch('/api/newpage', { method: 'POST', body: fd });
            if (res.ok) {
                window.location.href = '/' + fileName + '.html?edit=true';
            } else {
                alert("Failed to create new page!");
            }
        }

        async function saveNote() {
            let content = document.getElementById('editor').value;
            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                window.location.reload();
            } else {
                alert('Failed to save!');
            }
        }"""
        
        if js_pattern.search(js_content):
            # FIXED: Using lambda _ : new_js prevents re.sub from attempting to parse '\s' as an escape sequence!
            js_content = js_pattern.sub(lambda _: new_js, js_content)
            with open(js_path, "w", encoding="utf-8") as f:
                f.write(js_content)
            print("  [+] SUCCESS: Frontend API hook updated via Regex safely.")
        else:
            print("  [-] ERROR: Could not find JS block to rewrite via Regex!")
    else:
        print(f"  [-] ERROR: File {js_path} not found!")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "fix(core): migrate backlinking logic and patch python script regex escape error\n\nVersion bumped to 1.3.14"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()