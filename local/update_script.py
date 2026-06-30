#!/usr/bin/env python3
import re, os, sys

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
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
    # 1. Try to apply the patch first (idempotency: fail if already applied)
    old_block = (
        "\t\t// Skip ignored files\n"
        "\t\tif matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {\n"
        "\t\t\tlog.Printf(\"[sync] Ignoring %s (matches .gitignore)\", name)\n"
        "\t\t\tcontinue\n"
        "\t\t}"
    )
    new_block = (
        "\t\t// Exclude root config.json explicitly\n"
        "\t\tif name == \"config.json\" {\n"
        "\t\t\tlog.Printf(\"[sync] Ignoring root config.json (preserve locally)\")\n"
        "\t\t\tcontinue\n"
        "\t\t}\n"
        "\t\t// Skip ignored files\n"
        "\t\tif matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {\n"
        "\t\t\tlog.Printf(\"[sync] Ignoring %s (matches .gitignore)\", name)\n"
        "\t\t\tcontinue\n"
        "\t\t}"
    )

    try:
        patch_file("backend/git_helper.go", old_block, new_block)
    except ValueError as e:
        print("Patch already applied, nothing to do.")
        sys.exit(0)

    # 2. Auto‑detect current version from backend/version.go and bump it
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
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 3. Print commit message
    commit_msg = (
        "fix(sync): prevent root config.json from being staged\n\n"
        "- Added hard exclusion in commitLocalChanges for config.json in worktree root\n"
        "- Prevents accidental tracking and subsequent overwriting during pull\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()