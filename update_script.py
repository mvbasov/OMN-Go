import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.12"', 'APP_VERSION = "1.0.13"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.12";', 'const APP_VERSION = "1.0.13";')
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
    
    # 2. Patch Dockerfile to forcibly inject targetSdkVersion into gomobile's AST engine
    patches = {
        "Dockerfile": [
            (
                'RUN git clone --depth 1 https://github.com/golang/mobile.git /tmp/mobile && \\\n    sed -i \'s/targetSdkVersion="29"/targetSdkVersion="34"/g\' /tmp/mobile/cmd/gomobile/build_androidapp.go && \\\n    cd /tmp/mobile/cmd/gomobile && \\\n    go install . && \\\n    gomobile init',
                'RUN git clone --depth 1 https://github.com/golang/mobile.git /tmp/mobile && \\\n    sed -i \'s/targetSdkVersion="29"/targetSdkVersion="34"/g\' /tmp/mobile/cmd/gomobile/build_androidapp.go && \\\n    sed -i \'s/minSdkVersion="{{.MinSDK}}"/minSdkVersion="21"/g\' /tmp/mobile/cmd/gomobile/build_androidapp.go && \\\n    sed -i -E \'s/(Name: ([a-zA-Z.]+)\\{Space: nsAndroid, Local: "minSdkVersion"\\}, Value: )[^}]+(\\},)/\\1"21"\\3\\n\\t\\t\\t\\t{Name: \\2{Space: nsAndroid, Local: "targetSdkVersion"}, Value: "34"},/g\' /tmp/mobile/cmd/gomobile/build_androidapp.go && \\\n    cd /tmp/mobile/cmd/gomobile && \\\n    go install . && \\\n    gomobile init'
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
    commit_msg = """build(android): brutally enforce min/target SDK via AST sed injection

Because gomobile omits the targetSdkVersion attribute when dynamically injecting 
limits into a custom AndroidManifest.xml, Android natively defaults the target 
SDK down to the fallback minimum SDK (resulting in 16). This patch extends the 
previous gomobile sed hack to directly intercept its XML AST generator and 
explicitly hardcode both minSdkVersion="21" and targetSdkVersion="34" into 
the final merged manifest structure.

Version bumped to 1.0.13"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()