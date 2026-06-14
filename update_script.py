import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.15"', 'APP_VERSION = "1.0.16"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.15";', 'const APP_VERSION = "1.0.16";')
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

    # 2. Patch Dockerfile to replace AAPT flags wrapper with XML file interceptor
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        # Regex safely targets the 1.0.15 AAPT flag wrapper
        old_pattern = r"# Intercept AAPT to guarantee SDK Target 34 at the lowest compiler level\nRUN printf '#!/bin/bash.*?cp /tmp/aapt_wrapper\.sh /opt/android/build-tools/33\.0\.2/aapt2"
        
        # New block intercepts the actual intermediate AndroidManifest.xml file
        new_block = r'''# Intercept AAPT to guarantee SDK Target 34 at the lowest compiler level
RUN printf '#!/bin/bash\n\
MANIFEST_PATH=""\n\
get_next=0\n\
for arg in "$@"; do\n\
    if [ $get_next -eq 1 ]; then\n\
        MANIFEST_PATH="$arg"\n\
        get_next=0\n\
    elif [[ "$arg" == "-M" ]] || [[ "$arg" == "--manifest" ]]; then\n\
        get_next=1\n\
    elif [[ "$arg" == --manifest=* ]]; then\n\
        MANIFEST_PATH="${arg#*=}"\n\
    fi\n\
done\n\
if [ -n "$MANIFEST_PATH" ] && [ -f "$MANIFEST_PATH" ]; then\n\
    sed -i '\''s/<uses-sdk[^>]*>/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\\/>/g'\'' "$MANIFEST_PATH"\n\
    if ! grep -q "uses-sdk" "$MANIFEST_PATH"; then\n\
        sed -i '\''s/<application/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\\/> <application/g'\'' "$MANIFEST_PATH"\n\
    fi\n\
fi\n\
COMMAND_NAME=$(basename "$0")\n\
exec /opt/android/build-tools/33.0.2/${COMMAND_NAME}.real "$@"\n' > /tmp/aapt_wrapper.sh && \
    chmod +x /tmp/aapt_wrapper.sh && \
    mv /opt/android/build-tools/33.0.2/aapt /opt/android/build-tools/33.0.2/aapt.real && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt && \
    mv /opt/android/build-tools/33.0.2/aapt2 /opt/android/build-tools/33.0.2/aapt2.real && \
    cp /tmp/aapt_wrapper.sh /opt/android/build-tools/33.0.2/aapt2'''
        
        if re.search(old_pattern, content, flags=re.DOTALL):
            content = re.sub(old_pattern, lambda m: new_block, content, flags=re.DOTALL)
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Successfully patched Dockerfile with XML-intercepting AAPT wrapper.")
        elif "MANIFEST_PATH" in content:
            print("Dockerfile is already patched with the XML-intercepting AAPT wrapper.")
        else:
            print("Warning: Could not find old AAPT wrapper block in Dockerfile.")

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): inject target SDK directly into intermediate XML
    
AAPT ignores command-line flags (--min-sdk-version) if the AndroidManifest.xml
already contains explicit declarations. Because gomobile forces a default 
<uses-sdk android:minSdkVersion="16"/> into its intermediate merged manifest, 
our previous command-line wrapper was being ignored by AAPT.

This patch refines the OS-level AAPT wrapper. Instead of appending flags, the 
wrapper intercepts the -M /path/to/AndroidManifest.xml argument, reads the 
intermediate XML file that gomobile generated, and directly injects 
targetSdkVersion="34" into it before executing AAPT. This guarantees that 
AAPT receives exactly the right attributes.

Version bumped to 1.0.16"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()