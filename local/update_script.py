#!/usr/bin/env python3
"""OMN-Go 1.3.35 → 1.3.36: fix editor height, metadata toggle, button behavior."""

import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

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
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}', f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # --- 2. Fix index.html: metaToggleBtn toggles metadataPanel instead of header ---
    idx_path = "backend/frontend/index.html"
    old_btn = ('<button id="metaToggleBtn" onclick="var h=document.getElementById(\'hidable_header\');h.classList.toggle(\'hidden\');if(typeof updateArrow===\'function\')updateArrow();" class="btn-metadata-toggle" title="Toggle header"><i class="material-icons">info</i></button>')
    new_btn = ('<button id="metaToggleBtn" onclick="var p=document.getElementById(\'metadataPanel\');p.classList.toggle(\'hidden\');" class="btn-metadata-toggle" title="Toggle metadata"><i class="material-icons">info</i></button>')
    patch_file(idx_path, old_btn, new_btn)

    # --- 3. Fix CSS: editor height and flex layout for .page-content ---
    css_path = "backend/frontend/html/css/omn-go-core.css"
    css = read_file(css_path)
    # Add flex layout to .page-content
    old_page_content = ".page-content {\n    padding: 0.5em;\n    overflow: auto;\n    flex: 1;\n}"
    new_page_content = (".page-content {\n"
                        "    padding: 0.5em;\n"
                        "    overflow: auto;\n"
                        "    flex: 1;\n"
                        "    display: flex;\n"
                        "    flex-direction: column;\n"
                        "}")
    if old_page_content in css:
        css = css.replace(old_page_content, new_page_content)
    else:
        # fallback: append if not found (maybe already modified)
        raise ValueError("Could not find .page-content definition to patch")

    # Make #editor and #preview fill remaining space when visible
    old_editor_css = ".page-content #editor {\n    border: 1px solid #ddd;      /* Subtle border for textarea only */\n}"
    new_editor_css = (".page-content #editor {\n"
                      "    border: 1px solid #ddd;\n"
                      "    flex: 1;\n"
                      "    min-height: 0;\n"
                      "    width: 100%;\n"
                      "}")
    if old_editor_css in css:
        css = css.replace(old_editor_css, new_editor_css)
    else:
        # If the old block is missing, just append the new rules
        css += "\n" + new_editor_css

    old_preview_css = ".page-content #preview {\n    border: none;                /* Remove the old border */\n    background: transparent;\n    padding: 10px 0;\n}"
    new_preview_css = (".page-content #preview {\n"
                       "    border: none;\n"
                       "    background: transparent;\n"
                       "    padding: 10px 0;\n"
                       "    flex: 1;\n"
                       "    min-height: 0;\n"
                       "    width: 100%;\n"
                       "}")
    if old_preview_css in css:
        css = css.replace(old_preview_css, new_preview_css)
    else:
        css += "\n" + new_preview_css

    write_file(css_path, css)

    # --- 4. Commit message ---
    commit_msg = (
        f"fix(ui): restore metadata toggle, fix editor height\n\n"
        "- The info (i) button now toggles the metadata panel again, not the header.\n"
        "- Header folding is only triggered by clicking the page title.\n"
        "- Added flex layout to .page-content so the editor and preview fill\n"
        "  the available vertical space, fixing the tiny textarea problem.\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()