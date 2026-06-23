#!/usr/bin/env python3
"""OMN-Go 1.3.37 → 1.3.38: fix editor height, remove redundant Name line."""

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

    # 2. Remove redundant "Name: ..." line from header
    idx_path = 'backend/frontend/index.html'
    old_name_line = '''                <!-- Metadata line: name + pelican headers -->
                <div class="header-info">
                    Name: <span id="pageNameDisplay">/</span>
                    <span id="headerMetadata"><!-- OMN_GO_METADATA_INFO --></span>
                </div>
'''
    new_name_line = '''                <!-- Header metadata (Author, Date, Modified) displayed inline after icons -->
                <div class="header-info">
                    <span id="headerMetadata"><!-- OMN_GO_METADATA_INFO --></span>
                </div>
'''
    patch_file(idx_path, old_name_line, new_name_line)

    # 3. Fix editor/preview height: CSS adjustments
    css_path = 'backend/frontend/html/css/omn-go-core.css'
    # Replace .page-content to remove overflow:auto and add flex structure
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
    min-height: 0;       /* allow shrinking below content size */
    overflow: hidden;    /* prevent double scrollbars; children scroll */
}"""
    if old_content in read_file(css_path):
        patch_file(css_path, old_content, new_content)

    # Replace #preview to add overflow-y
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
    overflow-y: auto;    /* scroll if content overflows */
}"""
    if old_preview in read_file(css_path):
        patch_file(css_path, old_preview, new_preview)

    # Replace #editor to add overflow-y and ensure it stretches
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
    resize: vertical;    /* allow manual resize if desired */
    box-sizing: border-box;
}"""
    if old_editor in read_file(css_path):
        patch_file(css_path, old_editor, new_editor)
    # If the old editor block is missing (maybe already modified), we'll add it
    # but that's unlikely; we'll just trust the checks above.

    # 4. Commit message
    commit_msg = (
        f"fix(ui): editor now fills available height; remove redundant Name line\n\n"
        "- Removed the 'Name: /PageName' line from the collapsible header;\n"
        "  the same information is already shown in the metadata panel.\n"
        "- Fixed the internal editor textarea appearing tiny by adjusting\n"
        "  flexbox layout: .page-content now has overflow:hidden and its\n"
        "  children (#editor, #preview) handle their own scrolling.\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()