import os
import re
import shutil

def update_application():
    # 0. Migrate Static Assets to clean subdirectories
    frontend_dir = "backend/frontend"
    js_dir = os.path.join(frontend_dir, "js")
    css_dir = os.path.join(frontend_dir, "css")
    
    os.makedirs(js_dir, exist_ok=True)
    os.makedirs(css_dir, exist_ok=True)
    
    # Move JS files safely
    for js_file in ["marked.min.js", "Bookmarker.js"]:
        src = os.path.join(frontend_dir, js_file)
        dst = os.path.join(js_dir, js_file)
        if os.path.exists(src) and not os.path.exists(dst):
            shutil.move(src, dst)
            print(f"[+] Relocated {js_file} to {js_dir}/")
            
    # Move CSS file safely
    css_file = "Bookmarker.css"
    src = os.path.join(frontend_dir, css_file)
    dst = os.path.join(css_dir, css_file)
    if os.path.exists(src) and not os.path.exists(dst):
        shutil.move(src, dst)
        print(f"[+] Relocated {css_file} to {css_dir}/")

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.29"', 'APP_VERSION = "1.0.30"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.29";', 'const APP_VERSION = "1.0.30";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10030', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.30"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                r'''<script src="/marked.min.js"></script>''',
                r'''<script src="/js/marked.min.js"></script>'''
            )
        ],
        "backend/server.go": [
            (
                # Inject io/fs library required for recursive FileServer serving
                r'''import (
	_ "embed"''',
                r'''import (
	"embed"
	"io/fs"'''
            ),
            (
                # Change raw byte arrays to a seamless recursive embed filesystem
                r'''//go:embed frontend/marked.min.js
var markedJS []byte

//go:embed frontend/Bookmarker.js
var bookmarkerJS []byte

//go:embed frontend/Bookmarker.css
var bookmarkerCSS []byte''',
                r'''//go:embed frontend/js frontend/css
var staticFS embed.FS'''
            ),
            (
                # Generate 'md' directory and auto-migrate legacy files on boot
                r'''	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	// 2. Init Config''',
                r'''	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Fatalf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(storageDir, "md")
	os.MkdirAll(mdDir, 0755)

	// Migrate existing .md files recursively
	files, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// 2. Init Config'''
            ),
            (
                # Redirect default notes output to the new 'md' directory
                r'''	// 3. Init Default Notes
	welcomePath := filepath.Join(storageDir, "Welcome.md")''',
                r'''	// 3. Init Default Notes
	welcomePath := filepath.Join(mdDir, "Welcome.md")'''
            ),
            (
                r'''	quickPath := filepath.Join(storageDir, "QuickNotes.md")''',
                r'''	quickPath := filepath.Join(mdDir, "QuickNotes.md")'''
            ),
            (
                r'''	bmPath := filepath.Join(storageDir, "Bookmarks.md")''',
                r'''	bmPath := filepath.Join(mdDir, "Bookmarks.md")'''
            ),
            (
                # Update QuickNotes Handler
                r'''	path := filepath.Join(storageDir, "QuickNotes.md")
	data, _ := os.ReadFile(path)''',
                r'''	path := filepath.Join(storageDir, "md", "QuickNotes.md")
	data, _ := os.ReadFile(path)'''
            ),
            (
                # Update Bookmarks Handler
                r'''	path := filepath.Join(storageDir, "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")''',
                r'''	path := filepath.Join(storageDir, "md", "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")'''
            ),
            (
                # Map standard note fetching to 'md' directory
                r'''	data, err := os.ReadFile(filepath.Join(storageDir, filepath.Clean(name)))
	if err != nil {
		title := strings.TrimSuffix(name, ".md")''',
                r'''	data, err := os.ReadFile(filepath.Join(storageDir, "md", filepath.Clean(name)))
	if err != nil {
		title := strings.TrimSuffix(name, ".md")'''
            ),
            (
                # Ensure multi-level directories are generated automatically during save
                r'''	path := filepath.Join(storageDir, filepath.Clean(name))
	os.WriteFile(path, []byte(content), 0644)
	w.Write([]byte("Saved"))''',
                r'''	path := filepath.Join(storageDir, "md", filepath.Clean(name))
	os.MkdirAll(filepath.Dir(path), 0755)
	os.WriteFile(path, []byte(content), 0644)
	w.Write([]byte("Saved"))'''
            ),
            (
                # Remove deprecated hardcoded handlers
                r'''func serveMarked(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(markedJS)
}

func serveBookmarkerJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript")
	w.Write(bookmarkerJS)
}

func serveBookmarkerCSS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/css")
	w.Write(bookmarkerCSS)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {''',
                r'''func serveFrontend(w http.ResponseWriter, r *http.Request) {'''
            ),
            (
                # Replace legacy mux routes with the recursive FileServer endpoints
                r'''		mux.HandleFunc("/marked.min.js", serveMarked)
		mux.HandleFunc("/js/Bookmarker.js", serveBookmarkerJS)
		mux.HandleFunc("/css/Bookmarker.css", serveBookmarkerCSS)''',
                r'''		fSys, _ := fs.Sub(staticFS, "frontend")
		mux.Handle("/js/", http.FileServer(http.FS(fSys)))
		mux.Handle("/css/", http.FileServer(http.FS(fSys)))'''
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
    commit_msg = """refactor(storage): restructure data directories and static routing

- Migrated all markdown storage to dedicated `md/` subdirectories
- Built backwards-compatible migration routine to auto-move existing `.md` files from legacy root directories on boot
- Relocated JS and CSS assets into frontend `js/` and `css/` subdirectories
- Transitioned static asset serving to Go's `embed.FS` and `http.FileServer` to natively support infinite subdirectory recursion mapping
- Added `os.MkdirAll` into the Save Note API to gracefully handle creation of nested markdown folders
- Bumped Android versionCode to 10030

Version bumped to 1.0.30"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.30!")

if __name__ == "__main__":
    update_application()