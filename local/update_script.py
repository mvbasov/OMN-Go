import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.33"', 'APP_VERSION = "1.0.34"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.33";', 'const APP_VERSION = "1.0.34";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10034', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.34"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                # Part 1: Early Return JS Reinject, Format Metadata Panel, and Support History PushState
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

        function renderView(text) {
            let header = '';
            let body = text;
            const parts = text.split(/(?:\r?\n){2,}/);
            if (parts.length > 0 && /^[A-Za-z0-9_-]+:/.test(parts[0])) {
                header = parts[0];
                body = text.substring(header.length).replace(/^\s+/, '');
            }
            
            const metaPanel = document.getElementById('metadataPanel');
            const metaBtn = document.getElementById('metaToggleBtn');
            if (header) {
                metaPanel.innerText = header;
                metaBtn.style.display = 'block';
            } else {
                metaPanel.innerText = '';
                metaBtn.style.display = 'none';
                metaPanel.classList.add('hidden');
            }
            
            const preview = document.getElementById('preview');
            preview.innerHTML = marked.parse(body || '*(Empty content)*');
            executeScripts(preview);
        }

        async function loadNote(name) {
            currentNote = name;
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));
            const text = await res.text();
            document.getElementById('editor').value = text;
            renderView(text);
        }''',
                r'''        function executeScripts(container) {
            return; // Experiment: Temporarily disabled script evaluation
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }

        function renderView(text) {
            let header = '';
            let body = text;
            const parts = text.split(/(?:\r?\n){2,}/);
            if (parts.length > 0 && /^[A-Za-z0-9_-]+:/.test(parts[0])) {
                header = parts[0];
                body = text.substring(header.length).replace(/^\s+/, '');
            }
            
            const metaPanel = document.getElementById('metadataPanel');
            const metaBtn = document.getElementById('metaToggleBtn');
            
            metaPanel.innerText = `[File: ${currentNote}]\n\n` + (header || 'No Metadata Found');
            metaBtn.style.display = 'block';
            if (!header) metaPanel.classList.add('hidden');
            
            const preview = document.getElementById('preview');
            preview.innerHTML = marked.parse(body || '*(Empty content)*');
            executeScripts(preview);
        }

        window.addEventListener('popstate', (e) => {
            if (e.state && e.state.note) {
                let fullHash = window.location.hash.substring(1);
                let scrollHash = fullHash.includes('#') ? fullHash.split('#')[1] : null;
                if (currentNote !== e.state.note) {
                    loadNoteInternal(e.state.note, scrollHash);
                } else if (scrollHash) {
                    let el = document.getElementById(scrollHash);
                    if (el) el.scrollIntoView();
                }
            }
        });

        async function loadNote(name, hash) {
            history.pushState({note: name}, "", "#" + name + (hash ? "#" + hash : ""));
            if (currentNote !== name) {
                await loadNoteInternal(name, hash);
            } else if (hash) {
                let el = document.getElementById(hash);
                if (el) el.scrollIntoView();
            }
        }

        async function loadNoteInternal(name, hash) {
            currentNote = name;
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));
            const text = await res.text();
            document.getElementById('editor').value = text;
            renderView(text);
            if (hash) {
                setTimeout(() => {
                    let el = document.getElementById(hash);
                    if (el) el.scrollIntoView();
                }, 100);
            }
        }'''
            ),
            (
                # Part 2: Fragment Safe Link Loading and Initial Boot state integration
                r'''        window.onload = () => loadNote('Welcome');

        // Intercept Markdown links to load internally
        document.getElementById('preview').addEventListener('click', (e) => {
            if(e.target.tagName === 'A') {
                const href = e.target.getAttribute('href');
                if(href && !href.startsWith('http')) {
                    e.preventDefault();
                    loadNote(href);
                } else if (href && href.startsWith('http')) {
                    e.preventDefault();
                    window.open(href, '_blank');
                }
            }
        });''',
                r'''        window.onload = () => {
            let fullHash = window.location.hash.substring(1);
            let startNote = fullHash.split('#')[0] || 'Welcome';
            let scrollHash = fullHash.includes('#') ? fullHash.split('#')[1] : null;
            history.replaceState({note: startNote}, "", "#" + (fullHash || 'Welcome'));
            loadNoteInternal(startNote, scrollHash);
        };

        // Intercept Markdown links to load internally
        document.getElementById('preview').addEventListener('click', (e) => {
            let target = e.target.closest('a');
            if(target) {
                const href = target.getAttribute('href');
                if(href && !href.startsWith('http') && !href.startsWith('javascript:')) {
                    e.preventDefault();
                    let pathAndQuery = href.split('#')[0];
                    let file = pathAndQuery.split('?')[0]; 
                    let hash = href.includes('#') ? href.substring(href.indexOf('#') + 1) : null;
                    if (!file && hash) {
                        let el = document.getElementById(hash);
                        if(el) el.scrollIntoView();
                        return;
                    }
                    if (!file) file = currentNote;
                    loadNote(file, hash);
                } else if (href && href.startsWith('http')) {
                    e.preventDefault();
                    window.open(href, '_blank');
                }
            }
        });'''
            ),
            (
                # Part 3: Inject `Modified:` timestamp dynamically into save mechanic
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
        }''',
                r'''        async function saveNote() {
            let content = document.getElementById('editor').value;
            const parts = content.split(/(?:\r?\n){2,}/);
            if (parts.length > 0 && /^[A-Za-z0-9_-]+:/.test(parts[0])) {
                let headerLines = parts[0].split(/\r?\n/);
                let modIdx = headerLines.findIndex(l => l.startsWith('Modified:'));
                let now = new Date().toISOString().replace('T', ' ').substring(0, 19);
                if (modIdx !== -1) {
                    headerLines[modIdx] = `Modified: ${now}`;
                } else {
                    headerLines.push(`Modified: ${now}`);
                }
                parts[0] = headerLines.join('\n');
                content = parts.join('\n\n');
                document.getElementById('editor').value = content;
            }

            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                toggleMode();
            } else {
                alert('Failed to save!');
            }
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
    commit_msg = """feat(ui): implement back stack routing, link fragments, and auto-modified headers

- Added history.pushState and popstate event listeners to perfectly emulate native page navigation, correctly routing Android hardware back-button flows instead of abruptly closing the app.
- Updated link interception logic to defensively parse and separate paths, `?` queries, and `#` fragment anchors safely.
- Embedded an auto-patcher inside `saveNote` to read the Pelican block from the active textarea and dynamically inject or update the `Modified: YYYY-MM-DD HH:MM:SS` parameter prior to server transmission.
- Updated Metadata View renderer to permanently display the current `[File: name.md]` as requested.
- Early returned `executeScripts` logic for experimental decoupling purposes.
- Bumped Android versionCode to 10034

Version bumped to 1.0.34"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.34!")

if __name__ == "__main__":
    update_application()