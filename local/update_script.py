import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.3.1 Android ABI Splits Patch...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.1"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.1"'),
        "android/app/build.gradle": (r'versionCode 103\d{2}', 'versionCode 10301')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.1';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.1"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Inject ABI Splits into Android build.gradle
    build_gradle = "android/app/build.gradle"
    if os.path.exists(build_gradle):
        with open(build_gradle, "r", encoding="utf-8") as f:
            gradle_code = f.read()

        if "splits {" not in gradle_code:
            # We inject the splits configuration securely right above the buildTypes block
            split_logic = '''splits {
        abi {
            enable true
            reset()
            include "armeabi-v7a", "arm64-v8a", "x86", "x86_64"
            universalApk true // Set to false if you want ONLY the split APKs
        }
    }

    buildTypes {'''
            new_gradle = gradle_code.replace("buildTypes {", split_logic)

            if new_gradle != gradle_code:
                with open(build_gradle, "w", encoding="utf-8") as f:
                    f.write(new_gradle)
                print("  [+] Successfully injected ABI splits configuration into build.gradle.")
            else:
                print("  [-] Could not locate buildTypes block to inject splits.")
        else:
            print("  [=] ABI splits are already configured.")

    commit_msg = """build(android): implement ABI architecture splits for optimized APK sizing

- Injected `splits { abi { ... } }` configuration into `android/app/build.gradle`.
- Gradle now unpacks the fat Gomobile `.aar` library and generates distinct, lightweight APKs for `arm64-v8a`, `armeabi-v7a`, `x86`, and `x86_64` targets.
- Preserved `universalApk true` to guarantee a fallback fat binary is still generated alongside the optimized splits.
- Bumped application to V1.3.1 (Android 10301)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()