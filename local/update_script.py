#!/usr/bin/env python3
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
    old_code = int(cur_ver.replace(".", ""))
    new_code = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {old_code}', f'versionCode {new_code}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix the SSH known_hosts problem by setting HostKeyCallback to insecure
    old_auth_line = (
        '\t\tauth = &ssh.PublicKeys{User: sshUser, Signer: signer}\n'
        '\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer")'
    )
    new_auth_line = (
        '\t\tauth = &ssh.PublicKeys{User: sshUser, Signer: signer, HostKeyCallback: ssh.InsecureIgnoreHostKey()}\n'
        '\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer (insecure host key)")'
    )
    try:
        patch_file("backend/handlers.go", old_auth_line, new_auth_line)
        print("✅ Added HostKeyCallback to SSH auth to bypass known_hosts requirement.")
    except ValueError as e:
        print(f"⚠️ Could not patch SSH auth line: {e}")

    # 3. Print commit message
    commit_msg = (
        "fix(sync): bypass known_hosts by setting HostKeyCallback to InsecureIgnoreHostKey\n\n"
        "- The SSH pull failed with 'unable to find any valid known_hosts file'\n"
        "- Now uses go-git's InsecureIgnoreHostKey callback, consistent with GetInsecureSSHAuth\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()