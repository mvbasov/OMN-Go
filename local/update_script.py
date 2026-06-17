import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.28 Embed Directive Fix...")

    # 1. Version Bumps
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.28"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.2\.\d+"', 'APP_VERSION = "1.2.28"'),
        "android/app/build.gradle": (r'versionCode 102\d{2}', 'versionCode 10228')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.2\.\d+';", "let v = '1.2.28';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.2\.\d+"', 'versionName "1.2.28"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch server.go to fix the broken []byte embed directive
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # Target the incorrectly modified frontendHTML directive
        old_embed = r'//go:embed frontend/html\s+var frontendHTML \[\]byte'
        new_embed = '//go:embed frontend/index.html\nvar frontendHTML []byte'

        new_server_code = re.sub(old_embed, new_embed, server_code)

        if new_server_code != server_code:
            with open(server_go, "w", encoding="utf-8") as f:
                f.write(new_server_code)
            print("  [+] Successfully restored //go:embed frontend/index.html for []byte type.")
        else:
            print("  [=] Embed directive already correct or pattern not found.")

    commit_msg = """fix(compiler): restore single-file embed directive for frontendHTML []byte

- Reverted accidental regex overwrite of the `frontendHTML` embed directive. `[]byte` types strictly require a single file target (`frontend/index.html`) and cannot embed directories.
- Preserved the new unified `frontend/html` directory embed for `staticFS`.
- Bumped application to V1.2.28 (Android 10228)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()