import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/version.go", 'const APP_VERSION = "1.3.22"', 'const APP_VERSION = "1.3.23"'),
        ("backend/frontend/html/js/omn-go-core.js", "let v = '1.3.22';", "let v = '1.3.23';"),
        ("android/app/build.gradle", "versionCode 10322", "versionCode 10323"),
        ("android/app/build.gradle", 'versionName "1.3.22"', 'versionName "1.3.23"')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "backend/frontend/html/css/omn-go-core.css": [
            # Inject new Console CSS Classes cleanly before the Responsive Design block
            (
                "/* Responsive Design */",
                "/* JS Console UI */\n"
                ".console-modal { display: none; position: fixed; top: 10%; left: 10%; width: 80%; height: 80%; background: #1e1e1e; color: #00ff00; z-index: 10000; border: 2px solid #555; border-radius: 8px; flex-direction: column; font-family: monospace; box-shadow: 0 4px 12px rgba(0,0,0,0.5); }\n"
                ".console-header { padding: 10px; background: #333; color: #fff; display: flex; justify-content: space-between; align-items: center; border-bottom: 1px solid #555; font-weight: bold; }\n"
                ".console-actions { display: flex; gap: 8px; }\n"
                ".console-logs { flex: 1; overflow-y: auto; padding: 10px; white-space: pre-wrap; word-break: break-all; font-size: 12px; line-height: 1.4; }\n"
                ".btn-console { color: white; border: none; border-radius: 4px; padding: 4px 8px; cursor: pointer; display: flex; align-items: center; justify-content: center; }\n"
                ".btn-console-clear { background: #888; }\n"
                ".btn-console-close { background: #ff5555; }\n"
                ".btn-console-main { margin-left: 8px; padding: 4px 8px; background: #ff9800; color: #fff; border: none; border-radius: 4px; cursor: pointer; font-size: 0.8rem; font-weight: bold; display: flex; align-items: center; }\n"
                ".btn-console-main-fixed { position: fixed; bottom: 4px; left: 8px; z-index: 9999; margin-left: 0; }\n"
                ".icon-sm { font-size: 18px; }\n"
                ".icon-xs { font-size: 16px; margin-right: 4px; }\n\n"
                "/* Responsive Design */"
            )
        ],
        "backend/frontend/html/js/omn-go-core.js": [
            # Fix Toggle Buttons
            (
                "btn.innerText = 'View Mode';",
                "btn.innerHTML = '<i class=\"material-icons\" title=\"Switch to View Mode\">visibility</i>';"
            ),
            (
                "btn.innerText = 'Edit Mode';",
                "btn.innerHTML = '<i class=\"material-icons\" title=\"Switch to Edit Mode\">edit</i>';"
            ),
            # Strip inline CSS and inject CSS Classes for Console Layout
            (
                "consoleModal = document.createElement('div');\n                consoleModal.id = 'omn-go-console-modal';\n                consoleModal.style.cssText = 'display:none; position:fixed; top:10%; left:10%; width:80%; height:80%; background:#1e1e1e; color:#00ff00; z-index:10000; border:2px solid #555; border-radius:8px; flex-direction:column; font-family:monospace; box-shadow: 0 4px 12px rgba(0,0,0,0.5);';\n\n                const header = document.createElement('div');\n                header.style.cssText = 'padding:10px; background:#333; color:#fff; display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid #555; font-weight:bold;';\n                header.innerHTML = '<span>JS Console Output</span><div><button id=\"omn-go-console-clear\" style=\"background:#888; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer; margin-right:8px;\">Clear</button><button id=\"omn-go-console-close\" style=\"background:#ff5555; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer;\">Close</button></div>';\n\n                logsContainer = document.createElement('div');\n                logsContainer.style.cssText = 'flex:1; overflow-y:auto; padding:10px; white-space:pre-wrap; word-break:break-all; font-size:12px; line-height:1.4;';",
                "consoleModal = document.createElement('div');\n                consoleModal.id = 'omn-go-console-modal';\n                consoleModal.className = 'console-modal';\n\n                const header = document.createElement('div');\n                header.className = 'console-header';\n                header.innerHTML = '<span>JS Console Output</span><div class=\"console-actions\"><button id=\"omn-go-console-clear\" class=\"btn-console btn-console-clear\" title=\"Clear Console\"><i class=\"material-icons icon-sm\">delete_sweep</i></button><button id=\"omn-go-console-close\" class=\"btn-console btn-console-close\" title=\"Close Console\"><i class=\"material-icons icon-sm\">close</i></button></div>';\n\n                logsContainer = document.createElement('div');\n                logsContainer.className = 'console-logs';"
            ),
            # Strip inline CSS and inject CSS Classes for Console Float Button
            (
                "consoleBtn = document.createElement('button');\n                consoleBtn.id = 'omn-go-console-btn';\n                consoleBtn.innerText = 'Console (0)';\n                consoleBtn.style.cssText = 'margin-left:8px; padding:4px 8px; background:#ff9800; color:#fff; border:none; border-radius:4px; cursor:pointer; font-size:0.8rem; font-weight:bold;';\n                consoleBtn.onclick = () => {\n                    consoleModal.style.display = 'flex';\n                };\n\n                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {\n                    if (el.children.length > 0) return false;\n                    const text = (el.textContent || '').toLowerCase();\n                    const id = (el.id || '').toLowerCase();\n                    const cls = (el.className || '').toLowerCase();\n                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');\n                });\n\n                if (metadataEl && metadataEl.parentNode) {\n                    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);\n                } else {\n                    consoleBtn.style.position = 'fixed';\n                    consoleBtn.style.bottom = '4px';\n                    consoleBtn.style.left = '8px';\n                    consoleBtn.style.zIndex = '9999';\n                    document.body.appendChild(consoleBtn);\n                }",
                "consoleBtn = document.createElement('button');\n                consoleBtn.id = 'omn-go-console-btn';\n                consoleBtn.className = 'btn-console-main';\n                consoleBtn.innerHTML = '<i class=\"material-icons icon-xs\">terminal</i><span>0</span>';\n                consoleBtn.onclick = () => {\n                    consoleModal.style.display = 'flex';\n                };\n\n                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {\n                    if (el.children.length > 0) return false;\n                    const text = (el.textContent || '').toLowerCase();\n                    const id = (el.id || '').toLowerCase();\n                    const cls = (el.className || '').toLowerCase();\n                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');\n                });\n\n                if (metadataEl && metadataEl.parentNode) {\n                    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);\n                } else {\n                    consoleBtn.classList.add('btn-console-main-fixed');\n                    document.body.appendChild(consoleBtn);\n                }"
            ),
            # Fix dynamic console text updates
            (
                "if (consoleBtn) consoleBtn.innerText = 'Console (0)';",
                "if (consoleBtn) consoleBtn.innerHTML = '<i class=\"material-icons icon-xs\">terminal</i><span>0</span>';"
            ),
            (
                "if (!consoleBtn) initConsoleUI();\n                consoleBtn.innerText = `Console (${logs.length})`;",
                "if (!consoleBtn) initConsoleUI();\n                consoleBtn.innerHTML = `<i class=\"material-icons icon-xs\">terminal</i><span>${logs.length}</span>`;"
            )
        ]
    }

    print("[*] Starting OMN-Go Update Process...")

    # Execute Version Bumps
    for file_path, old_str, new_str in version_replacements:
        if os.path.exists(file_path):
            with open(file_path, "r", encoding="utf-8") as f:
                content = f.read()
            if old_str in content:
                content = content.replace(old_str, new_str)
                with open(file_path, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"[+] VERSION BUMP: {file_path}")
            else:
                print(f"[-] WARN: Version string not found in {file_path}")

    # Execute Logic Patches
    for file_path, file_patches in patches.items():
        if not os.path.exists(file_path):
            print(f"[!] ERROR: Target file {file_path} not found.")
            continue
            
        with open(file_path, "r", encoding="utf-8") as f:
            content = f.read()
            
        for old_str, new_str in file_patches:
            if old_str in content:
                content = content.replace(old_str, new_str)
                print(f"[+] PATCH SUCCESS: Applied logic injection to {file_path}")
            else:
                raise ValueError(f"\n[!] FATAL: Could not find target string in {file_path}:\n--- EXPECTED ---\n{old_str}\n----------------\n")
                
        with open(file_path, "w", encoding="utf-8") as f:
            f.write(content)

    # 3. Output Standardized Git Commit Message matching modifications
    commit_msg = """feat(ui): decouple JS inline styles and use CSS classes for icons

- Extracted hardcoded `style="..."` attributes from JS console UI and moved them strictly to `omn-go-core.css` using proper class mappings (.console-modal, .btn-console, etc).
- Replaced JS-injected text labels with `innerHTML` Material Icons logic.
- Corrected `toggleMode()` breaking the toolbar icon logic upon switching between Edit/View Mode.

Version bumped to 1.3.23"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()