import os
import re

def update_application():
    target_version = "1.0.50"
    target_code = "10050"

    # 1. Aggressive Global Version Catch-Up
    files_to_bump = ["server.go", "frontend/index.html"]
    for filepath in files_to_bump:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            
            # Force update any lagging APP_VERSION variable
            content = re.sub(r'APP_VERSION\s*=\s*"1\.0\.\d+"', f'APP_VERSION = "{target_version}"', content)
            content = re.sub(r"let v = '1\.0\.\d+';", f"let v = '{target_version}';", content)
            
            with open(actual_path, 'w', encoding='utf-8') as f:
                f.write(content)
            print(f"Force-synced versions to {target_version} in {actual_path}")

    # 2. Synchronize Android APK Version in build.gradle
    gradle_path = "android/app/build.gradle" if os.path.exists("android/app/build.gradle") else "backend/android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            content = f.read()

        content = re.sub(r'versionCode\s+\d+', f'versionCode {target_code}', content)
        content = re.sub(r'versionName\s+"[^"]+"', f'versionName "{target_version}"', content)

        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Successfully synchronized Android APK version in {gradle_path}")

    # 3. Repair the JS Console Crash Bug
    html_path = "frontend/index.html" if os.path.exists("frontend/index.html") else "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, 'r', encoding='utf-8') as f:
            content = f.read()
            
        original_newlines = "\r\n" if "\r\n" in content else "\n"
        normalized_content = content.replace("\r\n", "\n")

        # Patch A: Fix the header injection using triple quotes (no backslash issues)
        old_header = '''header.innerHTML = '<span>JS Console Output</span><button id="goomn-console-close" style="background:#ff5555; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer;">Close</button>';'''
        new_header = '''header.innerHTML = '<span>JS Console Output</span><div><button id="goomn-console-clear" style="background:#888; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer; margin-right:8px;">Clear</button><button id="goomn-console-close" style="background:#ff5555; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer;">Close</button></div>';'''
        
        if old_header in normalized_content:
            normalized_content = normalized_content.replace(old_header, new_header)

        # Patch B: Add a null-check to the handler so it never crashes the IIFE again
        old_handler = '''                document.getElementById('goomn-console-clear').onclick = () => {
                    logs = [];
                    if (logsContainer) logsContainer.innerHTML = '';
                    if (consoleBtn) consoleBtn.innerText = 'Console (0)';
                };'''
        new_handler = '''                let clrBtn = document.getElementById('goomn-console-clear');
                if (clrBtn) {
                    clrBtn.onclick = () => {
                        logs = [];
                        if (logsContainer) logsContainer.innerHTML = '';
                        if (consoleBtn) consoleBtn.innerText = 'Console (0)';
                    };
                }'''

        if old_handler in normalized_content:
            normalized_content = normalized_content.replace(old_handler, new_handler)

        final_content = normalized_content.replace("\n", original_newlines)
        with open(html_path, 'w', encoding='utf-8') as f:
            f.write(final_content)
        print(f"Successfully repaired Console UI logic in {html_path}")

    # 4. Output Standardized Git Commit Message
    commit_msg = """fix(frontend): repair JS console clear button crash
    
- Remove invalid python regex backslashes that prevented the Clear button HTML from injecting.
- Implement robust null checks on console JS bindings to prevent fatal UI thread crashes.
- Synchronize Version to 1.0.50.

Version bumped to 1.0.50"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()