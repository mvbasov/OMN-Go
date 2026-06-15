import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.38"', 'APP_VERSION = "1.0.39"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.38";', 'const APP_VERSION = "1.0.39";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10039', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.39"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Resilient File Patches (Micro-substring replacements)
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, 'r', encoding='utf-8') as f:
            server_content = f.read()

        # Target 1: Inject ScriptRules.md link into Welcome.md literal string
        # Using double-escaped newlines to match Go's literal "\n" string in the source code
        old_welcome = '- [Bookmarks](Bookmarks.md)\\n\\nNavigate'
        new_welcome = '- [Bookmarks](Bookmarks.md)\\n- [Scripting Rules](ScriptRules.md)\\n\\nNavigate'
        if old_welcome in server_content:
            server_content = server_content.replace(old_welcome, new_welcome)
            print("[+] Successfully patched Welcome.md generation payload.")
        elif new_welcome not in server_content:
            print("Warning: Could not find Welcome.md target in backend/server.go")

        # Target 2: Bump QuickNotes headers from #### to #####
        old_header = '\\n#### %s\\n%s\\n'
        new_header = '\\n##### %s\\n%s\\n'
        if old_header in server_content:
            server_content = server_content.replace(old_header, new_header)
            print("[+] Successfully patched QuickNotes header level.")
        elif new_header not in server_content:
            print("Warning: Could not find QuickNotes header target in backend/server.go")

        with open(server_path, 'w', encoding='utf-8') as f:
            f.write(server_content)
    else:
        print(f"Warning: {server_path} not found!")

    # 4. Physically update any existing Welcome.md files on disk to include ScriptRules.md
    for storage_dir in ["data/md", "android/app/media/net.basov.goomn/md"]:
        welcome_path = os.path.join(storage_dir, "Welcome.md")
        if os.path.exists(welcome_path):
            with open(welcome_path, "r", encoding="utf-8") as f:
                content = f.read()
            if "ScriptRules.md" not in content and "[Bookmarks](Bookmarks.md)" in content:
                # Using actual newlines here because physical Markdown files contain real line breaks
                content = content.replace("- [Bookmarks](Bookmarks.md)", "- [Bookmarks](Bookmarks.md)\n- [Scripting Rules](ScriptRules.md)")
                with open(welcome_path, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"[+] Updated existing physical '{welcome_path}' to include Script Rules link.")

    # 5. Output Standardized Git Commit Message
    commit_msg = """fix(markdown): add script rules link to welcome page and fix quicknote header level

- Replaced fragile multi-line exact matches with highly resilient micro-substring replacements for Go backend source edits.
- Added loop to automatically edit any pre-existing physical `Welcome.md` files so users immediately see the rules link.
- Updated `handleQuickNote` backend endpoint to generate `#####` (Level 5) headers instead of `####` for timestamp entries.
- Bumped Android versionCode to 10039

Version bumped to 1.0.39"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.39!")

if __name__ == "__main__":
    update_application()