import os
import re
import shutil

def update_application():
    print("Applying Vector Drawable and Android 14 bounds fixes...")

    # 1. Force all Vector Drawables to exactly 108dp
    # The converter likely output android:width="576dp". This violates Adaptive Icon constraints.
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
                print(f" -> Fixed dimensions to 108dp in: {filepath}")

    # 2. Provide a Base Mipmap Fallback
    # Some OEM Launchers (Samsung/Pixel) fail if ONLY -v26 exists and no base mipmap is declared
    base_mipmap = "android/app/src/main/res/mipmap-anydpi"
    v26_mipmap = "android/app/src/main/res/mipmap-anydpi-v26"
    
    if os.path.exists(v26_mipmap):
        os.makedirs(base_mipmap, exist_ok=True)
        for xml_name in ["ic_launcher.xml", "ic_launcher_round.xml"]:
            src = os.path.join(v26_mipmap, xml_name)
            tgt = os.path.join(base_mipmap, xml_name)
            if os.path.exists(src) and not os.path.exists(tgt):
                shutil.copy(src, tgt)
                print(f" -> Cloned base fallback to: {tgt}")

    # 3. Ensure AndroidManifest.xml Application Tag actually received the attributes
    # If your original manifest didn't have an icon tag, the previous regex wouldn't have matched it.
    manifest_path = "android/app/src/main/AndroidManifest.xml"
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as f:
            manifest = f.read()
        
        # Inject if totally missing
        if 'android:icon=' not in manifest:
            manifest = re.sub(
                r'(<application\s+)', 
                r'\1android:icon="@mipmap/ic_launcher"\n        android:roundIcon="@mipmap/ic_launcher_round"\n        ', 
                manifest
            )
            with open(manifest_path, "w", encoding="utf-8") as f:
                f.write(manifest)
            print(" -> Forcefully injected missing android:icon into AndroidManifest.xml")

    # 4. Bump Versions to 1.3.9
    print("\nBumping application version to 1.3.9...")
    
    # server.go
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'const APP_VERSION = "1\.3\.\d+"', 'const APP_VERSION = "1.3.9"', code)
        with open(server_path, "w", encoding="utf-8") as f:
            f.write(code)
            
    # index.html
    index_path = "backend/frontend/index.html"
    if os.path.exists(index_path):
        with open(index_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'const APP_VERSION = "1\.3\.\d+";', 'const APP_VERSION = "1.3.9";', code)
        with open(index_path, "w", encoding="utf-8") as f:
            f.write(code)

    # core.js
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.9';", code)
        with open(js_path, "w", encoding="utf-8") as f:
            f.write(code)

    # build.gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'versionCode \d+', 'versionCode 10309', code)
        code = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.9"', code)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(code)
            
    print(" -> SUCCESS: Versions updated to 1.3.9 across backend, frontend, and Android.")

    commit_msg = "fix(android): enforce 108dp vector bounds and bump version\n\nCorrected SVG-to-Vector converter dimension mismatches that caused Android 14 to reject the adaptive icon payload. Propagated base mipmap-anydpi fallback for strict OEM launchers. Bumped app version to 1.3.9."
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()