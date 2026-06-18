import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.3.2 APK Version Renaming Patch...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.2"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.2"'),
        "android/app/build.gradle": (r'versionCode 103\d{2}', 'versionCode 10302')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.2';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.2"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Inject Automated APK Renaming into Android build.gradle
    build_gradle = "android/app/build.gradle"
    if os.path.exists(build_gradle):
        with open(build_gradle, "r", encoding="utf-8") as f:
            gradle_code = f.read()

        if "applicationVariants.all" not in gradle_code:
            # We inject the file renaming configuration right before compileOptions
            rename_logic = '''applicationVariants.all { variant ->
        variant.outputs.all { output ->
            outputFileName = "omn-go-v${variant.versionName}-${output.baseName}.apk"
        }
    }

    compileOptions {'''
            new_gradle = gradle_code.replace("compileOptions {", rename_logic)

            if new_gradle != gradle_code:
                with open(build_gradle, "w", encoding="utf-8") as f:
                    f.write(new_gradle)
                print("  [+] Successfully injected dynamic APK renaming configuration into build.gradle.")
            else:
                print("  [-] Could not locate compileOptions block to inject renaming hook.")
        else:
            print("  [=] Automated APK renaming is already configured.")

    commit_msg = """build(android): automatically inject app version into apk filenames

- Injected `applicationVariants.all` configuration into `android/app/build.gradle`.
- Gradle now natively overrides the standard `app-debug.apk` naming convention and dynamically interpolates the current `versionName` and architecture ABI string into the filename (e.g., `omn-go-v1.3.2-arm64-v8a-debug.apk`).
- Bumped application to V1.3.2 (Android 10302)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()