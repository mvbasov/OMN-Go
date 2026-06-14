import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.13"', 'APP_VERSION = "1.0.14"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.13";', 'const APP_VERSION = "1.0.14";')
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
    
    # 2. Delete custom AndroidManifest.xml to avoid gomobile's destructive XML parser
    if os.path.exists("AndroidManifest.xml"):
        os.remove("AndroidManifest.xml")
        print("Deleted custom AndroidManifest.xml to force gomobile to use the patched template.")

    # 3. Patch Dockerfile using resilient regex to strictly patch gomobile's text template
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        # Regex safely targets the entire git clone & sed block regardless of line endings
        old_pattern = r'RUN git clone --depth 1 https://github\.com/golang/mobile\.git /tmp/mobile && .*?gomobile init'
        
        # New block targets the default manifest template string directly
        new_block = r'''RUN git clone --depth 1 https://github.com/golang/mobile.git /tmp/mobile && \
    sed -i 's/<uses-sdk.*/<uses-sdk android:minSdkVersion="21" android:targetSdkVersion="34"\/>/g' /tmp/mobile/cmd/gomobile/build_androidapp.go && \
    sed -i 's/uses-permission.*INTERNET.*/&\n    <uses-permission android:name="android.permission.READ_EXTERNAL_STORAGE" \/>\n    <uses-permission android:name="android.permission.WRITE_EXTERNAL_STORAGE" \/>\n    <uses-permission android:name="android.permission.MANAGE_EXTERNAL_STORAGE" \/>/g' /tmp/mobile/cmd/gomobile/build_androidapp.go && \
    cd /tmp/mobile/cmd/gomobile && \
    go install . && \
    gomobile init'''
        
        if re.search(old_pattern, content, flags=re.DOTALL):
            content = re.sub(old_pattern, lambda m: new_block, content, flags=re.DOTALL)
            with open("Dockerfile", "w") as f:
                f.write(content)
        elif new_block in content:
            pass # Idempotency: Already patched
        else:
            print("Warning: Could not find gomobile git clone block in Dockerfile.")

    # 4. Output Standardized Git Commit Message
    commit_msg = """build(android): bypass gomobile XML parser to enforce target SDK 34
    
Gomobile's internal XML parser permanently deletes targetSdkVersion from any 
custom AndroidManifest.xml because its internal struct only maps minSdkVersion. 
To bypass this destructive behavior, this patch deletes the local AndroidManifest.xml
and directly modifies gomobile's internal text template via sed before compilation.
This guarantees both SDK limits and storage permissions are permanently hardcoded
into the final APK.

Version bumped to 1.0.14"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()