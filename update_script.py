import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.5"', 'APP_VERSION = "1.0.6"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.5";', 'const APP_VERSION = "1.0.6";')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "server.go": [
            (
                r'''import (
	"encoding/json"
	"fmt"''',
                r'''import (
	"embed"
	"encoding/json"
	"fmt"'''
            ),
            (
                r'''var (
	storageDir  string''',
                r'''//go:embed frontend/index.html
var frontendHTML []byte

var (
	storageDir  string'''
            ),
            (
                r'''func serveFrontend(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "frontend/index.html")
}''',
                r'''func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(frontendHTML)
}'''
            )
        ],
        "frontend/index.html": [
            (
                r'''        <div class="header">
            <strong>GoOMN</strong>
            <a href="#help" onclick="loadNote('Welcome')">Help</a>''',
                r'''        <div class="header">
            <strong>GoOMN</strong>
            <a href="#home" onclick="loadNote('Welcome')">Home</a>
            <a href="#help" onclick="loadNote('Welcome')">Help</a>'''
            ),
            (
                r'''    <!-- Quick Note Modal -->
    <div id="quickPanel" class="panel hidden">
        <h3>Quick Note</h3>
        <textarea id="quickText" rows="4"></textarea>
        <button onclick="submitQuickNote()">Append to QuickNotes.md</button>
    </div>

    <!-- Bookmark Modal -->
    <div id="bmPanel" class="panel hidden">
        <h3>Ingest Bookmark</h3>
        <input id="bmUrl" placeholder="URL">
        <input id="bmTitle" placeholder="Title">
        <input id="bmTags" placeholder="Tags (comma separated)">
        <textarea id="bmNotes" rows="2" placeholder="Notes"></textarea>
        <button onclick="submitBookmark()">Inject to Bookmarks.md</button>
    </div>''',
                r'''    <!-- Quick Note Modal -->
    <div id="quickPanel" class="panel hidden">
        <h3>Quick Note</h3>
        <textarea id="quickText" rows="4"></textarea>
        <div style="display: flex; gap: 10px;">
            <button onclick="submitQuickNote()">Save</button>
            <button onclick="document.getElementById('quickPanel').classList.add('hidden')" style="background: #dc3545;">Cancel</button>
        </div>
    </div>

    <!-- Bookmark Modal -->
    <div id="bmPanel" class="panel hidden">
        <h3>Ingest Bookmark</h3>
        <input id="bmUrl" placeholder="URL">
        <input id="bmTitle" placeholder="Title">
        <input id="bmTags" placeholder="Tags (comma separated)">
        <textarea id="bmNotes" rows="2" placeholder="Notes"></textarea>
        <div style="display: flex; gap: 10px;">
            <button onclick="submitBookmark()">Save</button>
            <button onclick="document.getElementById('bmPanel').classList.add('hidden')" style="background: #dc3545;">Cancel</button>
        </div>
    </div>'''
            )
        ],
        "Dockerfile": [
            (
                'sdkmanager "platforms;android-33"',
                'sdkmanager "platforms;android-34"'
            ),
            (
                '-androidapi 21 -o bin/goomn.apk .',
                '-androidapi 34 -o bin/goomn.apk .'
            )
        ]
    }

    # Execute version updates safely (idempotent)
    for filename, old_str, new_str in version_replacements:
        if os.path.exists(filename):
            with open(filename, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_str in content:
                content = content.replace(old_str, new_str)
                with open(filename, 'w', encoding='utf-8') as f:
                    f.write(content)
                    
    # Execute patching sequentially
    for filename, file_patches in patches.items():
        if not os.path.exists(filename):
            print(f"Skipping {filename}: File not found.")
            continue
            
        with open(filename, 'r', encoding='utf-8') as f:
            content = f.read()
        
        for idx, (old_str, new_str) in enumerate(file_patches):
            if old_str in content:
                content = content.replace(old_str, new_str)
            elif new_str in content:
                print(f"[{filename}] Patch target #{idx} is already applied. Skipping.")
            else:
                raise ValueError(f"Could not find patch target #{idx} in {filename}:\n{old_str[:100]}...")
                
        with open(filename, 'w', encoding='utf-8') as f:
            f.write(content)
            
        print(f"Patched: {filename}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """feat(ui): add home button, cancel buttons, embed frontend HTML, bump android api to 34

Version bumped to 1.0.6"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()