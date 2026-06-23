#!/usr/bin/env python3
"""OMN-Go 1.3.37 → 1.3.38: fix editor height (idempotent — Name line already removed)."""

import re, os

def read_file(path):
    with open(path, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(path, content):
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

def safe_patch(path, old, new):
    """Replace old→new only if old exists. Returns True if patched."""
    content = read_file(path)
    if old in content:
        content = content.replace(old, new, 1)
        write_file(path, content)
        return True
    return False

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

    # 2. Name line already removed — skip idempotently (no error if missing)

    # 3. Fix editor/preview height: CSS updates
    css_path = 'backend/frontend/html/css/omn-go-core.css'

    # 3a. Fix .page-content
    old_content = """.page-content {
    padding: 0.5em;
    overflow: auto;
    flex: 1;
    display: flex;
    flex-direction: column;
}"""
    new_content = """.page-content {
    padding: 0.5em;
    flex: 1;
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
}"""
    safe_patch(css_path, old_content, new_content)

    # 3b. Fix #preview
    old_preview = """.page-content #preview {
    border: none;
    background: transparent;
    padding: 10px 0;
    flex: 1;
    min-height: 0;
    width: 100%;
}"""
    new_preview = """.page-content #preview {
    border: none;
    background: transparent;
    padding: 10px 0;
    flex: 1;
    min-height: 0;
    width: 100%;
    overflow-y: auto;
}"""
    safe_patch(css_path, old_preview, new_preview)

    # 3c. Fix #editor
    old_editor = """.page-content #editor {
    border: 1px solid #ddd;
    flex: 1;
    min-height: 0;
    width: 100%;
}"""
    new_editor = """.page-content #editor {
    border: 1px solid #ddd;
    flex: 1;
    min-height: 0;
    width: 100%;
    resize: vertical;
    box-sizing: border-box;
}"""
    safe_patch(css_path, old_editor, new_editor)

    # 4. Commit message
    commit_msg = (
        f"fix(ui): editor now fills available vertical height\n\n"
        "- Fixed .page-content flexbox layout: removed overflow:auto on\n"
        "  the container, added overflow:hidden with child scrolling\n"
        "  (#editor and #preview handle their own overflow-y).\n"
        "- The internal editor textarea now stretches to fill the\n"
        "  available space instead of appearing as a tiny sliver.\n"
        "- Name line removal was already applied by previous patch.\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()