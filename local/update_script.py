import os
import re

def update_application():
    target_version = "1.1.0"
    target_code = "10100"

    # 1. Aggressive Global Version Catch-Up
    files_to_bump = ["server.go", "frontend/index.html"]
    for filepath in files_to_bump:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            
            # Force update any lagging APP_VERSION variable (regex expanded for minor/major bumps)
            content = re.sub(r'APP_VERSION\s*=\s*"\d+\.\d+\.\d+"', f'APP_VERSION = "{target_version}"', content)
            content = re.sub(r"let v = '\d+\.\d+\.\d+';", f"let v = '{target_version}';", content)
            
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

    # 3. Output Standardized Git Commit Message
    commit_msg = """chore(release): bump version to 1.1.0
    
- Update global application version strings to 1.1.0.
- Synchronize Android versionCode to 10100.
- Enhance regex version matchers to support minor/major version boundaries.

Version bumped to 1.1.0"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()