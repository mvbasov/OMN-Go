#!/usr/bin/env python3
"""
OMN-Go patcher – fix header toggle and console UI errors
Bumps version based on current state.
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

    # --- 2. Fix markdown.go: missing import + broken literal (if still broken) ---
    md_path = "backend/markdown.go"
    md = read_file(md_path)
    if '"path/filepath"' not in md:
        md = md.replace(
            'import (\n\t"bytes"',
            'import (\n\t"bytes"\n\t"path/filepath"'
        )
        write_file(md_path, md)
        md = read_file(md_path)

    # Fix the literal newline if still there
    old = '\t\tmetaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"'
    new = '\t\tmetaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"'
    if old in md:
        md = md.replace(old, new)
        write_file(md_path, md)
        md = read_file(md_path)
    else:
        # maybe already fixed? If not found, check if the correct one exists
        if 'metaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"' not in md:
            print("Warning: IS_MARKDOWN block format not recognized.")

    # --- 3. Fix omn-go-core.js: add toggleHeader/updateArrow if missing, fix console UI attachment ---
    js_path = "backend/frontend/html/js/omn-go-core.js"
    js = read_file(js_path)

    # 3a. Add toggleHeader and updateArrow if not present (before window.onload)
    if 'function toggleHeader()' not in js:
        insert_js = (
            "window.toggleHeader = function() {\n"
            "    var header = document.getElementById('hidable_header');\n"
            "    var arrow = document.getElementById('title_arrow');\n"
            "    if (header) {\n"
            "        if (header.classList.contains('hidden')) {\n"
            "            header.classList.remove('hidden');\n"
            "            if (arrow) arrow.textContent = '\\u2212';\n"
            "        } else {\n"
            "            header.classList.add('hidden');\n"
            "            if (arrow) arrow.textContent = '+';\n"
            "        }\n"
            "    }\n"
            "};\n"
            "window.updateArrow = function() {\n"
            "    var header = document.getElementById('hidable_header');\n"
            "    var arrow = document.getElementById('title_arrow');\n"
            "    if (header && arrow) {\n"
            "        arrow.textContent = header.classList.contains('hidden') ? '+' : '\\u2212';\n"
            "    }\n"
            "};\n"
        )
        # Insert before 'window.onload ='
        js = js.replace('window.onload = () => {', insert_js + '\nwindow.onload = () => {')

    # 3b. Fix initConsoleUI: target .header-actions instead of .toolbar (which is now hidden)
    old_target = "document.querySelector('.toolbar').appendChild(consoleBtn);"
    new_target = "var target = document.querySelector('.header-actions'); if (target) target.appendChild(consoleBtn); else { consoleBtn.classList.add('btn-console-main-fixed'); document.body.appendChild(consoleBtn); }"
    if old_target in js:
        js = js.replace(old_target, new_target)

    write_file(js_path, js)

    # --- 4. Commit message ---
    commit_msg = (
        f"fix(js): define toggleHeader/updateArrow; fix console UI attachment\n\n"
        "- Added missing global toggleHeader and updateArrow functions.\n"
        "- Fixed initConsoleUI to append button to .header-actions instead of\n"
        "  non-existent .toolbar (prevents null reference error).\n"
        f"Version bumped to {new_ver}\n"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()