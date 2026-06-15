import os
import re
import urllib.request

def update_application():
    # 0. Download highlight.js and css locally for offline mode
    frontend_js_dir = "backend/frontend/js"
    frontend_css_dir = "backend/frontend/css"
    os.makedirs(frontend_js_dir, exist_ok=True)
    os.makedirs(frontend_css_dir, exist_ok=True)
    
    hljs_js_path = os.path.join(frontend_js_dir, "highlight.min.js")
    if not os.path.exists(hljs_js_path):
        print("Downloading highlight.min.js for offline syntax highlighting...")
        try:
            urllib.request.urlretrieve("https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/highlight.min.js", hljs_js_path)
            print("[+] Successfully downloaded highlight.min.js")
        except Exception as e:
            print(f"Failed to download highlight.min.js: {e}")

    hljs_css_path = os.path.join(frontend_css_dir, "highlight.default.min.css")
    if not os.path.exists(hljs_css_path):
        print("Downloading highlight.default.min.css...")
        try:
            urllib.request.urlretrieve("https://cdnjs.cloudflare.com/ajax/libs/highlight.js/11.9.0/styles/default.min.css", hljs_css_path)
            print("[+] Successfully downloaded highlight.default.min.css")
        except Exception as e:
            print(f"Failed to download highlight.default.min.css: {e}")

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.35"', 'APP_VERSION = "1.0.36"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.35";', 'const APP_VERSION = "1.0.36";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10036', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.36"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Define File Patches
    patches = {
        "backend/frontend/index.html": [
            (
                # Inject highlight.js, its default theme, and initialize the requested marked options
                r'''<script src="/js/marked.min.js"></script>''',
                r'''<link rel="stylesheet" href="/css/highlight.default.min.css">
    <script src="/js/marked.min.js"></script>
    <script src="/js/highlight.min.js"></script>
    <script>
        const eHlJs = 'true';
        marked.setOptions({
            gfm: true,
            xhtml: true
        });

        if ('true' == eHlJs) {
            marked.setOptions({
                highlight: function (code) {
                    return hljs.highlightAuto(code).value;
                }
            });
        }
    </script>'''
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
    commit_msg = """feat(markdown): configure marked.js with GFM and highlight.js

- Downloaded and embedded local copies of highlight.min.js and default CSS for offline syntax highlighting
- Injected marked.setOptions to enable GitHub Flavored Markdown (gfm) and xhtml compliance
- Configured automatic syntax highlighting via hljs.highlightAuto when eHlJs parameter is enabled
- Bumped Android versionCode to 10036

Version bumped to 1.0.36"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.36!")

if __name__ == "__main__":
    update_application()