import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.3"', 'APP_VERSION = "1.0.4"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.3";', 'const APP_VERSION = "1.0.4";')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "Dockerfile": [
            (
                "RUN gomobile build -target=android -androidapi 21 -javapkg net.basov.goomn.fdroid -o bin/goomn.apk server.go main_android.go",
                "RUN gomobile build -target=android -androidapi 21 -o bin/goomn.apk server.go main_android.go"
            )
        ]
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
                    
    # Execute patching sequentially
    for filename, file_patches in patches.items():
        if not os.path.exists(filename):
            print(f"Skipping {filename}: File not found.")
            continue
            
        with open(filename, 'r', encoding='utf-8') as f:
            content = f.read()
        
        for idx, (old_str, new_str) in enumerate(file_patches):
            if old_str in content:
                content = content.replace(old_str, new_str)
            elif new_str in content:
                print(f"[{filename}] Patch target #{idx} is already applied. Skipping.")
            else:
                raise ValueError(f"Could not find patch target #{idx} in {filename}:\n{old_str[:100]}...")
                
        with open(filename, 'w', encoding='utf-8') as f:
            f.write(content)
            
        print(f"Patched: {filename}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """fix(build): remove invalid -javapkg flag from gomobile build command

Version bumped to 1.0.4"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()