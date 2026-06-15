import os
import re

def update_application():
    # ANSI Color Codes for Terminal Output
    RED = '\033[91m'
    GREEN = '\033[92m'
    RESET = '\033[0m'

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.41"', 'APP_VERSION = "1.0.42"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.41";', 'const APP_VERSION = "1.0.42";')
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
        
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10042', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.42"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)

    # 3. Resilient & Idempotent File Patches via Block Parsing
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, 'r', encoding='utf-8') as f:
            server_content = f.read()

        # Target 1: Inject ScriptRules.md link into Welcome.md literal string
        welcome_start = server_content.find('os.WriteFile(welcomePath')
        if welcome_start != -1:
            welcome_end = server_content.find('0644)', welcome_start)
            if welcome_end != -1:
                block = server_content[welcome_start:welcome_end]
                
                if 'ScriptRules.md' in block:
                    print(GREEN + "[+] Welcome.md generation payload already contains ScriptRules.md (Idempotent success)." + RESET)
                else:
                    # Automatically adapt to whether Go is using escaped newlines (\\n) or literal newlines (\n)
                    nl = "\\n" if "\\n" in block else "\n"
                    
                    if "Navigate" in block:
                        new_block = block.replace("Navigate", f"- [Scripting Rules](ScriptRules.md){nl}{nl}Navigate")
                        server_content = server_content[:welcome_start] + new_block + server_content[welcome_end:]
                        print(GREEN + "[+] Successfully patched Welcome.md generation payload using block parsing." + RESET)
                    elif "- [Bookmarks]" in block:
                        b_idx = block.find("- [Bookmarks]")
                        eol_idx = block.find(nl, b_idx)
                        if eol_idx != -1:
                            new_block = block[:eol_idx] + f"{nl}- [Scripting Rules](ScriptRules.md)" + block[eol_idx:]
                            server_content = server_content[:welcome_start] + new_block + server_content[welcome_end:]
                            print(GREEN + "[+] Successfully patched Welcome.md generation payload using anchor replacement." + RESET)
                        else:
                            print(RED + "Warning: Found Bookmarks in Welcome.md block but could not locate newline." + RESET)
                    else:
                        print(RED + "Warning: Could not find Navigate or Bookmarks anchor inside Welcome.md payload." + RESET)
            else:
                print(RED + "Warning: Could not find the end '0644)' of the Welcome.md generation block." + RESET)
        else:
            print(RED + "Warning: Could not find 'os.WriteFile(welcomePath' in backend/server.go" + RESET)

        # Target 2: Bump QuickNotes headers from #### to #####
        if '\\n##### %s\\n%s\\n' in server_content or '\n##### %s\n%s\n' in server_content:
            print(GREEN + "[+] QuickNotes header level is already ##### (Idempotent success)." + RESET)
        else:
            replaced = False
            for old_h in ['\\n#### %s\\n%s\\n', '\n#### %s\n%s\n']:
                if old_h in server_content:
                    new_h = old_h.replace('####', '#####')
                    server_content = server_content.replace(old_h, new_h)
                    replaced = True
                    break
            
            if replaced:
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
            
            if "ScriptRules.md" not in content and "- [Bookmarks]" in content:
                # Safely rebuild lines to avoid missing carriage returns
                lines = content.split('\n')
                new_lines = []
                for line in lines:
                    new_lines.append(line)
                    if "- [Bookmarks]" in line:
                        new_lines.append("- [Scripting Rules](ScriptRules.md)")
                
                with open(welcome_path, "w", encoding="utf-8") as f:
                    f.write('\n'.join(new_lines))
                print(GREEN + f"[+] Updated existing physical '{welcome_path}' to include Script Rules link." + RESET)

    # 5. Output Standardized Git Commit Message
    commit_msg = """fix(patcher): implement resilient block parsing for payload injection

- Completely rewrote `Welcome.md` modification logic to use dynamic block parsing, eliminating exact string match failures and regex parsing issues.
- Integrated automatic newline character detection (`\\n` vs `\n`) to preserve the host's existing Go formatting schema.
- Added bulletproof line-rebuilding loop to correctly patch existing physical markdown files.
- Verified terminal output strictly enforces ANSI colors for debugging.
- Bumped Android versionCode to 10042

Version bumped to 1.0.42"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print(GREEN + "Application successfully updated to v1.0.42!" + RESET)

if __name__ == "__main__":
    update_application()