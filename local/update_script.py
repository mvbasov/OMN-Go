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

def apply_fix(path, old, new, idem_marker):
    content = read_file(path)
    if idem_marker in content:
        return
    if old not in content:
        raise ValueError(f"❌ Old string not found in {path}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def update_application():
    # Bump version
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

    # Fix the import: replace unaliased "encoding/base64" with realgobase64 alias
    apply_fix("backend/handlers.go",
              '\t"encoding/base64"',
              '\trealgobase64 "encoding/base64"',
              'realgobase64 "encoding/base64"')

    # Ensure the log line uses realgobase64 (already present, but just in case)
    apply_fix("backend/handlers.go",
              'log.Printf("[sync] SSH public key blob (base64): %s", realgobase64.StdEncoding.EncodeToString(pubKeyBlob))',
              'log.Printf("[sync] SSH public key blob (base64): %s", realgobase64.StdEncoding.EncodeToString(pubKeyBlob))',
              'realgobase64.StdEncoding.EncodeToString')

    commit_msg = (
        "fix(build): correct encoding/base64 import alias\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()