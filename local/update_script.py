#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def safe_patch_file(path, old, new):
    content = read_file(path)
    if old in content:
        content = content.replace(old, new, 1)
        write_file(path, content)
        print(f"Patched {path}")
    elif new in content:
        print(f"Already patched: {path}")
    else:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Version bump
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix potential null reference in previewAndCommit
    sse_path = "backend/frontend/html/js/omn-go-sse.js"
    sse_content = read_file(sse_path)

    old_block = "            if (files.length === 0) {"
    new_block = "            if (!files || files.length === 0) {"
    if old_block in sse_content:
        sse_content = sse_content.replace(old_block, new_block, 1)
        write_file(sse_path, sse_content)
        print("Added null check for files array")
    else:
        print("Null check already present or block not found (already safe)")

    # Also add null check for commitFileList in commitAndUpload
    old_modal_show = "            document.getElementById('commitFileList').textContent = files.join('\\n');"
    new_modal_show = "            var listEl = document.getElementById('commitFileList');\n            if (listEl) listEl.textContent = files.join('\\n');"
    if old_modal_show in sse_content:
        sse_content = sse_content.replace(old_modal_show, new_modal_show)
        write_file(sse_path, sse_content)
        print("Added null check for commitFileList")
    else:
        print("commitFileList null check already present or code changed")

    commit_msg = (
        "fix(ui): add null checks for sync preview on Android\n\n"
        "- Prevent TypeError when file list is empty or modal element missing\n"
        "Recompile the binary to apply frontend changes.\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\n⚠️  IMPORTANT: Rebuild the Go binary to embed the updated frontend files.")

if __name__ == "__main__":
    update_application()