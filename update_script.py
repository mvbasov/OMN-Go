import os

def update_application():
    print("Fixing Go 1.25 tool dependencies for gomobile bind (Version 1.0.24)")

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.23"', 'APP_VERSION = "1.0.24"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.23";', 'const APP_VERSION = "1.0.24";'),
        ("android/app/build.gradle", 'versionCode 10023', 'versionCode 10024'),
        ("android/app/build.gradle", 'versionName "1.0.23"', 'versionName "1.0.24"')
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

    # 2. Patch Dockerfile to explicitly register the gobind tool dependency
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
            
        old_cmd = 'RUN mkdir -p android/app/libs && gomobile bind -target=android -androidapi 24 -javapkg net.basov.goomn -o android/app/libs/goomn.aar ./backend'
        new_cmd = 'RUN go get -tool golang.org/x/mobile/cmd/gobind && go mod tidy && mkdir -p android/app/libs && gomobile bind -target=android -androidapi 24 -javapkg net.basov.goomn -o android/app/libs/goomn.aar ./backend'
        
        if old_cmd in content:
            content = content.replace(old_cmd, new_cmd)
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Patched Dockerfile with explicit gobind tool dependency.")
        elif new_cmd in content:
            print("Dockerfile is already patched with the gobind tool dependency.")
        else:
            print("Warning: Could not find gomobile bind command in Dockerfile.")

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(android): add missing gobind tool dependency for Go 1.25
    
Go 1.25 introduces strict tool dependency tracking in go.mod. 
The gomobile bind command failed because golang.org/x/mobile/cmd/gobind 
was not explicitly tracked as a tool in the module dependency graph. 

This patch updates the Dockerfile to run the required `go get -tool` 
directive immediately prior to the bind execution to satisfy the new 
module graph requirements.

Version bumped to 1.0.24"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! Re-run your docker build command.")

if __name__ == "__main__":
    update_application()