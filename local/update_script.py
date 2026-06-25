#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def increment_version(ver_str):
    """'1.4.3' → '1.4.4'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def ensure_import_in_file(path, import_path, anchor_import):
    """
    Idempotently add *import_path* to the Go import block in *path*,
    placing it immediately before the line containing *anchor_import*.
    If *import_path* already exists, do nothing.
    Raises ValueError if *anchor_import* is not found.
    """
    content = read_file(path)
    if import_path in content:
        return  # already present

    # Find the line with the anchor import (any leading whitespace)
    anchor_regex = r'^(\s*)"' + re.escape(anchor_import) + r'"\s*$'
    match = re.search(anchor_regex, content, flags=re.MULTILINE)
    if not match:
        raise ValueError(f"❌ Anchor import '{anchor_import}' not found in {path}")

    indent = match.group(1)
    new_line = f'{indent}"{import_path}"\n'
    # Insert new import line right before the anchor line
    insert_pos = match.start()
    new_content = content[:insert_pos] + new_line + content[insert_pos:]
    write_file(path, new_content)

def update_application():
    # 1. Bump version
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    if not match:
        raise ValueError("APP_VERSION not found in version.go")
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    cur_vc = int(cur_ver.replace(".", ""))
    new_vc = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {cur_vc}', f'versionCode {new_vc}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Add missing transport import in handlers.go
    ensure_import_in_file(
        "backend/handlers.go",
        "github.com/go-git/go-git/v5/plumbing/transport",
        "github.com/go-git/go-git/v5/plumbing/transport/ssh"
    )

    commit_msg = (
        "fix(build): add missing go-git transport import\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()