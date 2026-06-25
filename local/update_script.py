#!/usr/bin/env python3
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

def ensure_snippet(path, marker_regex, snippet, idempotent_check):
    """
    Insert *snippet* immediately before the first occurrence of *marker_regex*.
    If *idempotent_check* is already present in the file, skip.
    """
    content = read_file(path)
    if idempotent_check in content:
        return
    match = re.search(marker_regex, content, flags=re.MULTILINE)
    if not match:
        raise ValueError(f"❌ Marker regex not found in {path}")
    insert_pos = match.start()
    new_content = content[:insert_pos] + snippet + "\n" + content[insert_pos:]
    write_file(path, new_content)

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
    cur_vc = int(cur_ver.replace(".", ""))
    new_vc = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {cur_vc}', f'versionCode {new_vc}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Add syncNow() function to omn-go-core.js
    js_path = "backend/frontend/html/js/omn-go-core.js"
    # Marker: the last event listener before file end (pageshow)
    marker = r"^window\.addEventListener\('pageshow', function\(event\) \{"
    snippet = """async function syncNow() {
            const res = await fetch('/api/sync', { method: 'POST' });
            if (res.ok) {
                alert('Sync complete!');
                window.location.reload();
            } else {
                let msg = await res.text();
                alert('Sync failed: ' + msg);
            }
        }"""
    ensure_snippet(js_path, marker, snippet, "async function syncNow")

    commit_msg = (
        "fix(ui): move syncNow() definition to omn-go-core.js\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()