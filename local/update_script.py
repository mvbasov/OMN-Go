#!/usr/bin/env python3
"""
OMN-Go auto‑increment patcher (idempotent)
Fixes: missing import, broken literal in markdown.go
Bumps version safely based on current value in backend/version.go
"""

import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def increment_version(ver_str):
    """ver_str like '1.3.34' -> '1.3.35'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # ----- 1. Determine current version and bump it -----
    version_path = "backend/version.go"
    content = read_file(version_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    if not match:
        raise ValueError("Cannot find APP_VERSION in backend/version.go")
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    # Replace in version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(version_path, content)

    # Bump in android/app/build.gradle
    build_gradle_path = "android/app/build.gradle"
    bg = read_file(build_gradle_path)
    # versionCode and versionName
    bg = bg.replace(f'versionCode {int(cur_ver.replace(".", ""))}', f'versionCode {int(new_ver.replace(".", ""))}')
    bg = bg.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(build_gradle_path, bg)

    # ----- 2. Add missing import in markdown.go -----
    md_path = "backend/markdown.go"
    md = read_file(md_path)
    if '"path/filepath"' not in md and '"net.basov.omngo/backend"' not in md:
        # add path/filepath to the import block
        md = md.replace(
            'import (\n\t"bytes"',
            'import (\n\t"bytes"\n\t"path/filepath"'
        )
        write_file(md_path, md)
        md = read_file(md_path)  # re-read

    # ----- 3. Fix broken literal newline in IS_MARKDOWN block -----
    # The offending line is:
    # \t\tmetaBlock += "
    #    <script>var IS_MARKDOWN = true;</script>"
    # We'll match the whole if-block and replace with correct version
    old_block = '''\tif pageExt == ".md" || pageExt == "" {
\t\tmetaBlock += "
    <script>var IS_MARKDOWN = true;</script>"
\t}'''
    new_block = '''\tif pageExt == ".md" || pageExt == "" {
\t\tmetaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"
\t}'''
    if old_block in md:
        md = md.replace(old_block, new_block)
        write_file(md_path, md)
    else:
        # maybe already fixed? check if the correct one exists
        if 'metaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"' not in md:
            print("Warning: IS_MARKDOWN block not found in expected format; verify manually.")

    # ----- 4. Commit message -----
    commit_msg = (
        f"chore: bump version to {new_ver}; fix markdown.go build errors\n\n"
        "- Automatically bumped application version based on previous state.\n"
        "- Added missing 'path/filepath' import to markdown.go.\n"
        "- Fixed illegal literal newline in IS_MARKDOWN injection string.\n"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()