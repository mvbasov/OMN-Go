#!/usr/bin/env python3
"""OMN-Go 1.3.39 → 1.3.40: remove unused metaInfo variable from markdown.go."""

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

    # 2. Remove the unused metaInfo variable
    old_block = '''\t// Metadata info now shown only in the metadata panel (via meta tags);
\t// the inline header line has been removed from the template.
\tmetaInfo := ""
\tlayout = strings.ReplaceAll(layout, "<!-- OMN_GO_TAGS -->", tagsHTML)'''
    new_block = '''\tlayout = strings.ReplaceAll(layout, "<!-- OMN_GO_TAGS -->", tagsHTML)'''
    patch_file('backend/markdown.go', old_block, new_block)

    # 3. Commit message
    commit_msg = (
        f"fix(build): remove unused metaInfo variable causing compilation error\n\n"
        "The variable was left behind when the OMN_GO_METADATA_INFO placeholder\n"
        "injection was removed.  Dropping it entirely cleans up the build.\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()