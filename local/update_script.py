import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.14 Dockerfile & Builder Clean Up...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.13"', 'APP_VERSION = "1.2.14"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.13";', 'const APP_VERSION = "1.2.14";'),
        ("backend/frontend/index.html", "let v = '1.2.13';", "let v = '1.2.14';"),
        ("android/app/build.gradle", "versionCode 10213", "versionCode 10214"),
        ("android/app/build.gradle", 'versionName "1.2.13"', 'versionName "1.2.14"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Forcefully rename the .aar output in Dockerfile
    dockerfile = "Dockerfile"
    if os.path.exists(dockerfile):
        with open(dockerfile, "r", encoding="utf-8") as f:
            d_content = f.read()
        
        # Catch both potential states of the gomobile bind command
        if "goomn.aar" in d_content:
            d_content = d_content.replace("goomn.aar", "omngo.aar")
            with open(dockerfile, "w", encoding="utf-8") as f:
                f.write(d_content)
            print(f"  [+] Purged 'goomn.aar' and replaced with 'omngo.aar' in {dockerfile}")
        else:
            print(f"  [=] Output target already clean in {dockerfile}")
    else:
        print(f"  [-] Could not find {dockerfile}")

    # 3. Rename goomn-builder to omn-go-builder across common documentation and script files
    docs_and_scripts = ["README.md", "build.sh", "generate_context.sh", "setup_project.py"]
    for filepath in docs_and_scripts:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if "goomn-builder" in content:
                content = content.replace("goomn-builder", "omn-go-builder")
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Renamed 'goomn-builder' to 'omn-go-builder' in {filepath}")

    commit_msg = """build(docker): enforce omngo.aar and builder naming convention

- Forcefully replaced remaining `goomn.aar` artifacts inside the Dockerfile with `omngo.aar`.
- Replaced references to `goomn-builder` with `omn-go-builder` across project documentation and scripts.
- Bumped application to V1.2.14 (Android 10214)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()