import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.34"', 'APP_VERSION = "1.0.35"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.34";', 'const APP_VERSION = "1.0.35";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10035', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.35"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                # Part 1: Re-enable JS injection
                r'''        function executeScripts(container) {
            return; // Experiment: Temporarily disabled script evaluation
            const scripts = container.querySelectorAll('script');''',
                r'''        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');'''
            ),
            (
                # Part 2: Introduce safe path resolving and force View Mode on navigation
                r'''        async function loadNoteInternal(name, hash) {
            currentNote = name;
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));''',
                r'''        function resolvePath(current, target) {
            if (target.startsWith('/')) {
                target = target.substring(1);
            } else {
                let parts = current.split('/');
                parts.pop();
                target = (parts.length > 0 ? parts.join('/') + '/' : '') + target;
            }
            let segments = target.split('/');
            let result = [];
            for (let seg of segments) {
                if (seg === '.' || seg === '') continue;
                if (seg === '..') {
                    if (result.length > 0) result.pop();
                } else {
                    result.push(seg);
                }
            }
            return result.join('/');
        }

        async function loadNoteInternal(name, hash) {
            currentNote = name;
            if (currentMode === 'edit') {
                document.getElementById('editor').style.display = 'none';
                document.getElementById('preview').style.display = 'block';
                document.getElementById('toggleBtn').innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'view';
            }
            const res = await fetch('/api/note?name=' + encodeURIComponent(name));'''
            ),
            (
                # Part 3: Map link interceptor logic to the new resolvePath abstraction
                r'''                    if (!file && hash) {
                        let el = document.getElementById(hash);
                        if(el) el.scrollIntoView();
                        return;
                    }
                    if (!file) file = currentNote;
                    loadNote(file, hash);''',
                r'''                    if (!file && hash) {
                        let el = document.getElementById(hash);
                        if(el) el.scrollIntoView();
                        return;
                    }
                    if (!file) {
                        file = currentNote;
                    } else {
                        file = resolvePath(currentNote, file);
                    }
                    loadNote(file, hash);'''
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
    commit_msg = """feat(navigation): add relative link resolution and restore JS injection

- Restored JS execution block inside innerHTML note loading to ensure embedded scripts like Bookmarker.js function normally.
- Implemented frontend path resolution logic to safely handle `../` relative links, bounding traversals so they cannot escape the root directory.
- Differentiated root links (`/filename.md`) from local links (`filename.md`) during path evaluation.
- Enforced automatic UI fallback to Preview Mode whenever a navigation event or hardware back-button press occurs while currently inside Edit Mode.
- Bumped Android versionCode to 10035

Version bumped to 1.0.35"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.35!")

if __name__ == "__main__":
    update_application()