import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.26"', 'APP_VERSION = "1.0.27"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.26";', 'const APP_VERSION = "1.0.27";')
    ]
    
    for filepath, old, new in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r', encoding='utf-8') as f:
                content = f.read()
            if old in content:
                with open(filepath, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old, new))

    # 2. Fix the Gradle Syntax Bug
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            gradle_content = f.read()
        
        # The previous script accidentally injected "signingConfig signingConfigs.release" 
        # inside the signingConfigs block itself. We remove it from that specific block.
        # We use a regex that looks for the bad injection right above storeFile.
        bad_block = r"release\s*\{\s*signingConfig signingConfigs\.release\s*storeFile file\('goomn\.keystore'\)"
        good_block = "release {\n            storeFile file('goomn.keystore')"
        gradle_content = re.sub(bad_block, good_block, gradle_content)
        
        # Also mathematically increment versionCode since we are creating a new patch
        gradle_content = re.sub(r'versionCode\s+\d+', 'versionCode 10027', gradle_content)
        gradle_content = re.sub(r'versionName\s+".*?"', 'versionName "1.0.27"', gradle_content)
        
        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(gradle_content)
    else:
        print("Warning: android/app/build.gradle not found!")

    # 3. Output Standardized Git Commit Message
    commit_msg = """fix(android): resolve build.gradle signingConfig syntax error

- Removed recursive signingConfig assignment inside the signingConfigs block
- Restored correct Gradle DSL format
- Bumped Android versionCode to 10027

Version bumped to 1.0.27"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]\n")
    print("Application successfully updated to v1.0.27!")

if __name__ == "__main__":
    update_application()