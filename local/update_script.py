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

    # 2. Fix gitignore pattern parsing – use single return value and nil check
    old_block = (
        '\tif data, err := os.ReadFile(gitignorePath); err == nil {\n'
        '\t\tlines := strings.Split(string(data), "\\n")\n'
        '\t\tfor _, line := range lines {\n'
        '\t\t\tline = strings.TrimSpace(line)\n'
        '\t\t\tif line == "" || strings.HasPrefix(line, "#") {\n'
        '\t\t\t\tcontinue\n'
        '\t\t\t}\n'
        '\t\t\tif pattern, err := gitignore.ParsePattern(line, nil); err == nil {\n'
        '\t\t\t\tps = append(ps, pattern)\n'
        '\t\t\t}\n'
        '\t\t}\n'
        '\t}'
    )
    new_block = (
        '\tif data, err := os.ReadFile(gitignorePath); err == nil {\n'
        '\t\tlines := strings.Split(string(data), "\\n")\n'
        '\t\tfor _, line := range lines {\n'
        '\t\t\tline = strings.TrimSpace(line)\n'
        '\t\t\tif line == "" || strings.HasPrefix(line, "#") {\n'
        '\t\t\t\tcontinue\n'
        '\t\t\t}\n'
        '\t\t\tpattern := gitignore.ParsePattern(line, nil)\n'
        '\t\t\tif pattern != nil {\n'
        '\t\t\t\tps = append(ps, pattern)\n'
        '\t\t\t}\n'
        '\t\t}\n'
        '\t}'
    )
    patch_file("backend/git_helper.go", old_block, new_block)

    # 3. Print commit message
    commit_msg = (
        "fix(sync): correct gitignore pattern parsing for Android compatibility\n\n"
        "- Use single return value of gitignore.ParsePattern (no error returned)\n"
        "- Nil‑check pattern before appending to avoid crashes\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()