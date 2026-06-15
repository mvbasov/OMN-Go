import os
import re

def update_application():
    # 0. Initialize JSON Directory
    frontend_dir = "backend/frontend"
    json_dir = os.path.join(frontend_dir, "json")
    os.makedirs(json_dir, exist_ok=True)
    
    # go:embed requires the directory to not be empty
    stub_path = os.path.join(json_dir, "stub.json")
    if not os.path.exists(stub_path):
        with open(stub_path, 'w', encoding='utf-8') as f:
            f.write('{\n  "status": "initialized"\n}')
            print(f"[+] Created {stub_path} to satisfy go:embed constraints.")

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.30"', 'APP_VERSION = "1.0.31"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.30";', 'const APP_VERSION = "1.0.31";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10031', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.31"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/server.go": [
            (
                # Add frontend/json to the embed virtual filesystem
                r'''//go:embed frontend/js frontend/css
var staticFS embed.FS''',
                r'''//go:embed frontend/js frontend/css frontend/json
var staticFS embed.FS'''
            ),
            (
                # Replace generic FileServer with a strict validation wrapper
                r'''		fSys, _ := fs.Sub(staticFS, "frontend")
		mux.Handle("/js/", http.FileServer(http.FS(fSys)))
		mux.Handle("/css/", http.FileServer(http.FS(fSys)))''',
                r'''		fSys, _ := fs.Sub(staticFS, "frontend")
		
		serveStrict := func(ext, cType string) http.Handler {
			fsHandler := http.FileServer(http.FS(fSys))
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if !strings.HasSuffix(r.URL.Path, ext) {
					http.Error(w, "Forbidden: Invalid file extension", http.StatusForbidden)
					return
				}
				w.Header().Set("Content-Type", cType)
				fsHandler.ServeHTTP(w, r)
			})
		}

		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))
		mux.Handle("/json/", serveStrict(".json", "application/json"))'''
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
    commit_msg = """feat(static): enforce strict content-type and extension validation

- Added strict extension checking to static asset routes to block unauthorized file reads
- Hardcoded explicit Content-Type headers for `js/`, `css/`, and new `json/` directories
- Scaffolding initialized for `json/` static payload storage
- Bumped Android versionCode to 10031

Version bumped to 1.0.31"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.31!")

if __name__ == "__main__":
    update_application()