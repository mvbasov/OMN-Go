import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.2"', 'APP_VERSION = "1.0.3"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.2";', 'const APP_VERSION = "1.0.3";')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "go.mod": [
            (
                "go 1.22",
                "go 1.25"
            ),
            (
                "require golang.org/x/mobile v0.0.0-20231127183840-76ac68780225",
                ""
            )
        ],
        "Dockerfile": [
            (
                "FROM golang:1.22-bookworm AS builder",
                "FROM golang:1.25-bookworm AS builder"
            ),
            (
                "RUN git clone https://github.com/golang/mobile.git /tmp/mobile && cd /tmp/mobile && git checkout 76ac68780225 && cd cmd/gomobile && go install . && gomobile init",
                "RUN go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init"
            ),
            (
                "COPY go.mod ./\nRUN go mod tidy && go mod download\n\n# STAGE 3: Build & Pack\nCOPY . .",
                "COPY go.mod ./\nRUN go mod download || true\n\n# STAGE 3: Build & Pack\nCOPY . .\nRUN go get golang.org/x/mobile@latest && go mod tidy"
            )
        ]
    }

    # Execute version updates safely (idempotent)
    for filename, old_str, new_str in version_replacements:
        if os.path.exists(filename):
            with open(filename, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_str in content:
                content = content.replace(old_str, new_str)
                with open(filename, 'w', encoding='utf-8') as f:
                    f.write(content)
                    
    # Execute patching sequentially
    for filename, file_patches in patches.items():
        if not os.path.exists(filename):
            print(f"Skipping {filename}: File not found.")
            continue
            
        with open(filename, 'r', encoding='utf-8') as f:
            content = f.read()
        
        for idx, (old_str, new_str) in enumerate(file_patches):
            if old_str in content:
                content = content.replace(old_str, new_str)
            elif new_str in content:
                print(f"[{filename}] Patch target #{idx} is already applied. Skipping.")
            else:
                raise ValueError(f"Could not find patch target #{idx} in {filename}:\n{old_str[:100]}...")
                
        with open(filename, 'w', encoding='utf-8') as f:
            f.write(content)
            
        print(f"Patched: {filename}")

    # 3. Output Standardized Git Commit Message
    commit_msg = """build(core): bump go version to 1.25 in builder and cleanly fetch gomobile latest

Version bumped to 1.0.3"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()