import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.1"', 'APP_VERSION = "1.0.2"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.1";', 'const APP_VERSION = "1.0.2";')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "Dockerfile": [
            (
                "RUN go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20231127183840-76ac68780225 && gomobile init",
                "RUN git clone https://github.com/golang/mobile.git /tmp/mobile && cd /tmp/mobile && git checkout 76ac68780225 && cd cmd/gomobile && go install . && gomobile init"
            )
        ],
        "server.go": [
            (
                r'''		welcomeContent := "Title: Welcome\nDate: 2026-06-14 12:00:00\nCategory: System\n\nWelcome to GoOMN. Start editing!"''',
                r'''		welcomeContent := "Title: Welcome\nDate: 2026-06-14 12:00:00\nCategory: System\n\nWelcome to GoOMN. Start editing!\n\n- [Help](Welcome)\n- [Bookmarks](Bookmarks)\n- [Quick Notes](QuickNotes)"'''
            ),
            (
                r'''	mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))''',
                r'''	mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
	mux.HandleFunc("/api/note", handleGetNote)'''
            ),
            (
                r'''func serveFrontend(w http.ResponseWriter, r *http.Request) {''',
                r'''func handleGetNote(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		name = "Welcome"
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	data, err := os.ReadFile(filepath.Join(storageDir, filepath.Clean(name)))
	if err != nil {
		w.Write([]byte("*(File not found)*"))
		return
	}
	w.Write(data)
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {'''
            )
        ],
        "frontend/index.html": [
            (
                r'''            <div id="preview">
                <h1>Welcome to GoOMN</h1>
                <p>Select an option from the menu above to get started.</p>
                <ul>
                    <li><strong>Help:</strong> View documentation (Welcome.md)</li>
                    <li><strong>Bookmarks:</strong> View saved links (Bookmarks.md)</li>
                    <li><strong>Quick Note:</strong> Add a timestamped entry to QuickNotes.md</li>
                </ul>
            </div>''',
                r'''            <div id="preview">Loading...</div>'''
            ),
            (
                r'''            <a href="#help" onclick="alert('Help page would fetch Welcome.md')">Help</a>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>
            <a href="#bookmarks" onclick="alert('Bookmarks page would fetch Bookmarks.md')">Bookmarks</a>''',
                r'''            <a href="#help" onclick="loadNote('Welcome')">Help</a>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>
            <a href="#bookmarks" onclick="loadNote('Bookmarks')">Bookmarks</a>'''
            ),
            (
                r'''        let currentMode = 'view';
        function toggleMode() {''',
                r'''        let currentNote = 'Welcome';
        
        async function loadNote(name) {
            currentNote = name;
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));
            const text = await res.text();
            document.getElementById('editor').value = text;
            document.getElementById('preview').innerHTML = marked.parse(text);
        }

        window.onload = () => loadNote('Welcome');

        // Intercept Markdown links to load internally
        document.getElementById('preview').addEventListener('click', (e) => {
            if(e.target.tagName === 'A') {
                const href = e.target.getAttribute('href');
                if(href && !href.startsWith('http')) {
                    e.preventDefault();
                    loadNote(href);
                }
            }
        });

        let currentMode = 'view';
        function toggleMode() {'''
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
    commit_msg = """feat(core): switch gomobile to git clone in dockerfile, implement internal markdown file routing

Version bumped to 1.0.2"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()