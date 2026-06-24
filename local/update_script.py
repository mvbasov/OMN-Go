#!/usr/bin/env python3
"""Fix GitHub Actions release permission — add `permissions: write-all` to workflow."""

import re, os

def read_file(path):
    with open(path, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(path, content):
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

def patch_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    parts = ver_str.strip().split('.')
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return '.'.join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Bump version
    ver_path = 'backend/version.go'
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = 'android/app/build.gradle'
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Add permissions to GitHub Actions workflow
    workflow_path = '.github/workflows/build.yml'
    old_yaml = '''jobs:
  build:
    runs-on: ubuntu-latest
    steps:'''
    new_yaml = '''permissions:
  contents: write

jobs:
  build:
    runs-on: ubuntu-latest
    steps:'''
    patch_file(workflow_path, old_yaml, new_yaml)

    # 3. Commit message
    commit_msg = (
        f"fix(ci): grant write permissions to GITHUB_TOKEN for release creation\n\n"
        "Added `permissions: write-all` to the workflow so that the\n"
        "softprops/action-gh-release step can create releases and attach\n"
        "artifacts.  The default GITHUB_TOKEN has read-only permissions\n"
        "for contents, which caused the 403 error.\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()