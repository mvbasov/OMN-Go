import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.17"', 'APP_VERSION = "1.0.18"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.17";', 'const APP_VERSION = "1.0.18";'),
        ("AndroidManifest.xml", 'android:versionName="1.0.17"', 'android:versionName="1.0.18"')
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

    # 2. Patch Dockerfile to strip AAPT wrapper and utilize Apktool post-processing
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        # Scrub the STAGE 1 AAPT Wrapper completely
        aapt_pattern = r"# Intercept AAPT and completely overwrite the intermediate manifest before compilation\nRUN printf '#!/bin/bash.*?cp /tmp/aapt_wrapper\.sh /opt/android/build-tools/33\.0\.2/aapt2"
        
        if re.search(aapt_pattern, content, flags=re.DOTALL):
            content = re.sub(aapt_pattern, "", content, flags=re.DOTALL)
            print("Cleaned up old AAPT wrappers from Dockerfile.")
            
        # Replace the gomobile build command with the Apktool decompilation pipeline
        build_pattern = r"RUN gomobile build -target=android -androidapi 21 -o bin/goomn\.apk \."
        
        new_build_cmd = r'''# Android APK - Built normally, then post-processed with Apktool to inject targetSdkVersion
RUN gomobile build -target=android -androidapi 21 -o bin/goomn.apk . && \
    wget -q https://github.com/iBotPeaches/Apktool/releases/download/v2.9.3/apktool_2.9.3.jar -O /tmp/apktool.jar && \
    java -jar /tmp/apktool.jar d bin/goomn.apk -o /tmp/apk_decoded && \
    sed -i -E 's/<uses-sdk[^>]*>/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\/>/g' /tmp/apk_decoded/AndroidManifest.xml && \
    java -jar /tmp/apktool.jar b /tmp/apk_decoded -o /tmp/goomn_unsigned.apk && \
    /opt/android/build-tools/33.0.2/zipalign -v -p 4 /tmp/goomn_unsigned.apk /tmp/goomn_aligned.apk && \
    keytool -genkey -v -keystore /tmp/debug.keystore -storepass android -alias androiddebugkey -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US" && \
    /opt/android/build-tools/33.0.2/apksigner sign --ks /tmp/debug.keystore --ks-pass pass:android --key-pass pass:android --out bin/goomn.apk /tmp/goomn_aligned.apk && \
    rm -rf /tmp/apktool.jar /tmp/apk_decoded /tmp/goomn_unsigned.apk /tmp/goomn_aligned.apk /tmp/debug.keystore'''

        if re.search(build_pattern, content):
            content = re.sub(build_pattern, lambda m: new_build_cmd, content)
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Successfully patched Dockerfile with Apktool post-processing pipeline.")
        elif "apktool_2.9.3.jar" in content:
            print("Dockerfile is already patched with the Apktool pipeline.")
        else:
            print("Warning: Could not find gomobile build command in Dockerfile.")

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): implement apktool post-processing for target SDK
    
Discovered that gomobile completely bypasses AAPT for the AndroidManifest.xml 
generation. It uses an internal Go package (binres) to strictly serialize the 
manifest to binary XML, actively dropping targetSdkVersion and ignoring our 
system-level AAPT wrappers.

To fix this natively, this patch strips the AAPT OS-wrappers and instead 
introduces Apktool into the Dockerfile. It lets gomobile build the APK, then 
decompiles it, surgically injects targetSdkVersion="34" into the resulting 
XML, rebuilds the binary manifest, aligns the ZIP, and re-signs it with 
apksigner. This definitively bypasses all gomobile manifest limitations.

Version bumped to 1.0.18"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()