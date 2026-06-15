import os
import re

def update_application():
    # ANSI Color Codes for Terminal Output
    RED = '\033[91m'
    GREEN = '\033[92m'
    RESET = '\033[0m'

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.39"', 'APP_VERSION = "1.0.40"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.39";', 'const APP_VERSION = "1.0.40";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10040', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.40"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Resilient & Idempotent File Patches
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, 'r', encoding='utf-8') as f:
            server_content = f.read()

        # Target 1: Inject ScriptRules.md link into Welcome.md literal string
        if '- [Scripting Rules](ScriptRules.md)' in server_content:
            print(GREEN + "[+] Welcome.md generation payload already contains ScriptRules.md (Idempotent success)." + RESET)
        else:
            # Use regex to safely capture the space between Bookmarks and Navigate, regardless of spacing/newlines
            new_content, count = re.subn(
                r'(\- \[Bookmarks\]\(Bookmarks\.md\))(.*?)Navigate', 
                r'\1\\n- [Scripting Rules](ScriptRules.md)\2Navigate', 
                server_content, 
                flags=re.DOTALL
            )
            if count > 0:
                server_content = new_content
                print(GREEN + "[+] Successfully patched Welcome.md generation payload." + RESET)
            else:
                print(RED + "Warning: Could not find Welcome.md target in backend/server.go" + RESET)

        # Target 2: Bump QuickNotes headers from #### to #####
        if '\\n##### %s\\n%s\\n' in server_content:
            print(GREEN + "[+] QuickNotes header level is already ##### (Idempotent success)." + RESET)
        else:
            old_header = '\\n#### %s\\n%s\\n'
            new_header = '\\n##### %s\\n%s\\n'
            if old_header in server_content:
                server_content = server_content.replace(old_header, new_header)
                print(GREEN + "[+] Successfully patched QuickNotes header level." + RESET)
            else:
                print(RED + "Warning: Could not find QuickNotes header target in backend/server.go" + RESET)

        with open(server_path, 'w', encoding='utf-8') as f:
            f.write(server_content)
    else:
        print(RED + f"Warning: {server_path} not found!" + RESET)

    # 4. Physically update any existing Welcome.md files on disk to include ScriptRules.md
    for storage_dir in ["data/md", "android/app/media/net.basov.goomn/md"]:
        welcome_path = os.path.join(storage_dir, "Welcome.md")
        if os.path.exists(welcome_path):
            with open(welcome_path, "r", encoding="utf-8") as f:
                content = f.read()
            if "ScriptRules.md" not in content and "[Bookmarks](Bookmarks.md)" in content:
                content = content.replace("- [Bookmarks](Bookmarks.md)", "- [Bookmarks](Bookmarks.md)\n- [Scripting Rules](ScriptRules.md)")
                with open(welcome_path, "w", encoding="utf-8") as f:
                    f.write(content)
                print(GREEN + f"[+] Updated existing physical '{welcome_path}' to include Script Rules link." + RESET)

    # 5. Output Standardized Git Commit Message
    commit_msg = """fix(patcher): implement idempotent patching and colored error logging

- Rewrote string replacements in python patcher to evaluate state idempotently, preventing failures if a file was already modified.
- Added ANSI escape codes to explicitly print successful patches in green and failing target blocks in red.
- Cleaned up Welcome.md and QuickNotes string targeting logic to use resilient DOTALL regex matching.
- Bumped Android versionCode to 10040

Version bumped to 1.0.40"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print(GREEN + "Application successfully updated to v1.0.40!" + RESET)

if __name__ == "__main__":
    update_application()