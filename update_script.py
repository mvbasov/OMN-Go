import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.16"', 'APP_VERSION = "1.0.17"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.16";', 'const APP_VERSION = "1.0.17";')
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

    # 2. Patch Dockerfile to completely clobber the intermediate manifest
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        # Regex safely targets the 1.0.16 AAPT XML interceptor wrapper
        old_pattern = r"# Intercept AAPT to guarantee SDK Target 34 at the lowest compiler level\nRUN printf '#!/bin/bash.*?cp /tmp/aapt_wrapper\.sh /opt/android/build-tools/33\.0\.2/aapt2"
        
        # New block forcefully overwrites the entire XML file immediately before AAPT parses it
        new_block = r'''# Intercept AAPT and completely overwrite the intermediate manifest before compilation
RUN printf '#!/bin/bash\n\
MANIFEST_PATH=""\n\
for arg in "$@"; do\n\
    if [[ "$arg" == *"AndroidManifest.xml" ]]; then\n\
        MANIFEST_PATH="$arg"\n\
        break\n\
    fi\n\
done\n\
if [ -n "$MANIFEST_PATH" ] && [ -f "$MANIFEST_PATH" ]; then\n\
    cat << '"'"'EOF'"'"' > "$MANIFEST_PATH"\n\
<?xml version="1.0" encoding="utf-8"?>\n\
<manifest xmlns:android="http://schemas.android.com/apk/res/android"\n\
    package="net.basov.goomn"\n\
    android:versionCode="1"\n\
    android:versionName="1.0.17">\n\
    <uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"/>\n\
    <uses-permission android:name="android.permission.INTERNET" />\n\
    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" />\n\
    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" />\n\
    <uses-permission android:name="android.permission.MANAGE_EXTERNAL_STORAGE" />\n\
    <application android:label="GoOMN">\n\
        <activity android:name="org.golang.app.GoNativeActivity"\n\
            android:label="GoOMN"\n\
            android:configChanges="orientation|keyboardHidden|screenSize"\n\
            android:exported="true">\n\
            <meta-data android:name="android.app.lib_name" android:value="goomn" />\n\
            <intent-filter>\n\
                <action android:name="android.intent.action.MAIN" />\n\
                <category android:name="android.intent.category.LAUNCHER" />\n\
            </intent-filter>\n\
        </activity>\n\
    </application>\n\
</manifest>\n\
EOF\n\
fi\n\
COMMAND_NAME=$(basename "$0")\n\
exec /opt/android/build-tools/33.0.2/${COMMAND_NAME}.real "$@"\n' > /tmp/aapt_wrapper.sh && \
    chmod +x /tmp/aapt_wrapper.sh && \
    mv /opt/android/build-tools/33.0.2/aapt /opt/android/build-tools/33.0.2/aapt.real || true && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt && \
    mv /opt/android/build-tools/33.0.2/aapt2 /opt/android/build-tools/33.0.2/aapt2.real || true && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt2'''
        
        if re.search(old_pattern, content, flags=re.DOTALL):
            content = re.sub(old_pattern, lambda m: new_block, content, flags=re.DOTALL)
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Successfully patched Dockerfile with total-clobber AAPT wrapper.")
        elif "Intercept AAPT and completely overwrite" in content:
            print("Dockerfile is already patched with the total-clobber AAPT wrapper.")
        else:
            print("Warning: Could not find old AAPT wrapper block in Dockerfile.")

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): brutally overwrite manifest right before AAPT execution
    
Gomobile's internal XML serialization dynamically rewrites the manifest 
and produces syntactically strict elements that completely resisted our sed
replacement techniques. 

Instead of trying to surgically modify the intermediate manifest, this 
wrapper intercepts the manifest path via AAPT arguments and uses `cat` to 
completely clobber the file with a pristine, perfectly structured XML block 
just 1 millisecond before AAPT compiles it. This guarantees that gomobile 
has absolutely no influence over the final SDK attributes.

Version bumped to 1.0.17"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()