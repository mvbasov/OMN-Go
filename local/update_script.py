import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.24"', 'APP_VERSION = "1.0.25"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.24";', 'const APP_VERSION = "1.0.25";')
    ]
    
    # 2. Define File Patches (Target exact string mapping using raw strings to prevent escaping issues)
    patches = {
        "backend/server.go": [
            (
                # Generate Pelican header for new files dynamically
                r'''	data, err := os.ReadFile(filepath.Join(storageDir, filepath.Clean(name)))
	if err != nil {
		w.Write([]byte("*(File not found)*"))
		return
	}
	w.Write(data)''',
                r'''	data, err := os.ReadFile(filepath.Join(storageDir, filepath.Clean(name)))
	if err != nil {
		title := strings.TrimSuffix(name, ".md")
		timestamp := time.Now().Format("2006-01-02 15:04:05")
		newContent := fmt.Sprintf("Title: %s\nDate: %s\nCategory: Notes\n\n", title, timestamp)
		w.Write([]byte(newContent))
		return
	}
	w.Write(data)'''
            ),
            (
                # Create the Save Note API Endpoint
                r'''func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(frontendHTML)
}''',
                r'''func handleSaveNote(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	content := r.FormValue("content")
	if name == "" {
		return
	}
	if !strings.HasSuffix(name, ".md") {
		name += ".md"
	}
	path := filepath.Join(storageDir, filepath.Clean(name))
	os.WriteFile(path, []byte(content), 0644)
	w.Write([]byte("Saved"))
}

func serveFrontend(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write(frontendHTML)
}'''
            ),
            (
                # Map the save endpoint to the mux router
                r'''		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/note", handleGetNote)''',
                r'''		mux.HandleFunc("/api/upload", authMiddleware(handleUpload, true))
		mux.HandleFunc("/api/note", handleGetNote)
		mux.HandleFunc("/api/save", authMiddleware(handleSaveNote, true))'''
            )
        ],
        "backend/frontend/index.html": [
            (
                # Add Save Button to Toolbar
                r'''            <div class="toolbar">
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>''',
                r'''            <div class="toolbar">
                <button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;">Save Note</button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>'''
            ),
            (
                # Add toggling logic for the Save button
                r'''            if(currentMode === 'view') {
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
            }''',
                r'''            if(currentMode === 'view') {
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerText = 'View Mode';
                document.getElementById('saveBtn').style.display = 'block';
                currentMode = 'edit';
            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                preview.innerHTML = marked.parse(editor.value || '*(Empty content)*');
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }'''
            ),
            (
                # Guard the save button from unauthorized Guest users
                r'''                    if(el.id === 'toggleBtn' || el.id === 'editor') el.style.display = 'none';''',
                r'''                    if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';'''
            ),
            (
                # Inject JavaScript Save Logic
                r'''        async function submitQuickNote() {''',
                r'''        async function saveNote() {
            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', document.getElementById('editor').value);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                toggleMode();
            } else {
                alert('Failed to save!');
            }
        }

        async function submitQuickNote() {'''
            )
        ]
    }

    # Execute Version Bump
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                content = content.replace(old, new)
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content)

    # Execute Patches Sequentially
    for filepath, file_patches in patches.items():
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            
            for old, new in file_patches:
                if old in content:
                    content = content.replace(old, new)
                elif new not in content:
                    raise ValueError(f"Could not find patch target in {filepath}:\n{old}")
            
            with open(filepath, 'w', encoding='utf-8') as f:
                f.write(content)
        else:
            print(f"Warning: {filepath} not found.")

    # 3. Output Standardized Git Commit Message
    commit_msg = """feat(editor): implement save logic and auto-generate pelican headers

- Added /api/save endpoint to persist markdown note edits
- Injected explicit "Save Note" action into frontend toggle toolbar
- Updated handleGetNote to auto-generate timestamped Pelican templates for missing files
- Hidden save mechanics from Guest role context

Version bumped to 1.0.25"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.25!")

if __name__ == "__main__":
    update_application()