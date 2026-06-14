import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.11"', 'APP_VERSION = "1.0.12"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.11";', 'const APP_VERSION = "1.0.12";')
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
    
    # 2. Patch AndroidManifest.xml to remove API 29+ attributes
    patches = {
        "AndroidManifest.xml": [
            (
                '<application android:label="GoOMN" android:hasFragileUserData="true" android:requestLegacyExternalStorage="true">',
                '<application android:label="GoOMN">'
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

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): fix AAPT attribute parsing error for API 21

Removed 'android:hasFragileUserData' and 'android:requestLegacyExternalStorage' 
from the custom AndroidManifest.xml. These attributes were introduced in 
Android 10 (API 29) and caused the gomobile resource compiler (AAPT) to fail 
when building against -androidapi 21. Removing them allows the APK to compile 
successfully while maintaining compatibility with older devices.

Version bumped to 1.0.12"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()