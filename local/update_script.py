import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.7 Compiler Fix (Restoring htmlEscape)...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.6"', 'APP_VERSION = "1.2.7"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.6";', 'const APP_VERSION = "1.2.7";'),
        ("backend/frontend/index.html", "let v = '1.2.6';", "let v = '1.2.7';"),
        ("android/app/build.gradle", "versionCode 10206", "versionCode 10207"),
        ("android/app/build.gradle", 'versionName "1.2.6"', 'versionName "1.2.7"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")
            else:
                print(f"  [-] Version string not found in {filepath} (Already updated?)")

    # 2. Restore the missing htmlEscape function in server.go
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()
        
        old_compile_page_def = "func compilePage(name string, mdContent []byte) []byte {"
        
        # We inject htmlEscape right above compilePage where it used to be
        restored_html_escape = """func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\\"", "&quot;")
	return s
}

func compilePage(name string, mdContent []byte) []byte {"""

        if "func htmlEscape(s string) string {" not in server_code:
            if old_compile_page_def in server_code:
                server_code = server_code.replace(old_compile_page_def, restored_html_escape)
                with open(server_go, "w", encoding="utf-8") as f:
                    f.write(server_code)
                print("  [+] Successfully restored htmlEscape() utility to server.go.")
            else:
                print("  [!] Error: Could not find compilePage definition to anchor htmlEscape.")
        else:
            print("  [=] htmlEscape() already exists in server.go.")

    commit_msg = """fix(compiler): restore htmlEscape utility deleted by regex

- Restored `htmlEscape` function that was accidentally removed during the Goldmark regex engine swap.
- This resolves the `undefined: htmlEscape` Go compilation failure.
- Bumped application to V1.2.7 (Android 10207)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()