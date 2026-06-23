#!/usr/bin/env python3
"""
OMN-Go auto‑increment patcher – fix broken literal in markdown.go
"""

import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # --- 1. Bump version ---
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    if not match:
        raise ValueError("Cannot find APP_VERSION in backend/version.go")
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}', f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # --- 2. Add missing import if needed ---
    md_path = "backend/markdown.go"
    md = read_file(md_path)
    if '"path/filepath"' not in md:
        md = md.replace(
            'import (\n\t"bytes"',
            'import (\n\t"bytes"\n\t"path/filepath"'
        )
        write_file(md_path, md)
        md = read_file(md_path)

    # --- 3. Fix the broken literal newline ---
    # The exact broken line (as bytes in file):
    # \t\tmetaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"
    # We replace it with:
    # \t\tmetaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"
    old = '\t\tmetaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"'
    new = '\t\tmetaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"'
    if old in md:
        md = md.replace(old, new)
        write_file(md_path, md)
    else:
        # Already fixed? Check if the correct version exists
        if 'metaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"' not in md:
            print("Warning: IS_MARKDOWN block not found as expected; please verify manually.")

    # --- 4. Commit message ---
    commit_msg = (
        f"chore: bump version to {new_ver}; fix literal newline in markdown.go\n\n"
        "Corrected an illegal literal newline inside a Go string literal by\n"
        "using the \\n escape sequence.  This resolves the 'newline in string'\n"
        "compilation error."
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()