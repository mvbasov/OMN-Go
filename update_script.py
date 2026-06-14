import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.10"', 'APP_VERSION = "1.0.11"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.10";', 'const APP_VERSION = "1.0.11";')
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
    
    # 2. Fix AndroidManifest.xml by removing the illegal <uses-sdk> tag
    manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="net.basov.goomn"
    android:versionCode="1"
    android:versionName="1.0.11">
    
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

    # 3. Patch Dockerfile to hack gomobile's internal template and restore broad min SDK
    patches = {
        "Dockerfile": [
            (
                'RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init',
                'RUN git clone --depth 1 https://github.com/golang/mobile.git /tmp/mobile && \\\n    sed -i \'s/targetSdkVersion="29"/targetSdkVersion="34"/g\' /tmp/mobile/cmd/gomobile/build_androidapp.go && \\\n    cd /tmp/mobile/cmd/gomobile && \\\n    go install . && \\\n    gomobile init'
            ),
            (
                'RUN gomobile build -target=android -androidapi 33 -o bin/goomn.apk .',
                'RUN gomobile build -target=android -androidapi 21 -o bin/goomn.apk .'
            )
        ]
    }

    # Execute updates sequentially
    for filepath, file_patches in patches.items():
        if not os.path.exists(filepath):
            print(f"Warning: {filepath} not found, skipping patch.")
            continue
            
        with open(filepath, 'r') as f:
            content = f.read()
            
        for old_block, new_block in file_patches:
            if old_block in content:
                content = content.replace(old_block, new_block)
            elif new_block in content:
                # Idempotency: Already patched
                continue
            else:
                print(f"Warning: Target block not found in {filepath}:\n{old_block}")
                
        with open(filepath, 'w') as f:
            f.write(content)

    # 4. Output Standardized Git Commit Message
    commit_msg = """build(android): fix gomobile uses-sdk restriction and target SDK

Removed the explicit <uses-sdk> tag from custom AndroidManifest.xml as 
gomobile strictly rejects it. Instead, patched the Dockerfile to clone the 
gomobile source, intercept its internal build_androidapp.go template, and 
force targetSdkVersion="34" before compiling gomobile itself. This gracefully 
forces the highest target SDK to bypass Android 14 warnings while allowing 
us to safely drop the gomobile build flag back to -androidapi 21 for 
maximum older device compatibility.

Version bumped to 1.0.11"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()