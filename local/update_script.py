import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.16 WebView Routing & Stability Fixes...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.15"', 'APP_VERSION = "1.2.16"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.15";', 'const APP_VERSION = "1.2.16";'),
        ("backend/frontend/index.html", "let v = '1.2.15';", "let v = '1.2.16';"),
        ("android/app/build.gradle", "versionCode 10215", "versionCode 10216"),
        ("android/app/build.gradle", 'versionName "1.2.15"', 'versionName "1.2.16"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Add WAKE_LOCK Permission to AndroidManifest.xml
    manifest_path = "android/app/src/main/AndroidManifest.xml"
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as f:
            manifest_content = f.read()
        
        old_perms = '<uses-permission android:name="android.permission.INTERNET" />'
        new_perms = '<uses-permission android:name="android.permission.INTERNET" />\n    <uses-permission android:name="android.permission.WAKE_LOCK" />'
        
        if old_perms in manifest_content and "android.permission.WAKE_LOCK" not in manifest_content:
            manifest_content = manifest_content.replace(old_perms, new_perms)
            with open(manifest_path, "w", encoding="utf-8") as f:
                f.write(manifest_content)
            print(f"  [+] Injected WAKE_LOCK permission into AndroidManifest.xml")
        else:
            print(f"  [=] WAKE_LOCK permission already present or target missing in manifest.")

    # 3. Patch MainActivity.java: Fix URL overriding and Add WakeLock Acquisition
    main_activity = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(main_activity):
        with open(main_activity, "r", encoding="utf-8") as f:
            java_content = f.read()

        # Fix A: Update shouldOverrideUrlLoading to accept 127.0.0.1
        old_url_check = 'if (!url.contains("localhost")) {'
        new_url_check = 'if (!url.contains("localhost") && !url.contains("127.0.0.1")) {'
        if old_url_check in java_content:
            java_content = java_content.replace(old_url_check, new_url_check)
            print("  [+] Fixed WebView interceptor to retain 127.0.0.1 inside the app.")

        # Fix B: Acquire Partial Wake Lock on Server Start
        old_backend_start = 'Backend.startServer();'
        new_backend_start = '''Backend.startServer();

        // Acquire partial wake lock to keep the Go server alive in the background
        try {
            android.os.PowerManager pm = (android.os.PowerManager) getSystemService(android.content.Context.POWER_SERVICE);
            android.os.PowerManager.WakeLock wl = pm.newWakeLock(android.os.PowerManager.PARTIAL_WAKE_LOCK, "OMNGo::ServerWakeLock");
            wl.acquire();
        } catch (Exception e) {
            e.printStackTrace();
        }'''
        
        if old_backend_start in java_content and "PARTIAL_WAKE_LOCK" not in java_content:
            java_content = java_content.replace(old_backend_start, new_backend_start)
            print("  [+] Injected Partial Wake Lock acquisition to prevent background sleeping.")

        with open(main_activity, "w", encoding="utf-8") as f:
            f.write(java_content)

    commit_msg = """fix(android): resolve black screen webview bypass and background sleep

- Updated `shouldOverrideUrlLoading` inside `MainActivity.java` to whitelist `127.0.0.1`, stopping the app from punting its own localhost connection out to the Chrome system browser.
- Added `WAKE_LOCK` permissions to the Android Manifest.
- Initialized a `PARTIAL_WAKE_LOCK` upon server boot to stop Android from freezing the CPU and killing the Go server when the app is minimized or the screen turns off.
- Bumped application to V1.2.16 (Android 10216)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()