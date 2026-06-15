import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.31"', 'APP_VERSION = "1.0.32"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.31";', 'const APP_VERSION = "1.0.32";')
    ]
    
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old, new))

    # 2. Bump the Android Version in Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            gradle_content = f.read()
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10032', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.32"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/server.go": [
            (
                # Fix Markdown insertion to be resilient against \n vs \r\n line endings
                r'''		marker := "<!-- Don't edit body below this line -->\n"
		idx := strings.Index(content, marker)
		if idx != -1 {
			insertPos := idx + len(marker)
			newContent := content[:insertPos] + entry + content[insertPos:]
			os.WriteFile(path, []byte(newContent), 0644)
		} else {''',
                r'''		marker := "<!-- Don't edit body below this line -->"
		if strings.Contains(content, marker) {
			newContent := strings.Replace(content, marker, marker+"\n"+entry, 1)
			os.WriteFile(path, []byte(newContent), 0644)
		} else {'''
            ),
            (
                # Intercept Bookmarker.js on the fly: Wrap in IIFE and retarget DOM IDs
                r'''		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				fsHandler.ServeHTTP(w, r)
			})
		}''',
                r'''		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				
				if r.URL.Path == "/js/Bookmarker.js" {
					data, err := fs.ReadFile(fSys, "js/Bookmarker.js")
					if err == nil {
						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						w.Write([]byte("(function(){\n" + js + "\n})();"))
						return
					}
				}
				
				fsHandler.ServeHTTP(w, r)
			})
		}'''
            )
        ],
        "backend/frontend/index.html": [
            (
                # Inject executeScripts parser into the Note Loader to forcibly run innerHTML scripts
                r'''        async function loadNote(name) {
            currentNote = name;
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));
            const text = await res.text();
            document.getElementById('editor').value = text;
            document.getElementById('preview').innerHTML = marked.parse(text);
        }''',
                r'''        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }

        async function loadNote(name) {
            currentNote = name;
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));
            const text = await res.text();
            document.getElementById('editor').value = text;
            document.getElementById('preview').innerHTML = marked.parse(text);
            executeScripts(document.getElementById('preview'));
        }'''
            ),
            (
                # Ensure the script parser also fires when toggling out of Edit Mode
                r'''            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                preview.innerHTML = marked.parse(editor.value || '*(Empty content)*');
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }''',
                r'''            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                preview.innerHTML = marked.parse(editor.value || '*(Empty content)*');
                executeScripts(preview);
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }'''
            )
        ]
    }

    # Execute Patches Sequentially
    for filepath_target, file_patches in patches.items():
        if os.path.exists(filepath_target):
            with open(filepath_target, 'r', encoding='utf-8') as f:
                content = f.read()
            for old, new in file_patches:
                if old in content:
                    content = content.replace(old, new)
                elif new not in content:
                    print(f"Warning: Could not find patch target in {filepath_target}:\n{old[:50]}...")
            with open(filepath_target, 'w', encoding='utf-8') as f:
                f.write(content)

    # 4. Output Standardized Git Commit Message
    commit_msg = """fix(bookmarks): render internal scripts and resolve bookmark insertion bugs

- Replaced rigid newline matching in `handleBookmark` with a resilient string replacement to ensure new bookmarks append correctly regardless of OS line endings.
- Implemented an `executeScripts` DOM hook in `index.html` to safely evaluate `script` tags embedded inside Markdown files after rendering them via `marked.js`
- Intercepted `/js/Bookmarker.js` in `serveStrict` to dynamically wrap the payload in an IIFE to prevent `SyntaxError` redeclarations on repeat visits, and dynamically pointed its DOM hooks to `#preview` instead of `#content`.
- Bumped Android versionCode to 10032

Version bumped to 1.0.32"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.32!")

if __name__ == "__main__":
    update_application()