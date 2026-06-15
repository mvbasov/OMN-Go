import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.32"', 'APP_VERSION = "1.0.33"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.32";', 'const APP_VERSION = "1.0.33";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10033', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.33"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                # Inject Metadata Toggle Button and Expandable Panel into the Toolbar
                r'''            <div class="toolbar">
                <button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;">Save Note</button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>''',
                r'''            <div class="toolbar">
                <button id="metaToggleBtn" onclick="document.getElementById('metadataPanel').classList.toggle('hidden')" style="display: none; background: #17a2b8; color: white; border: none;">Metadata</button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;">Save Note</button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only">Edit Mode</button>
            </div>
            <div id="metadataPanel" class="hidden" style="background: #e9ecef; padding: 15px; font-family: monospace; white-space: pre-wrap; border: 1px solid #ccc; margin-bottom: 10px; border-radius: 4px; font-size: 13px;"></div>'''
            ),
            (
                # Inject dynamic Pelican header extraction logic into the loader
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
        }'''
            ),
            (
                # Ensure the toggler uses the new extraction logic when reverting from Edit to View Mode
                r'''        let currentMode = 'view';
        function toggleMode() {
            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');
            
            if(currentMode === 'view') {
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerText = 'View Mode';
                document.getElementById('saveBtn').style.display = 'block';
                currentMode = 'edit';
            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                preview.innerHTML = marked.parse(editor.value || '*(Empty content)*');
                executeScripts(preview);
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }
        }''',
                r'''        let currentMode = 'view';
        function toggleMode() {
            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');
            
            if(currentMode === 'view') {
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerText = 'View Mode';
                document.getElementById('saveBtn').style.display = 'block';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'edit';
            } else {
                editor.style.display = 'none';
                preview.style.display = 'block';
                renderView(editor.value || '*(Empty content)*');
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
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
    commit_msg = """feat(ui): decouple pelican metadata rendering from main markdown view

- Created `renderView` abstraction to dynamically strip Pelican metadata block from raw markdown during View Mode.
- Excluded metadata from `marked.js` parsing to clean up output visuals.
- Injected an interactive, hidden `metadataPanel` and toggle button into the toolbar to inspect extracted metadata headers.
- Refactored `toggleMode` so original metadata blocks remain permanently anchored and fully editable inside the raw textarea context.
- Bumped Android versionCode to 10033

Version bumped to 1.0.33"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.33!")

if __name__ == "__main__":
    update_application()