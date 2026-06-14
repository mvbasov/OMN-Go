import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.0"', 'APP_VERSION = "1.0.1"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.0";', 'const APP_VERSION = "1.0.1";')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "Dockerfile": [
            (
                "RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init",
                "RUN go install golang.org/x/mobile/cmd/gomobile@v0.0.0-20231127183840-76ac68780225 && gomobile init"
            )
        ],
        "server.go": [
            (
                r'''	bmPath := filepath.Join(storageDir, "Bookmarks.html")
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		bmContent := `<script>bookmarks = [
<!-- Don't edit body below this line -->
];</script>`
		os.WriteFile(bmPath, []byte(bmContent), 0644)
	}''',
                r'''	bmPath := filepath.Join(storageDir, "Bookmarks.md")
	if _, err := os.Stat(bmPath); os.IsNotExist(err) {
		bmContent := "Title: Bookmarks\nDate: 2026-06-14 12:00:00\nCategory: Links\n\n"
		os.WriteFile(bmPath, []byte(bmContent), 0644)
	}'''
            ),
            (
                r'''func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "Bookmarks.html")
	data, _ := os.ReadFile(path)
	
	marker := "<script>bookmarks = [\n<!-- Don't edit body below this line -->"
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	tagsArr := strings.Split(tags, ",")
	tagsJSON, _ := json.Marshal(tagsArr)
	
	newEntry := fmt.Sprintf(`    {
      "date": "%s",
      "url": "%s",
      "title": "%s",
      "tags": %s,
      "notes": ["%s"]
    },`, timestamp, url, title, string(tagsJSON), notes)
	
	replaced := strings.Replace(string(data), marker, marker+"\n"+newEntry, 1)
	os.WriteFile(path, []byte(replaced), 0644)
	w.Write([]byte("Saved"))
}''',
                r'''func handleBookmark(w http.ResponseWriter, r *http.Request) {
	url := r.FormValue("url")
	title := r.FormValue("title")
	tags := r.FormValue("tags")
	notes := r.FormValue("notes")
	
	path := filepath.Join(storageDir, "Bookmarks.md")
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	
	entry := fmt.Sprintf("\n- [%s](%s) | Tags: %s | Notes: %s | Added: %s\n", title, url, tags, notes, timestamp)
	
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(entry)
	}
	w.Write([]byte("Saved"))
}'''
            )
        ],
        "frontend/index.html": [
            (
                '<script src="https://cdn.tailwindcss.com"></script>',
                '''<style>
        body { font-family: sans-serif; margin: 0; padding: 0; display: flex; flex-direction: column; height: 100vh; background: #f9f9f9; color: #333; }
        .overlay { position: fixed; top: 0; left: 0; right: 0; bottom: 0; background: rgba(0,0,0,0.7); display: flex; align-items: center; justify-content: center; z-index: 50; }
        .modal { background: #fff; padding: 20px; border-radius: 4px; box-shadow: 0 4px 6px rgba(0,0,0,0.1); width: 300px; }
        .modal input, .modal button, .modal textarea { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
        .modal button { background: #0056b3; color: white; border: none; cursor: pointer; border-radius: 4px; }
        #mainUI { display: none; flex: 1; flex-direction: column; }
        .header { background: #333; color: #fff; padding: 10px 20px; display: flex; gap: 15px; align-items: center; }
        .header a, .header button { color: #fff; text-decoration: none; cursor: pointer; background: transparent; border: 1px solid #555; padding: 5px 10px; border-radius: 4px; font-size: 14px; }
        .header a:hover, .header button:hover { background: #555; }
        .content-area { flex: 1; padding: 20px; position: relative; display: flex; flex-direction: column; }
        #editor { display: none; width: 100%; flex: 1; border: 1px solid #ccc; padding: 10px; font-family: monospace; resize: none; box-sizing: border-box; }
        #preview { width: 100%; flex: 1; background: #fff; border: 1px solid #ccc; padding: 20px; overflow-y: auto; box-sizing: border-box; line-height: 1.6; }
        .toolbar { display: flex; justify-content: flex-end; margin-bottom: 10px; gap: 10px; }
        .toolbar button { padding: 5px 15px; cursor: pointer; border: 1px solid #ccc; background: #eee; border-radius: 4px; }
        .hidden { display: none !important; }
        .panel { position: absolute; top: 50px; right: 20px; background: white; border: 1px solid #ccc; padding: 15px; box-shadow: 0 4px 8px rgba(0,0,0,0.1); width: 300px; z-index: 40; }
        .panel h3 { margin-top: 0; }
        .panel input, .panel textarea, .panel button { width: 100%; box-sizing: border-box; margin-bottom: 10px; padding: 8px; }
        .panel button { background: #28a745; color: white; border: none; cursor: pointer; border-radius: 4px; }
    </style>'''
            ),
            (
                '<body class="bg-gray-50 text-gray-900 h-screen flex flex-col font-sans">',
                '<body>'
            ),
            (
                '''    <!-- Login Overlay -->
    <div id="loginOverlay" class="fixed inset-0 bg-gray-900 bg-opacity-75 flex items-center justify-center z-50">
        <div class="bg-white p-8 rounded shadow-lg w-96">
            <h2 class="text-2xl font-bold mb-4">GoOMN Login</h2>
            <input type="password" id="pwdInput" class="w-full border p-2 mb-4 rounded" placeholder="Admin or Guest Password">
            <button onclick="login()" class="w-full bg-blue-600 text-white p-2 rounded">Enter</button>
        </div>
    </div>''',
                '''    <!-- Login Overlay -->
    <div id="loginOverlay" class="overlay">
        <div class="modal">
            <h2>GoOMN Login</h2>
            <input type="password" id="pwdInput" placeholder="Admin or Guest Password">
            <button onclick="login()">Enter</button>
        </div>
    </div>'''
            ),
            (
                '''    <!-- Main UI -->
    <div class="flex flex-1 overflow-hidden" id="mainUI" style="display:none;">
        <div class="w-64 bg-gray-800 text-white p-4 flex flex-col">
            <h1 class="text-xl font-bold mb-6">GoOMN <span class="text-xs text-gray-400">v1.0.0</span></h1>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="bg-indigo-600 p-2 rounded mb-2 w-full admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="bg-teal-600 p-2 rounded mb-4 w-full admin-only">Add Bookmark</button>
            <div class="flex-1 overflow-y-auto">
                <p class="text-gray-400 text-sm">Notes list placeholder...</p>
            </div>
        </div>

        <div class="flex-1 p-6 relative">
            <div class="flex h-full gap-4">
                <textarea id="editor" class="w-1/2 h-full border rounded p-4 font-mono admin-only" placeholder="Markdown content... Drag images here to upload."></textarea>
                <div id="preview" class="w-1/2 h-full border rounded p-4 bg-white overflow-y-auto prose max-w-none"></div>
            </div>
        </div>
    </div>''',
                '''    <!-- Main UI -->
    <div id="mainUI">
        <div class="header">
            <strong>GoOMN</strong>
            <a href="#help" onclick="alert('Help page would fetch Welcome.md')">Help</a>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>
            <a href="#bookmarks" onclick="alert('Bookmarks page would fetch Bookmarks.md')">Bookmarks</a>
        </div>

        <div class="content-area">
            <div class="toolbar">
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>
            <textarea id="editor" class="admin-only" placeholder="Markdown content... Drag images here to upload."></textarea>
            <div id="preview">
                <h1>Welcome to GoOMN</h1>
                <p>Select an option from the menu above to get started.</p>
                <ul>
                    <li><strong>Help:</strong> View documentation (Welcome.md)</li>
                    <li><strong>Bookmarks:</strong> View saved links (Bookmarks.md)</li>
                    <li><strong>Quick Note:</strong> Add a timestamped entry to QuickNotes.md</li>
                </ul>
            </div>
        </div>
    </div>'''
            ),
            (
                '''    <!-- Quick Note Modal -->
    <div id="quickPanel" class="hidden absolute top-20 left-1/3 w-1/3 bg-white border shadow-xl p-4 rounded z-40">
        <h3 class="font-bold mb-2">Quick Note</h3>
        <textarea id="quickText" class="w-full h-32 border p-2 mb-2"></textarea>
        <button onclick="submitQuickNote()" class="bg-blue-600 text-white px-4 py-2 rounded">Append to QuickNotes.md</button>
    </div>

    <!-- Bookmark Modal -->
    <div id="bmPanel" class="hidden absolute top-20 right-10 w-1/3 bg-white border shadow-xl p-4 rounded z-40">
        <h3 class="font-bold mb-2">Ingest Bookmark</h3>
        <input id="bmUrl" class="w-full border p-2 mb-2" placeholder="URL">
        <input id="bmTitle" class="w-full border p-2 mb-2" placeholder="Title">
        <input id="bmTags" class="w-full border p-2 mb-2" placeholder="Tags (comma separated)">
        <textarea id="bmNotes" class="w-full border p-2 mb-2 h-16" placeholder="Notes"></textarea>
        <button onclick="submitBookmark()" class="bg-blue-600 text-white px-4 py-2 rounded">Inject to Bookmarks.html</button>
    </div>''',
                '''    <!-- Quick Note Modal -->
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
    </div>'''
            ),
            (
                '''        // Init preview updates
        document.getElementById('editor').addEventListener('input', (e) => {
            document.getElementById('preview').innerHTML = marked.parse(e.target.value);
        });''',
                '''        let currentMode = 'view';
        function toggleMode() {
            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');
            
            if(currentMode === 'view') {
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerText = 'View Mode';
                currentMode = 'edit';
            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                preview.innerHTML = marked.parse(editor.value || '*(Empty content)*');
                btn.innerText = 'Edit Mode';
                currentMode = 'view';
            }
        }'''
            ),
            (
                '''        function checkRole() {
            if(document.cookie.includes('session_role=guest')) {
                document.querySelectorAll('.admin-only').forEach(el => el.disabled = true);
                editor.readOnly = true;
                editor.placeholder = "Read-only mode (Guest)";
            }
        }''',
                '''        function checkRole() {
            if(document.cookie.includes('session_role=guest')) {
                document.querySelectorAll('.admin-only').forEach(el => {
                    if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                    if(el.id === 'toggleBtn' || el.id === 'editor') el.style.display = 'none';
                });
            }
        }'''
            ),
            (
                '''            if(res.ok) {
                document.getElementById('quickText').value = '';
                document.getElementById('quickPanel').classList.add('hidden');
            }''',
                '''            if(res.ok) {
                document.getElementById('quickText').value = '';
                document.getElementById('quickPanel').classList.add('hidden');
                alert('Saved!');
            }'''
            ),
            (
                '''            if(res.ok) {
                document.getElementById('bmPanel').classList.add('hidden');
                document.querySelectorAll('#bmPanel input, #bmPanel textarea').forEach(el => el.value = '');
            }''',
                '''            if(res.ok) {
                document.getElementById('bmPanel').classList.add('hidden');
                document.querySelectorAll('#bmPanel input, #bmPanel textarea').forEach(el => el.value = '');
                alert('Saved!');
            }'''
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
                # If the script previously crashed halfway, this handles already applied patches gracefully
                print(f"[{filename}] Patch target #{idx} is already applied. Skipping.")
            else:
                raise ValueError(f"Could not find patch target #{idx} in {filename}:\n{old_str[:100]}...")
                
        with open(filename, 'w', encoding='utf-8') as f:
            f.write(content)
            
        print(f"Patched: {filename}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """feat(core): switch bookmarks to markdown, remove tailwind, implement mode toggling, and fix gomobile build constraints

Version bumped to 1.0.1"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()