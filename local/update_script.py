import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.25 EmbedFS & Data Unification...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.24"', 'APP_VERSION = "1.2.25"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.24";', 'const APP_VERSION = "1.2.25";'),
        ("backend/frontend/index.html", "let v = '1.2.24';", "let v = '1.2.25';"),
        ("android/app/build.gradle", "versionCode 10224", "versionCode 10225"),
        ("android/app/build.gradle", 'versionName "1.2.24"', 'versionName "1.2.25"')
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

    # 2. Patch server.go
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # A. Inject io/fs import for fs.Sub()
        if '"io/fs"' not in server_code:
            server_code = server_code.replace('"io"', '"io"\n\t"io/fs"')
            print("  [+] Imported 'io/fs' package.")

        # B. Update the global embed directive
        server_code = re.sub(r'//go:embed frontend/.*', '//go:embed frontend/html', server_code)
        print("  [+] Updated //go:embed directive to target frontend/html.")

        # C. Update explicit staticFS.ReadFile path lookups
        server_code = server_code.replace('"frontend/md/', '"frontend/html/md/')
        server_code = server_code.replace('"frontend/js/', '"frontend/html/js/')
        print("  [+] Rewrote staticFS.ReadFile paths to prepend html/.")

        # D. Replace embed.FS with fs.Sub() to securely root the static HTTP server
        old_fsys = 'fSys, _ := embed.FS(staticFS), error(nil)'
        new_fsys = 'fSys, _ := fs.Sub(staticFS, "frontend/html")'
        if old_fsys in server_code:
            server_code = server_code.replace(old_fsys, new_fsys)
            print("  [+] Configured static HTTP router to use fs.Sub(frontend/html).")

        # E. Fix serveStorageDir bug from V1.2.24 (Route dynamic images/json to data/html/)
        old_storage_dir = 'dirPath := filepath.Join(storageDir, subDir)'
        new_storage_dir = 'dirPath := filepath.Join(storageDir, "html", subDir)'
        if old_storage_dir in server_code:
            server_code = server_code.replace(old_storage_dir, new_storage_dir)
            print("  [+] Patched serveStorageDir to correctly serve dynamic files from data/html/.")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """refactor(storage): unify embedFS and physical data structure

- Updated `//go:embed` directive to bundle the unified `frontend/html/` directory instead of distinct root folders.
- Updated internal `staticFS.ReadFile` paths to correctly target the new `frontend/html/` namespace.
- Utilized `io/fs.Sub()` to safely root the fallback embedded `http.FileServer` to the nested HTML directory.
- Fixed a minor bug from V1.2.24 where dynamic image and JSON serving didn't target the nested `html/` storage directories.
- Bumped application to V1.2.25 (Android 10225)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()