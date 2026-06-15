import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.45"', 'APP_VERSION = "1.0.46"'),
        ("frontend/index.html", 'APP_VERSION = "1.0.45"', 'APP_VERSION = "1.0.46"')
    ]

    for filepath, old_v, new_v in version_replacements:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_v in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old_v, new_v))
            # Catch scenario where 1.0.45 version bump was skipped or failed
            elif 'APP_VERSION = "1.0.44"' in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace('APP_VERSION = "1.0.44"', new_v))

    # 2. Synchronize Android APK Version in build.gradle
    gradle_path = "android/app/build.gradle" if os.path.exists("android/app/build.gradle") else "backend/android/app/build.gradle"
    
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            content = f.read()

        # Safely update the versionCode (e.g., to 10046) and versionName (e.g., to "1.0.46")
        new_version_code = "10046"
        new_version_name = '"1.0.46"'
        
        # Regex to match and replace existing values regardless of what they currently are
        content = re.sub(r'versionCode\s+\d+', f'versionCode {new_version_code}', content)
        content = re.sub(r'versionName\s+"[^"]+"', f'versionName {new_version_name}', content)

        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Successfully synchronized Android APK version in {gradle_path}")
    else:
        print(f"Warning: {gradle_path} not found. Could not update Android APK version. Ensure the path is correct.")

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): synchronize APK version with global app version
    
- Implement regex update step for `android/app/build.gradle`.
- Bump native `versionCode` to 10046 and `versionName` to "1.0.46" to ensure Android OS recognizes the application update.

Version bumped to 1.0.46"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()