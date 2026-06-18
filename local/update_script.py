import os
import re
import shutil

def update_application():
    print("Configuring Android Application to consume Adaptive Icons...")

    # 1. Update AndroidManifest.xml to ensure it points to the new mipmap wrappers
    manifest_path = "android/app/src/main/AndroidManifest.xml"
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as f:
            manifest = f.read()

        # Update android:icon
        if 'android:icon=' in manifest:
            manifest = re.sub(r'android:icon="[^"]+"', 'android:icon="@mipmap/ic_launcher"', manifest)
        
        # Update or Inject android:roundIcon
        if 'android:roundIcon=' in manifest:
            manifest = re.sub(r'android:roundIcon="[^"]+"', 'android:roundIcon="@mipmap/ic_launcher_round"', manifest)
        elif 'android:icon="@mipmap/ic_launcher"' in manifest:
            # Inject roundIcon right after standard icon if missing
            manifest = manifest.replace(
                'android:icon="@mipmap/ic_launcher"', 
                'android:icon="@mipmap/ic_launcher"\n        android:roundIcon="@mipmap/ic_launcher_round"'
            )

        with open(manifest_path, "w", encoding="utf-8") as f:
            f.write(manifest)
        print(" -> SUCCESS: AndroidManifest.xml wired to @mipmap/ic_launcher")
    else:
        print(f" -> WARNING: Could not find {manifest_path}")

    # 2. Restructure the F-Droid specific icon into the correct Android Flavor directory
    main_drawable_dir = "android/app/src/main/res/drawable"
    fdroid_res_dir = "android/app/src/fdroid/res/drawable"
    
    fdroid_source = os.path.join(main_drawable_dir, "ic_launcher_foreground_fdroid.xml")
    fdroid_target = os.path.join(fdroid_res_dir, "ic_launcher_foreground.xml")

    if os.path.exists(fdroid_source):
        os.makedirs(fdroid_res_dir, exist_ok=True)
        # Move and rename the file so it exactly overrides the main variant
        shutil.move(fdroid_source, fdroid_target)
        print(f" -> SUCCESS: Relocated F-Droid foreground to '{fdroid_target}'")
    else:
        print(f" -> INFO: '{fdroid_source}' not found. Assuming it was already moved or not present.")

    # 3. Bump Versions to 1.3.8
    print("\nBumping application version to 1.3.8...")
    
    # server.go
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'const APP_VERSION = "1\.3\.\d+"', 'const APP_VERSION = "1.3.8"', code)
        with open(server_path, "w", encoding="utf-8") as f:
            f.write(code)
            
    # index.html
    index_path = "backend/frontend/index.html"
    if os.path.exists(index_path):
        with open(index_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'const APP_VERSION = "1\.3\.\d+";', 'const APP_VERSION = "1.3.8";', code)
        with open(index_path, "w", encoding="utf-8") as f:
            f.write(code)

    # core.js
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.8';", code)
        with open(js_path, "w", encoding="utf-8") as f:
            f.write(code)

    # build.gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            code = f.read()
        code = re.sub(r'versionCode \d+', 'versionCode 10308', code)
        code = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.8"', code)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(code)
            
    print(" -> SUCCESS: Versions updated to 1.3.8 across backend, frontend, and Android.")

    # 4. Output Git Commit Message
    commit_msg = "chore(android): configure adaptive icons and bump version to 1.3.8\n\nWired AndroidManifest.xml to consume the new mipmap wrappers. Migrated the F-Droid specific foreground XML into its dedicated flavor SourceSet to allow automatic build-time asset swapping. Bumped app versions to 1.3.8."
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()