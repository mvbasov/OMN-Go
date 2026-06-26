#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*.  Raise ValueError if *old* missing."""
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    """'1.3.35' → '1.3.36'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version from backend/version.go and bump it
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle (versionCode and versionName)
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_version_code = int(cur_ver.replace(".", ""))
    new_version_code = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {old_version_code}',
                            f'versionCode {new_version_code}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix compile error: replace git.Storer with storage.Storer in helper function
    old_func = (
        'func buildTreeFromWorktree(dir string, storer git.Storer) (plumbing.Hash, error) {\n'
    )
    new_func = (
        'func buildTreeFromWorktree(dir string, storer storage.Storer) (plumbing.Hash, error) {\n'
    )
    try:
        patch_file("backend/handlers.go", old_func, new_func)
        print("✅ Fixed function signature: git.Storer → storage.Storer")
    except ValueError:
        print("⚠️ Function signature already fixed or not found.")

    # 3. Ensure import of "github.com/go-git/go-git/v5/storage" exists
    handlers_content = read_file("backend/handlers.go")
    # Check if import line already present
    if '"github.com/go-git/go-git/v5/storage"' not in handlers_content:
        # Find the import block and add the new import
        # The import block likely starts with "import (" and ends with ")"
        # We'll insert the line after the last import of go-git packages
        import_target = '\t"github.com/go-git/go-git/v5/plumbing/transport/ssh"'
        if import_target in handlers_content:
            new_import = (
                '\t"github.com/go-git/go-git/v5/plumbing/transport/ssh"\n'
                '\t"github.com/go-git/go-git/v5/storage"'
            )
            patch_file("backend/handlers.go", import_target, new_import)
            print("✅ Added import for github.com/go-git/go-git/v5/storage")
        else:
            print("⚠️ Could not find import target to add storage import; may need manual fix")
    else:
        print("✅ storage import already present")

    # 4. Print the standardised Git commit message
    commit_msg = (
        "fix(sync): correct storage.Storer type in buildTreeFromWorktree\n\n"
        "- Replace undefined git.Storer with storage.Storer from go-git\n"
        "- Add missing import for github.com/go-git/go-git/v5/storage\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()