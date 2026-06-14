import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.7"', 'APP_VERSION = "1.0.8"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.7";', 'const APP_VERSION = "1.0.8";')
    ]
    
    # 2. Define File Patches
    patches = {
        # Intentionally left empty. The fixes for server.go are handled safely 
        # via the regex cleanup block below to repair the dirty file state.
    }

    # Execute version updates safely (idempotent)
    for filename, old_str, new_str in version_replacements:
        if os.path.exists(filename):
            with open(filename, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_str in content:
                content = content.replace(old_str, new_str)
                with open(filename, 'w', encoding='utf-8') as f:
                    f.write(content)

    # --- Repair duplicated injection states (Fixes the redeclaration bugs) ---
    if os.path.exists("server.go"):
        with open("server.go", 'r', encoding='utf-8') as f:
            content = f.read()
        
        # 1. Collapse multiple go:embed variable declarations down to just ONE
        # (Using \r? to account for Windows/Docker CRLF bridging)
        content = re.sub(
            r'(//go:embed frontend/index\.html\r?\nvar frontendHTML \[\]byte\r?\n+)+', 
            '//go:embed frontend/index.html\nvar frontendHTML []byte\n\n', 
            content
        )
                         
        # 2. Fix the embed import to use the blank identifier (agnostic to \n vs \r\n)
        content = re.sub(r'^[ \t]*"embed"[ \t]*$', '\t_ "embed"', content, flags=re.MULTILINE)
        
        # 3. Collapse multiple blank embed imports into one just in case
        content = re.sub(r'(^[ \t]*_[ \t]+"embed"[ \t]*\r?\n)+', '\t_ "embed"\n', content, flags=re.MULTILINE)
        
        with open("server.go", 'w', encoding='utf-8') as f:
            f.write(content)
        print("Patched: server.go (Cleaned up imports and line endings)")
    # -------------------------------------------------------------------------
                
    # Execute patching sequentially
    for filename, file_patches in patches.items():
        if not os.path.exists(filename):
            print(f"Skipping {filename}: File not found.")
            continue
            
        with open(filename, 'r', encoding='utf-8') as f:
            content = f.read()
        
        for idx, (old_str, new_str) in enumerate(file_patches):
            # Check for new_str FIRST to prevent substring duplication loops on multiple runs
            if new_str in content:
                print(f"[{filename}] Patch target #{idx} is already applied. Skipping.")
            elif old_str in content:
                content = content.replace(old_str, new_str)
            else:
                raise ValueError(f"Could not find patch target #{idx} in {filename}:\n{old_str[:100]}...")
                
        with open(filename, 'w', encoding='utf-8') as f:
            f.write(content)
            
        print(f"Patched: {filename}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """fix(core): properly assign blank identifier to embed import handling CRLF endings

Version bumped to 1.0.8"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()