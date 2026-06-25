#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*. Raise ValueError if *old* missing."""
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def safe_patch(path, old, new):
    """Idempotent patch: apply only if old present, skip if new already there."""
    content = read_file(path)
    if old in content:
        patch_file(path, old, new)
    elif new not in content:
        raise ValueError(f"❌ Patch already partially applied? Neither old nor new string found in {path}")

def increment_version(ver_str):
    """'1.4.3' → '1.4.4'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

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

    # 2. Make sync_ssh_key relative to storageDir when not absolute
    old_line = "\t\tkeyBytes, err := os.ReadFile(appConfig.SyncSSHKey)"
    new_block = (
        "\t\tkeyPath := appConfig.SyncSSHKey\n"
        "\t\tif !filepath.IsAbs(keyPath) {\n"
        "\t\t\tkeyPath = filepath.Join(storageDir, keyPath)\n"
        "\t\t}\n"
        "\t\tkeyBytes, err := os.ReadFile(keyPath)"
    )
    safe_patch("backend/handlers.go", old_line, new_block)

    commit_msg = (
        "feat(sync): resolve sync_ssh_key relative to storageDir when not absolute\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()