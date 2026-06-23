#!/usr/bin/env python3
"""OMN-Go 1.3.40 → 1.3.41: fix versionCode computation, fix save button never shown."""

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
    """'1.3.40' → '1.3.41'"""
    parts = ver_str.strip().split('.')
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return '.'.join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def version_name_to_code(ver_str):
    """'1.3.41' → 10341"""
    major, minor, patch = map(int, ver_str.split('.'))
    return major * 10000 + minor * 100 + patch

def update_application():
    # 1. Detect current version
    ver_path = 'backend/version.go'
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    new_code = version_name_to_code(new_ver)

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle
    gradle_path = 'android/app/build.gradle'
    gradle = read_file(gradle_path)

    # Patch versionCode: find the existing line and replace with correct new code
    # The previous versionCode may have been wrongly computed (e.g. 1340 instead of 10340)
    # We'll use a regex to replace any versionCode number with the correct one.
    gradle = re.sub(r'versionCode \d+', f'versionCode {new_code}', gradle)
    # Patch versionName similarly
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix save button CSS: replace !important rule with ID + class selector
    css_path = 'backend/frontend/html/css/omn-go-core.css'
    old_save_css = '''.btn-save-note {
    display: none !important;        /* hidden in view mode, JS sets inline style in edit mode */
    background: #28a745;
    color: white;
    border: none;
}'''
    new_save_css = '''#saveBtn.btn-save-note {
    display: none;                   /* hidden in view mode; inline style takes over in edit */
    background: #28a745;
    color: white;
    border: none;
}'''
    patch_file(css_path, old_save_css, new_save_css)

    # 3. Commit message
    commit_msg = (
        f"fix(android,css): correct versionCode computation; restore save button visibility\n\n"
        "- versionCode now correctly computed as major*10000+minor*100+patch\n"
        "  (was using raw concatenation, e.g. '1340' instead of '10340').\n"
        "- Fixed CSS rule that permanently hid the save button by using\n"
        "  !important, which blocked JavaScript inline style overrides.\n"
        "  Now uses #saveBtn.btn-save-note with normal specificity, allowing\n"
        "  the inline display:block set by toggleMode() to win.\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()