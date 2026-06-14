import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.9"', 'APP_VERSION = "1.0.10"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.9";', 'const APP_VERSION = "1.0.10";')
    ]
    
    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r') as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, 'w') as f:
                    f.write(content)
            elif new_val not in content:
                print(f"Warning: Could not find '{old_val}' in {filepath}")
    
    # 2. Inject explicit AndroidManifest.xml to control targetSdkVersion
    manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="net.basov.goomn"
    android:versionCode="1"
    android:versionName="1.0.10">
    
    <!-- Force targetSdkVersion to 34 to satisfy Android 14 requirements -->
    <uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34" />
    
    <!-- Network and Storage Permissions -->
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" android:maxSdkVersion="29" />
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" android:maxSdkVersion="29" />
    <uses-permission android:name="android.permission.MANAGE_EXTERNAL_STORAGE" />

    <application android:label="GoOMN" android:hasFragileUserData="true" android:requestLegacyExternalStorage="true">
        <activity android:name="org.golang.app.GoNativeActivity"
            android:label="GoOMN"
            android:configChanges="orientation|keyboardHidden|screenSize"
            android:exported="true">
            <meta-data android:name="android.app.lib_name" android:value="goomn" />
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
                <category android:name="android.intent.category.LAUNCHER" />
            </intent-filter>
        </activity>
    </application>
</manifest>
"""
    with open("AndroidManifest.xml", "w") as f:
        f.write(manifest_content)

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): enforce targetSdkVersion 34 via custom manifest

Added a custom AndroidManifest.xml to explicitly set targetSdkVersion to 34.
This overrides gomobile's default manifest generation (which left the target 
SDK too low when using the older NDK build boundaries) and resolves the 
'Built for an older version' warning on Android 14. Also explicitly includes 
required network and storage permissions.

Version bumped to 1.0.10"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()