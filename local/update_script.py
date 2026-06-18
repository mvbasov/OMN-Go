import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.3.4 Dynamic Versioning Patch...")

    new_version = "1.3.4"
    new_version_code = "10304"

    # 1. Version Bumps (server.go, index.html, build.gradle)
    files_to_bump = {
        "backend/server.go": (r'APP_VERSION = "1\.3\.\d+"', f'APP_VERSION = "{new_version}"'),
        "backend/frontend/index.html": (r'APP_VERSION = "1\.3\.\d+"', f'APP_VERSION = "{new_version}"'),
        "android/app/build.gradle": (r'versionCode 103\d{2}', f'versionCode {new_version_code}')
    }

    for filepath, (pattern, replacement) in files_to_bump.items():
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            
            new_content = re.sub(pattern, replacement, content)
            
            if "index.html" in filepath:
                new_content = re.sub(r"let v = '1\.3\.\d+';", f"let v = '{new_version}';", new_content)

            if "build.gradle" in filepath:
                new_content = re.sub(r'versionName "1\.3\.\d+"', f'versionName "{new_version}"', new_content)

            if new_content != content:
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(new_content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch Dockerfile for Dynamic Bash Versioning
    dockerfile = "Dockerfile"
    if os.path.exists(dockerfile):
        with open(dockerfile, "r", encoding="utf-8") as f:
            docker_code = f.read()

        # Find the bounds of the Desktop build instructions
        start_str = "# Desktop Binary"
        end_str = "# Android APK"
        start_idx = docker_code.find(start_str)
        end_idx = docker_code.find(end_str)

        if start_idx != -1 and end_idx != -1:
            # Inject dynamic awk extraction and bash variable interpolation
            dynamic_run = '''# Desktop Binary (OMN-Go naming convention)
RUN VERSION=$(awk -F'"' '/APP_VERSION =/ {print $2}' backend/server.go) && \\
    GOOS=linux GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-linux-amd64" main_desktop.go && \\
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o "bin/omn-go-v${VERSION}-desktop-windows-amd64.exe" main_desktop.go

'''
            # Slice out the old hardcoded block and replace it
            new_docker_code = docker_code[:start_idx] + dynamic_run + docker_code[end_idx:]

            if new_docker_code != docker_code:
                with open(dockerfile, "w", encoding="utf-8") as f:
                    f.write(new_docker_code)
                print("  [+] Successfully injected dynamic version extraction into Dockerfile.")
        else:
            print("  [-] Could not find bounds in Dockerfile to replace.")

    commit_msg = f"""build(docker): dynamically extract desktop app version from server.go

- Replaced the hardcoded version strings in the `Dockerfile` with a dynamic bash `awk` extraction sequence.
- The Docker container now parses `APP_VERSION` directly from `backend/server.go` during the build sequence and uses it to automatically name the Linux and Windows binaries.
- This eliminates the need to manually patch the `Dockerfile` on every release.
- Bumped application to V{new_version} (Android {new_version_code})."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()