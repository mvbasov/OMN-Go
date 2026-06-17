import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.26 Compiler Fix...")

    # 1. Version Bumps (Handles both .24 and .25 states)
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.25"', 'APP_VERSION = "1.2.26"'),
        ("backend/server.go", 'APP_VERSION = "1.2.24"', 'APP_VERSION = "1.2.26"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.25";', 'const APP_VERSION = "1.2.26";'),
        ("backend/frontend/index.html", "let v = '1.2.25';", "let v = '1.2.26';"),
        ("backend/frontend/index.html", "let v = '1.2.24';", "let v = '1.2.26';"),
        ("android/app/build.gradle", "versionCode 10225", "versionCode 10226"),
        ("android/app/build.gradle", "versionCode 10224", "versionCode 10226"),
        ("android/app/build.gradle", 'versionName "1.2.25"', 'versionName "1.2.26"'),
        ("android/app/build.gradle", 'versionName "1.2.24"', 'versionName "1.2.26"')
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

    # 2. Patch server.go using Regex
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Fix A: Ensure 'io/fs' is imported cleanly
        if '"io/fs"' not in server_code:
            server_code = server_code.replace('"io"', '"io"\n\t"io/fs"')
            print("  [+] Imported 'io/fs' package.")

        # Fix B: Use robust RegEx to force the injection of fs.Sub, bypassing any spacing/formatting issues
        old_code = server_code
        server_code = re.sub(
            r'fSys\s*,\s*_\s*:=\s*[^\n]+', 
            r'fSys, _ := fs.Sub(staticFS, "frontend/html")', 
            server_code
        )
        
        if old_code != server_code:
            print("  [+] Regex successfully injected fs.Sub(), resolving the 'unused import' error.")
        else:
            print("  [=] fs.Sub() already present or fSys pattern not found.")

        # Fix C: Re-apply V1.2.25 namespace routing fixes just in case they failed alongside fSys
        server_code = re.sub(r'//go:embed frontend/.*', '//go:embed frontend/html', server_code)
        server_code = server_code.replace('"frontend/md/', '"frontend/html/md/')
        server_code = server_code.replace('"frontend/js/', '"frontend/html/js/')
        
        old_storage_dir = 'dirPath := filepath.Join(storageDir, subDir)'
        new_storage_dir = 'dirPath := filepath.Join(storageDir, "html", subDir)'
        if old_storage_dir in server_code:
            server_code = server_code.replace(old_storage_dir, new_storage_dir)
            print("  [+] Enforced static router mappings to data/html/")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    commit_msg = """fix(compiler): resolve unused io/fs import during EmbedFS unification

- Used strict Regular Expressions to guarantee `fs.Sub()` injection, fixing the `imported and not used` Go compiler panic caused by code-formatting mismatches.
- Ensured `frontend/html` directory mappings were reliably applied.
- Bumped application to V1.2.26 (Android 10226)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()