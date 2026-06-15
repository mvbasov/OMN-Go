import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.18"', 'APP_VERSION = "1.0.19"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.18";', 'const APP_VERSION = "1.0.19";'),
        ("AndroidManifest.xml", 'android:versionName="1.0.18"', 'android:versionName="1.0.19"')
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

    # 2. Patch server.go Storage Path to prevent the EACCES Crash Loop
    if os.path.exists("server.go"):
        with open("server.go", "r") as f:
            content = f.read()
        old_path = 'storageDir = "/storage/emulated/0/Media/net.basov.goomn"'
        new_path = 'storageDir = "/storage/emulated/0/Android/media/net.basov.goomn"'
        if old_path in content:
            content = content.replace(old_path, new_path)
            with open("server.go", "w") as f:
                f.write(content)
            print("Updated isolated storage path in server.go")

    # 3. Patch Dockerfile to increment versionCode logically (10019)
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        # Use regex to safely capture the existing Apktool RUN block regardless of whether it has 2, 10018, or none.
        old_pattern = r"# Android APK - Built normally, then post-processed with Apktool.*?(?=rm -rf /tmp/apktool\.jar).*?rm -rf /tmp/apktool\.jar[^\n]*"
        
        new_block = r'''# Android APK - Built normally, then post-processed with Apktool to inject targetSdkVersion and bump versionCode
RUN gomobile build -target=android -androidapi 21 -o bin/goomn.apk . && \
    wget -q https://github.com/iBotPeaches/Apktool/releases/download/v2.9.3/apktool_2.9.3.jar -O /tmp/apktool.jar && \
    java -jar /tmp/apktool.jar d bin/goomn.apk -o /tmp/apk_decoded && \
    perl -0777 -pi -e 's/<uses-sdk[^>]*>/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\/>/gs' /tmp/apk_decoded/AndroidManifest.xml && \
    sed -i -E 's/android:versionCode="[0-9]+"/android:versionCode="10019"/g' /tmp/apk_decoded/AndroidManifest.xml && \
    sed -i -E "s/versionCode: '[0-9]+'/versionCode: '10019'/g" /tmp/apk_decoded/apktool.yml && \
    java -jar /tmp/apktool.jar b /tmp/apk_decoded -o /tmp/goomn_unsigned.apk && \
    /opt/android/build-tools/33.0.2/zipalign -v -p 4 /tmp/goomn_unsigned.apk /tmp/goomn_aligned.apk && \
    keytool -genkey -v -keystore /tmp/debug.keystore -storepass android -alias androiddebugkey -keypass android -keyalg RSA -keysize 2048 -validity 10000 -dname "CN=Android Debug,O=Android,C=US" && \
    /opt/android/build-tools/33.0.2/apksigner sign --ks /tmp/debug.keystore --ks-pass pass:android --key-pass pass:android --out bin/goomn.apk /tmp/goomn_aligned.apk && \
    rm -rf /tmp/apktool.jar /tmp/apk_decoded /tmp/goomn_unsigned.apk /tmp/goomn_aligned.apk /tmp/debug.keystore'''

        if re.search(old_pattern, content, flags=re.DOTALL):
            content = re.sub(old_pattern, lambda m: new_block, content, flags=re.DOTALL)
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Successfully patched Dockerfile with Apktool versionCode bump to 10019.")
        elif "versionCode: '10019'" in content:
            print("Dockerfile is already patched with the 10019 versionCode bump.")
        else:
            print("Warning: Could not find Apktool block in Dockerfile.")

    # 4. Output Standardized Git Commit Message
    commit_msg = """fix(android): resolve start crash loop and update versionCode scheme
    
Changed the target storage path from the strictly locked public 
directory (/Media/net.basov.goomn) to the app-specific isolated media 
directory (/Android/media/net.basov.goomn) which grants automatic R/W 
access and fixes the EACCES crash loop on Android 11+.

Additionally, adjusted the Apktool sed scripts to bump the versionCode 
using a logical 5-digit scheme (10019 matching version 1.0.19) rather 
than simple increments. This clears Android's "Older Version" warning 
cache gracefully.

Version bumped to 1.0.19"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()