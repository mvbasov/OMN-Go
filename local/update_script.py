import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.3.0 Release Bump...")

    # 1. Regex definitions to catch any 1.2.x version and upgrade to 1.3.0
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.3.0"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.3.0"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10300')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            
            # Apply primary regex bump
            new_content = re.sub(pattern, replacement, content)
            
            # Apply secondary regex for JS variable
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.3.0';", new_content)

            # Apply secondary regex for Android versionName
            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.3.0"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Successfully bumped to V1.3.0 in {filepath}")
            else:
                print(f"  [-] Could not find 1.2.x version signature in {filepath}")

    commit_msg = """chore(release): bump application to v1.3.0

- Upgraded Go backend and HTML frontend version strings to `1.3.0`.
- Incremented Android `versionCode` to `10300` and `versionName` to `1.3.0`.
- This release marks the successful stabilization of the unified `data/html` storage workspace and the zero-dependency embedded Lazy Extraction Router."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()