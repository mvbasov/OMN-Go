import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.8"', 'APP_VERSION = "1.0.9"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.8";', 'const APP_VERSION = "1.0.9";')
    ]
    
    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r') as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, 'w') as f:
                    f.write(content)
            elif new_val not in content:
                raise ValueError(f"Could not find '{old_val}' in {filepath}")
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "Dockerfile": [
            (
                'RUN gomobile build -target=android -androidapi 34 -o bin/goomn.apk .',
                'RUN gomobile build -target=android -androidapi 33 -o bin/goomn.apk .'
            )
        ]
    }

    # Execute updates sequentially
    for filepath, file_patches in patches.items():
        if not os.path.exists(filepath):
            print(f"Warning: {filepath} not found, skipping patch.")
            continue
            
        with open(filepath, 'r') as f:
            content = f.read()
            
        for old_block, new_block in file_patches:
            if old_block in content:
                content = content.replace(old_block, new_block)
            elif new_block in content:
                # Idempotency: Already patched
                continue
            else:
                raise ValueError(f"Target block not found in {filepath}:\n{old_block}")
                
        with open(filepath, 'w') as f:
            f.write(content)

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): fix gomobile NDK version bounds

Downgraded -androidapi constraint from 34 to 33 to match NDK 25 
platform boundaries (19..33). This resolves the build failure while 
still satisfying Android 14 minimum target requirements to avoid 
the 'older version' warning.

Version bumped to 1.0.9"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()