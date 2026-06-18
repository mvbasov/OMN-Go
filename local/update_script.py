import os
import re
import shutil

def update_application():
    print("Fixing AAPT build failure and cleaning up invalid mipmaps...")

    # 1. Remove the invalid <adaptive-icon> fallbacks from mipmap-anydpi
    # AAPT fails because <adaptive-icon> is strictly API 26+ only.
    invalid_base_mipmap = "android/app/src/main/res/mipmap-anydpi"
    if os.path.exists(invalid_base_mipmap):
        for xml_name in ["ic_launcher.xml", "ic_launcher_round.xml"]:
            invalid_file = os.path.join(invalid_base_mipmap, xml_name)
            if os.path.exists(invalid_file):
                os.remove(invalid_file)
                print(f" -> Removed invalid fallback: {invalid_file}")
        
        # If the directory is now empty, remove it to be clean
        if not os.listdir(invalid_base_mipmap):
            os.rmdir(invalid_base_mipmap)
            print(f" -> Removed empty directory: {invalid_base_mipmap}")

    # Ensure the v26 directory exists as it's the correct place for adaptive icons
    v26_mipmap = "android/app/src/main/res/mipmap-anydpi-v26"
    if not os.path.exists(v26_mipmap):
         print(f" -> WARNING: {v26_mipmap} does not exist. Ensure your adaptive XMLs are here.")

    # 2. Force all Vector Drawables to exactly 108dp (Keeping previous valid fix)
    target_dirs = [
        "android/app/src/main/res/drawable",
        "android/app/src/fdroid/res/drawable"
    ]

    for d in target_dirs:
        if not os.path.exists(d): 
            continue
        for filename in os.listdir(d):
            if not filename.endswith(".xml"): 
                continue
            
            filepath = os.path.join(d, filename)
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()

            if "<vector" in content:
                # Fix Width
                if 'android:width="' in content:
                    content = re.sub(r'android:width="[^"]+"', 'android:width="108dp"', content)
                else:
                    content = content.replace("<vector", '<vector android:width="108dp"')
                
                # Fix Height
                if 'android:height="' in content:
                    content = re.sub(r'android:height="[^"]+"', 'android:height="108dp"', content)
                else:
                    content = content.replace("<vector", '<vector android:height="108dp"')

                # Ensure standard Android namespace exists just in case
                if 'xmlns:android=' not in content:
                    content = content.replace("<vector", '<vector xmlns:android="http://schemas.android.com/apk/res/android"')

                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f" -> Verified 108dp bounds in: {filepath}")

    # 3. Bump Versions to 1.3.10
    print("\nBumping application version to 1.3.10...")
    
    # server.go
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'const APP_VERSION = "1\.3\.\d+"', 'const APP_VERSION = "1.3.10"', code)
        with open(server_path, "w", encoding="utf-8") as f:
            f.write(code)
            
    # index.html
    index_path = "backend/frontend/index.html"
    if os.path.exists(index_path):
        with open(index_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'const APP_VERSION = "1\.3\.\d+";', 'const APP_VERSION = "1.3.10";', code)
        with open(index_path, "w", encoding="utf-8") as f:
            f.write(code)

    # core.js
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.10';", code)
        with open(js_path, "w", encoding="utf-8") as f:
            f.write(code)

    # build.gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'versionCode \d+', 'versionCode 10310', code)
        code = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.10"', code)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(code)
            
    print(" -> SUCCESS: Versions updated to 1.3.10 across backend, frontend, and Android.")

    commit_msg = "fix(android): remove invalid adaptive icon fallback and bump version\n\nDeleted the mipmap-anydpi fallback for adaptive icons which caused AAPT compilation failures on older API targets. Maintained the correct mipmap-anydpi-v26 location. Bumped app version to 1.3.10."
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()