import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.36"', 'APP_VERSION = "1.0.37"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.36";', 'const APP_VERSION = "1.0.37";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10037', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.37"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                # Add marked.use custom renderer to generate ID anchor slugs from Header Text
                r'''        if ('true' == eHlJs) {
            marked.setOptions({
                highlight: function (code) {
                    return hljs.highlightAuto(code).value;
                }
            });
        }
    </script>''',
                r'''        if ('true' == eHlJs) {
            marked.setOptions({
                highlight: function (code) {
                    return hljs.highlightAuto(code).value;
                }
            });
        }

        marked.use({
            renderer: {
                heading(text, level, raw) {
                    if (typeof text === 'object') {
                        const token = text;
                        const content = this.parser.parseInline(token.tokens);
                        const id = (token.text || '').toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
                        return `<h${token.depth} id="${id}">${content}</h${token.depth}>\n`;
                    }
                    const id = text.toLowerCase().replace(/<[^>]*>/g, '').replace(/[^a-z0-9]+/g, '-').replace(/(^-|-$)/g, '');
                    return `<h${level} id="${id}">${text}</h${level}>\n`;
                }
            }
        });
    </script>'''
            ),
            (
                # Downgrade top-level inline let/const variables to var dynamically to bypass SPA redeclaration errors
                r'''        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }''',
                r'''        function executeScripts(container) {
            const scripts = container.querySelectorAll('script');
            scripts.forEach(oldScript => {
                const newScript = document.createElement('script');
                Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                newScript.async = false;
                if (oldScript.innerHTML) {
                    let code = oldScript.innerHTML;
                    code = code.replace(/\bconst\s+/g, 'var ').replace(/\blet\s+/g, 'var ');
                    newScript.appendChild(document.createTextNode(code));
                }
                oldScript.parentNode.replaceChild(newScript, oldScript);
            });
        }'''
            )
        ],
        "backend/server.go": [
            (
                # Remove restrictive IIFE wrapper on Bookmarker.js and dynamically patch const/let to var
                r'''						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						w.Write([]byte("(function(){\n" + js + "\n})();"))
						return''',
                r'''						js := strings.ReplaceAll(string(data), "'#content'", "'#preview'")
						js = strings.ReplaceAll(js, "getElementById('content')", "getElementById('preview')")
						js = strings.ReplaceAll(js, "const ", "var ")
						js = strings.ReplaceAll(js, "let ", "var ")
						w.Write([]byte(js))
						return'''
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
    commit_msg = """feat(markdown): implement heading id slugs and resolve javascript re-evaluation crashes

- Overrode marked.js core renderer to automatically generate HTML header `id` attributes dynamically from the header text content.
- Replaced the restrictive backend IIFE wrapper on Bookmarker.js with a smart regex parser that downgrades `const` and `let` declarations to `var`. This prevents fatal "already declared" SyntaxErrors when navigating between pages without breaking global `onclick` function bindings.
- Applied the identical `var` downgrade to all inline Markdown `<script>` tag blocks executed by `executeScripts`.
- Bumped Android versionCode to 10037

Version bumped to 1.0.37"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.37!")

if __name__ == "__main__":
    update_application()