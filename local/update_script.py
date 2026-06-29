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
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version and bump
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    new_code = int(new_ver.replace(".", ""))

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_code = int(cur_ver.replace(".", ""))
    gradle = gradle.replace(f"versionCode {old_code}", f"versionCode {new_code}")
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix: check error on blob writer Close to prevent silent failures on Android
    old_close = (
        "\t\t\tif _, err = w.Write(data); err != nil {\n"
        "\t\t\t\treturn plumbing.Hash{}, err\n"
        "\t\t\t}\n"
        "\t\t\tw.Close()\n"
        "\t\t\tblobHash, err := storer.SetEncodedObject(blobObj)"
    )
    new_close = (
        "\t\t\tif _, err = w.Write(data); err != nil {\n"
        "\t\t\t\treturn plumbing.Hash{}, err\n"
        "\t\t\t}\n"
        "\t\t\tif err = w.Close(); err != nil {\n"
        "\t\t\t\treturn plumbing.Hash{}, fmt.Errorf(\"close blob writer: %%v\", err)\n"
        "\t\t\t}\n"
        "\t\t\tblobHash, err := storer.SetEncodedObject(blobObj)"
    )
    patch_file("backend/git_helper.go", old_close, new_close)

    # 3. Print commit message
    commit_msg = (
        "fix(sync): check Close error on blob writer to avoid 'entry not found' on Android\n\n"
        "- Ignored error from blobObj.Writer().Close() could leave objects incomplete\n"
        "- Now returns the Close error, which may reveal Android filesystem quirks\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()