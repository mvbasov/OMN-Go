#!/usr/bin/env python3
"""OMN-Go patcher – fix toggleHeader not defined and console UI crash by wrapping
   immediate DOM queries in DOMContentLoaded.  Remove duplicate function defs."""

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

    # --- 2. Fix markdown.go (missing import / broken literal) if still broken ---
    md_path = "backend/markdown.go"
    md = read_file(md_path)
    if '"path/filepath"' not in md:
        md = md.replace(
            'import (\n\t"bytes"',
            'import (\n\t"bytes"\n\t"path/filepath"'
        )
        write_file(md_path, md)
        md = read_file(md_path)
    old_broken = '\t\tmetaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"'
    new_fixed  = '\t\tmetaBlock += "\\n    <script>var IS_MARKDOWN = true;</script>"'
    if old_broken in md:
        md = md.replace(old_broken, new_fixed)
        write_file(md_path, md)

    # --- 3. Fix omn-go-core.js ---
    js_path = "backend/frontend/html/js/omn-go-core.js"
    js = read_file(js_path)

    # 3a. Remove duplicate toggleHeader / updateArrow definitions
    #     Keep only the last (cleaner) pair, remove the first pair.
    first_dup = (
        "window.toggleHeader = function() {\n"
        "            var header = document.getElementById('hidable_header');\n"
        "            var arrow = document.getElementById('title_arrow');\n"
        "            if (header) {\n"
        "                if (header.classList.contains('hidden')) {\n"
        "                    header.classList.remove('hidden');\n"
        "                    if (arrow) arrow.textContent = '\u2212';\n"
        "                } else {\n"
        "                    header.classList.add('hidden');\n"
        "                    if (arrow) arrow.textContent = '+';\n"
        "                }\n"
        "            }\n"
        "        };\n"
        "        window.updateArrow = function() {\n"
        "            var header = document.getElementById('hidable_header');\n"
        "            var arrow = document.getElementById('title_arrow');\n"
        "            if (header && arrow) {\n"
        "                arrow.textContent = header.classList.contains('hidden') ? '+' : '\u2212';\n"
        "            }\n"
        "        };\n"
    )
    if first_dup in js:
        js = js.replace(first_dup, "")
        write_file(js_path, js)
        js = read_file(js_path)

    # 3b. Wrap immediate DOM queries (preview click, editor drag/drop) in a
    #     queued setup that runs after the elements exist.
    old_immediate_code = """        // Intercept Markdown links for standard browser-side redirects
        document.getElementById('preview').addEventListener('click', (e) => {"""
    new_deferred_code = """        // Intercept Markdown links for standard browser-side redirects
        function setupPreviewLinkInterceptor() {
            var preview = document.getElementById('preview');
            if (!preview) return;
            preview.addEventListener('click', (e) => {"""
    if old_immediate_code in js:
        js = js.replace(old_immediate_code, new_deferred_code)
        # Now find the closing of that block and add the deferred call
        # The original ends with:
        #             }
        #         }
        #     }
        # });
        # We need to close the setup function and call it on DOMContentLoaded
        old_closing = """                }
            }
        });"""
        new_closing = """                }
            }
        });
        }
        document.addEventListener('DOMContentLoaded', setupPreviewLinkInterceptor);"""
        if old_closing in js:
            js = js.replace(old_closing, new_closing, 1)  # only first occurrence
        write_file(js_path, js)
        js = read_file(js_path)

    # 3c. Wrap the editor drag/drop setup (also runs immediately)
    old_editor_setup = """        // Image Drag & Drop
        const editor = document.getElementById('editor');
        editor.addEventListener('dragover', e => e.preventDefault());
        editor.addEventListener('drop', async e => {"""
    new_editor_setup = """        // Image Drag & Drop
        function setupEditorDragDrop() {
            const editor = document.getElementById('editor');
            if (!editor) return;
            editor.addEventListener('dragover', e => e.preventDefault());
            editor.addEventListener('drop', async e => {"""
    if old_editor_setup in js:
        js = js.replace(old_editor_setup, new_editor_setup)
        # Find the closing of the editor drop handler and add the deferred call
        # The original ends with:
        #             }
        #         }
        #     }
        # });
        # Find the SECOND occurrence (after the editor drop handler)
        old_editor_closing = """                }
            }
        });"""
        # We need to be more specific.  The editor drop handler is the SECOND
        # occurrence of this pattern.  Let's use a marker.
        marker = """                    editor.dispatchEvent(new Event('input'));
                }
            }
        });"""
        new_marker = """                    editor.dispatchEvent(new Event('input'));
                }
            }
        });
        }
        document.addEventListener('DOMContentLoaded', setupEditorDragDrop);"""
        if marker in js:
            js = js.replace(marker, new_marker)
        write_file(js_path, js)
        js = read_file(js_path)

    # 3d. Make initConsoleUI safe even when .header-actions or body is null
    old_init_ui_frag = (
        "var target = document.querySelector('.header-actions'); "
        "if (target) target.appendChild(consoleBtn); "
        "else { consoleBtn.classList.add('btn-console-main-fixed'); document.body.appendChild(consoleBtn); }"
    )
    new_init_ui_frag = (
        "var target = document.querySelector('.header-actions'); "
        "if (target) { target.appendChild(consoleBtn); } "
        "else if (document.body) { consoleBtn.classList.add('btn-console-main-fixed'); document.body.appendChild(consoleBtn); }"
    )
    if old_init_ui_frag in js:
        js = js.replace(old_init_ui_frag, new_init_ui_frag)
    write_file(js_path, js)

    # --- 4. Commit message ---
    commit_msg = (
        f"fix(js): defer DOM queries to DOMContentLoaded; deduplicate toggleHeader\n\n"
        "- Wrapped preview click interceptor and editor drag/drop setup inside\n"
        "  DOMContentLoaded callbacks so they don't crash when the script loads\n"
        "  before the body.\n"
        "- Removed duplicate toggleHeader/updateArrow definitions left over from\n"
        "  previous patches.\n"
        "- Made initConsoleUI safe when .header-actions or body is not yet\n"
        "  available.\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()