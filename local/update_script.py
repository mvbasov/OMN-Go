import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.3 Editor Save & Routing Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.2"', 'APP_VERSION = "1.2.3"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.2";', 'const APP_VERSION = "1.2.3";'),
        ("backend/frontend/index.html", "let v = '1.2.2';", "let v = '1.2.3';"),
        ("android/app/build.gradle", 'versionCode 10202', 'versionCode 10203'),
        ("android/app/build.gradle", 'versionName "1.2.2"', 'versionName "1.2.3"')
    ]

    for filepath, old_v, new_v in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_v in content:
                content = content.replace(old_v, new_v)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")
            else:
                print(f"  [-] Version string not found in {filepath} (Already updated?)")

    # 2. Define complex code block patches
    patches = {
        "backend/server.go": [
            (
                '\tname := r.FormValue("name")\n\tcontent := r.FormValue("content")\n\tif name == "" {\n\t\treturn\n\t}\n\n\tvar path string\n\tif strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {',
                '\tname := r.FormValue("name")\n\tcontent := r.FormValue("content")\n\tif name == "" {\n\t\treturn\n\t}\n\n\t// Normalize textarea line endings to prevent Pelican header breakage\n\tcontent = strings.ReplaceAll(content, "\\r\\n", "\\n")\n\n\tvar path string\n\tif !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {'
            ),
            (
                '\tvar path string\n\tvar data []byte\n\tvar err error\n\n\tif strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {',
                '\tvar path string\n\tvar data []byte\n\tvar err error\n\n\tif !strings.Contains(name, ".") || strings.HasSuffix(name, ".md") || strings.HasSuffix(name, ".html") {'
            ),
            (
                '\tlayout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", string(mdContent))',
                '\tlayout = strings.ReplaceAll(layout, "<!-- OMN_GO_RAW_MD -->", htmlEscape(string(mdContent)))'
            )
        ],
        "backend/frontend/index.html": [
            (
                "let currentMode = 'view';\n        async function toggleMode() {\n            try {\n                const res = await fetch('/api/config');\n                if (res.ok) {",
                "let currentMode = 'view';\n        async function toggleMode() {\n            try {\n                const res = await fetch('/api/config', { cache: 'no-store' });\n                if (res.ok) {"
            )
        ]
    }

    # Execute patches safely
    for filepath, file_patches in patches.items():
        if not os.path.exists(filepath):
            print(f"  [!] Missing file: {filepath}")
            continue
            
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
            
        for old_str, new_str in file_patches:
            if old_str in content:
                content = content.replace(old_str, new_str)
                print(f"  [+] Patched target in {filepath}")
            elif new_str in content:
                print(f"  [=] Target already patched in {filepath}")
            else:
                print(f"  [!] WARNING: Target string missing in {filepath}\n      Expected snippet: {old_str[:60]}...")
                
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)

    commit_msg = """fix(engine): resolve markdown save failures and external editor cache

- Added cache: 'no-store' to the config fetch request to ensure the browser doesn't swallow external editor checks.
- Altered server.go to recognize extensionless files (like 'Welcome') as Markdown notes, fixing the save bypass bug.
- Normalized incoming CRLF line endings to prevent Pelican headers from being misidentified.
- Added htmlEscape to raw MD injection to prevent unescaped characters breaking the editor textarea boundaries.
- Bumped application version to 1.2.3 (Android 10203)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()