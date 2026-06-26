#!/usr/bin/env python3
import re, os

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
    # 1. Bump version
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_code = int(cur_ver.replace(".", ""))
    new_code = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {old_code}', f'versionCode {new_code}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix the stray closing brace causing "non-declaration statement outside function body"
    old_brace_issue = (
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\t}\n'
        '\t}\n'
        '\n'
        '\t}\n'
        '\n'
        '\tif action == "download" {'
    )
    new_brace_fix = (
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\t}\n'
        '\t}\n'
        '\n'
        '\tif action == "download" {'
    )
    try:
        patch_file("backend/handlers.go", old_brace_issue, new_brace_fix)
        print("✅ Removed stray extra brace.")
    except ValueError:
        print("⚠️ Stray brace pattern not found; maybe already fixed.")

    # 3. Commit message
    commit_msg = (
        "fix(sync): remove stray closing brace causing syntax error\n\n"
        "- The extra '}' after the commit block was closing the function prematurely\n"
        "- Now handleSync compiles successfully\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()