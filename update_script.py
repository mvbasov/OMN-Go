import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.6"', 'APP_VERSION = "1.0.7"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.6";', 'const APP_VERSION = "1.0.7";')
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
        content = re.sub(
            r'(//go:embed frontend/index\.html\nvar frontendHTML \[\]byte\n+)+', 
            '//go:embed frontend/index.html\nvar frontendHTML []byte\n\n', 
            content
        )
                         
        # 2. Collapse multiple embed imports AND satisfy Go's blank identifier rule
        # This turns any combination of `\t"embed"\n` or `\t_ "embed"\n` into a single `\t_ "embed"\n`
        content = re.sub(
            r'(\t_? "embed"\n)+', 
            '\t_ "embed"\n', 
            content
        )
        
        with open("server.go", 'w', encoding='utf-8') as f:
            f.write(content)
        print("Patched: server.go (Cleaned up imports and removed duplicated variables)")
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
    commit_msg = """fix(core): resolve unused embed import and prevent redundant go:embed variable declarations

Version bumped to 1.0.7"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()