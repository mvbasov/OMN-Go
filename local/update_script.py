import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.22 Metadata UI Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.21"', 'APP_VERSION = "1.2.22"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.21";', 'const APP_VERSION = "1.2.22";'),
        ("backend/frontend/index.html", "let v = '1.2.21';", "let v = '1.2.22';"),
        ("android/app/build.gradle", "versionCode 10221", "versionCode 10222"),
        ("android/app/build.gradle", 'versionName "1.2.21"', 'versionName "1.2.22"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch index.html to fix Metadata button visibility logic
    index_html = "backend/frontend/index.html"
    if os.path.exists(index_html):
        with open(index_html, "r", encoding="utf-8") as f:
            html_content = f.read()

        # Fix A: Remove 'display: none;' from the raw HTML so it shows on load
        old_btn_html = '<button id="metaToggleBtn" onclick="document.getElementById(\'metadataPanel\').classList.toggle(\'hidden\')" style="display: none; background: #17a2b8; color: white; border: none;">Metadata</button>'
        new_btn_html = '<button id="metaToggleBtn" onclick="document.getElementById(\'metadataPanel\').classList.toggle(\'hidden\')" style="display: block; background: #17a2b8; color: white; border: none;">Metadata</button>'
        
        if old_btn_html in html_content:
            html_content = html_content.replace(old_btn_html, new_btn_html)
            print("  [+] Made Metadata button visible by default on page load.")

        # Fix B: Update toggleMode() to restore visibility when returning to View Mode
        old_toggle_logic = """            if(currentMode === 'view') {
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
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                currentMode = 'view';
            }"""

        new_toggle_logic = """            if(currentMode === 'view') {
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
                btn.innerText = 'Edit Mode';
                document.getElementById('saveBtn').style.display = 'none';
                document.getElementById('metaToggleBtn').style.display = 'block';
                currentMode = 'view';
            }"""

        if old_toggle_logic in html_content:
            html_content = html_content.replace(old_toggle_logic, new_toggle_logic)
            print("  [+] Added logic to restore Metadata button visibility when returning to View Mode.")

        with open(index_html, "w", encoding="utf-8") as f:
            f.write(html_content)

    commit_msg = """fix(ui): resolve hidden metadata toggle button

- Changed inline HTML styling on `metaToggleBtn` to `display: block` so the button correctly mounts during standard View Mode initialization.
- Added `style.display = 'block'` restoration inside the `toggleMode()` JavaScript flow, resolving a bug where the button would permanently disappear after exiting Edit Mode.
- Bumped application to V1.2.22 (Android 10222)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()