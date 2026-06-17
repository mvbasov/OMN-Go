import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.24 Directory Restructuring...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.23"', 'APP_VERSION = "1.2.24"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.23";', 'const APP_VERSION = "1.2.24";'),
        ("backend/frontend/index.html", "let v = '1.2.23';", "let v = '1.2.24';"),
        ("android/app/build.gradle", "versionCode 10223", "versionCode 10224"),
        ("android/app/build.gradle", 'versionName "1.2.23"', 'versionName "1.2.24"')
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

    # 2. Patch server.go to restructure directories
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        replacements = [
            # A. Add automated directory migration to initStorage()
            (
                '\t// Migrate legacy root md files recursively\n\tfiles, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))\n\tfor _, f := range files {\n\t\tos.Rename(f, filepath.Join(mdDir, filepath.Base(f)))\n\t}',
                '\t// Migrate legacy root md files recursively\n\tfiles, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))\n\tfor _, f := range files {\n\t\tos.Rename(f, filepath.Join(mdDir, filepath.Base(f)))\n\t}\n\n\t// Migrate static directories inside html/\n\tdirsToMove := []string{"images", "user_json", "css", "js", "json", "fonts"}\n\tfor _, d := range dirsToMove {\n\t\toldPath := filepath.Join(storageDir, d)\n\t\tnewPath := filepath.Join(htmlDir, d)\n\t\tif stat, err := os.Stat(oldPath); err == nil && stat.IsDir() {\n\t\t\tos.Rename(oldPath, newPath)\n\t\t}\n\t}'
            ),
            # B. Move handleUpload output to html/images
            (
                '\timgDir := filepath.Join(storageDir, "images")\n\tos.MkdirAll(imgDir, 0755)',
                '\timgDir := filepath.Join(storageDir, "html", "images")\n\tos.MkdirAll(imgDir, 0755)'
            ),
            # C. Move handleUploadJSON output to html/user_json
            (
                '\tjsonDir := filepath.Join(storageDir, "user_json")\n\tos.MkdirAll(jsonDir, 0755)',
                '\tjsonDir := filepath.Join(storageDir, "html", "user_json")\n\tos.MkdirAll(jsonDir, 0755)'
            ),
            # D. Redirect handleGetNote fallback to html/
            (
                '\t} else {\n\t\tpath = filepath.Join(storageDir, filepath.Clean(name))\n\t\tdata, err = os.ReadFile(path)',
                '\t} else {\n\t\tpath = filepath.Join(storageDir, "html", filepath.Clean(name))\n\t\tdata, err = os.ReadFile(path)'
            ),
            # E. Redirect handleSaveNote fallback to html/
            (
                '\t} else {\n\t\tpath = filepath.Join(storageDir, filepath.Clean(name))\n\t\tos.MkdirAll(filepath.Dir(path), 0755)',
                '\t} else {\n\t\tpath = filepath.Join(storageDir, "html", filepath.Clean(name))\n\t\tos.MkdirAll(filepath.Dir(path), 0755)'
            ),
            # F. Remap serveFrontend static assets to html/
            (
                '\t// Priority 1: User\'s Local Storage Directory (data/css, data/js, etc)\n\tfilePath := filepath.Join(storageDir, filepath.Clean(path))',
                '\t// Priority 1: User\'s Local Storage Directory (data/html/css, data/html/js, etc)\n\tfilePath := filepath.Join(storageDir, "html", filepath.Clean(path))'
            )
        ]

        for old_str, new_str in replacements:
            if old_str in server_code:
                server_code = server_code.replace(old_str, new_str)
                print("  [+] Successfully applied a directory routing patch.")
            else:
                print(f"  [-] Could not find target string: {old_str[:50]}...")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """feat(storage): restructure data directory keeping md and config at root

- Relocated static asset directories (`css`, `js`, `images`, `json`, `user_json`, `fonts`) strictly inside the `data/html/` subdirectory to declutter the root workspace.
- Added automated startup migration logic to silently move existing root folders into `html/` so users lose zero local modifications.
- Updated URL router and API saving/loading endpoints to dynamically point all non-Markdown file requests into the `html/` directory mapping.
- Bumped application to V1.2.24 (Android 10224)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()