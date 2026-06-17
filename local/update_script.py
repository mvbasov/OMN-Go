import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.23 Metadata Filename Display Update...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.22"', 'APP_VERSION = "1.2.23"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.22";', 'const APP_VERSION = "1.2.23";'),
        ("backend/frontend/index.html", "let v = '1.2.22';", "let v = '1.2.23';"),
        ("android/app/build.gradle", "versionCode 10222", "versionCode 10223"),
        ("android/app/build.gradle", 'versionName "1.2.22"', 'versionName "1.2.23"')
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

    # 2. Patch server.go to inject filename into the metadata display
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Find the point where the headers array is joined into the metadataStr
        old_meta_join = 'metadataStr := strings.Join(headers, "\\n")'
        
        # Prepend the file name format using fmt.Sprintf
        new_meta_join = 'metadataStr := fmt.Sprintf("File: %s.md\\n%s", name, strings.Join(headers, "\\n"))'
        
        if old_meta_join in server_code:
            server_code = server_code.replace(old_meta_join, new_meta_join)
            print("  [+] Appended dynamic filename display to the metadata panel compiler.")
        else:
            print("  [=] Filename display logic already patched or target string missing.")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """feat(ui): display current filename in metadata overlay

- Upgraded `compilePage` in `server.go` to dynamically prepend `File: [name].md` to the metadata strings.
- This safely exposes the current physical file location inside the read-only Metadata UI panel without polluting the raw Markdown source code payload.
- Bumped application to V1.2.23 (Android 10223)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()