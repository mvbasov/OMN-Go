import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.14"', 'APP_VERSION = "1.0.15"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.14";', 'const APP_VERSION = "1.0.15";')
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
    
    # 2. Recreate the custom AndroidManifest.xml (removed in 1.0.14) to restore permissions
    manifest_content = """<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android"
    package="net.basov.goomn"
    android:versionCode="1"
    android:versionName="1.0.15">
    
    <!-- Network and Storage Permissions -->
    <uses-permission android:name="android.permission.INTERNET" />
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" />
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" />
    <uses-permission android:name="android.permission.MANAGE_EXTERNAL_STORAGE" />

    <application android:label="GoOMN">
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
    print("Restored custom AndroidManifest.xml for permissions (without the forbidden uses-sdk tag).")

    # 3. Patch Dockerfile to revert gomobile sed hacks and inject the AAPT Bash wrapper instead
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        # Regex safely targets the fragile gomobile git clone & sed hack block
        old_pattern = r'RUN git clone --depth 1 https://github\.com/golang/mobile\.git /tmp/mobile && .*?gomobile init'
        
        # New block restores clean gomobile install and intercepts AAPT
        new_block = r'''RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init

# Intercept AAPT to guarantee SDK Target 34 at the lowest compiler level
RUN printf '#!/bin/bash\n\
new_args=()\n\
skip=0\n\
for arg in "$@"; do\n\
  if [ $skip -eq 1 ]; then skip=0; continue; fi\n\
  if [[ "$arg" == "--target-sdk-version" ]] || [[ "$arg" == "--min-sdk-version" ]]; then\n\
    skip=1\n\
  elif [[ "$arg" == --target-sdk-version=* ]] || [[ "$arg" == --min-sdk-version=* ]]; then\n\
    continue\n\
  else\n\
    new_args+=("$arg")\n\
  fi\n\
done\n\
if [[ "${new_args[0]}" == "package" ]] || [[ "${new_args[0]}" == "link" ]]; then\n\
  new_args+=("--min-sdk-version" "21" "--target-sdk-version" "34")\n\
fi\n\
COMMAND_NAME=$(basename "$0")\n\
exec /opt/android/build-tools/33.0.2/${COMMAND_NAME}.real "${new_args[@]}"\n' > /tmp/aapt_wrapper.sh && \
    chmod +x /tmp/aapt_wrapper.sh && \
    mv /opt/android/build-tools/33.0.2/aapt /opt/android/build-tools/33.0.2/aapt.real && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt && \
    mv /opt/android/build-tools/33.0.2/aapt2 /opt/android/build-tools/33.0.2/aapt2.real && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt2'''
        
        if re.search(old_pattern, content, flags=re.DOTALL):
            content = re.sub(old_pattern, lambda m: new_block, content, flags=re.DOTALL)
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Successfully patched Dockerfile with AAPT wrapper.")
        elif "aapt_wrapper.sh" in content:
            print("Dockerfile is already patched with AAPT wrapper.")
        else:
            print("Warning: Could not find gomobile git clone block in Dockerfile.")

    # 4. Output Standardized Git Commit Message
    commit_msg = """build(android): intercept AAPT at OS level to enforce SDK bounds
    
Discovered that gomobile bypasses the AndroidManifest.xml completely by directly 
passing --min-sdk-version and --target-sdk-version flags to AAPT/AAPT2 under 
the hood. This caused all previous XML hacks to be silently overwritten. 

This patch reverts the gomobile source hacks and instead replaces the Android 
AAPT binaries with bash wrappers. These wrappers safely strip gomobile's flags 
and dynamically inject min 21 and target 34 during the final linking step. It 
also restores the custom AndroidManifest.xml (without the forbidden tags) so 
storage permissions are correctly bundled into the final APK.

Version bumped to 1.0.15"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()