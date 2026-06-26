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

    # 2. Revert the broken HostKeyCallback line back to original
    old_broken_auth = (
        '\t\tauth = &ssh.PublicKeys{User: sshUser, Signer: signer, HostKeyCallback: ssh.InsecureIgnoreHostKey()}\n'
        '\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer (insecure host key)")'
    )
    original_auth = (
        '\t\tauth = &ssh.PublicKeys{User: sshUser, Signer: signer}\n'
        '\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer")'
    )
    try:
        patch_file("backend/handlers.go", old_broken_auth, original_auth)
        print("✅ Reverted SSH auth to original (without HostKeyCallback).")
    except ValueError:
        print("⚠️ Broken auth line not found, maybe already fixed.")

    # 3. Add known_hosts file creation and env setup before the SSH operations
    # Insert right before the comment "// Stage and commit local changes first"
    insert_marker = (
        '\t// Stage and commit local changes first (manual tree & commit to avoid os/user on Android)'
    )
    known_hosts_block = (
        '\t// Create an empty known_hosts file to bypass go-git SSH check on Android\n'
        '\tknownHostsPath := filepath.Join(storageDir, ".git", "known_hosts")\n'
        '\tif _, err := os.Stat(knownHostsPath); os.IsNotExist(err) {\n'
        '\t\tif err := os.MkdirAll(filepath.Dir(knownHostsPath), 0755); err != nil {\n'
        '\t\t\tlog.Printf("[sync] Failed to create .git dir: %v", err)\n'
        '\t\t} else if err := os.WriteFile(knownHostsPath, []byte{}, 0644); err != nil {\n'
        '\t\t\tlog.Printf("[sync] Failed to write known_hosts: %v", err)\n'
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Created empty known_hosts file")\n'
        '\t\t}\n'
        '\t}\n'
        '\tos.Setenv("SSH_KNOWN_HOSTS", knownHostsPath)\n'
        '\n'
        '\t// Stage and commit local changes first (manual tree & commit to avoid os/user on Android)'
    )
    if insert_marker in read_file("backend/handlers.go"):
        patch_file("backend/handlers.go", insert_marker, known_hosts_block)
        print("✅ Added known_hosts creation and SSH_KNOWN_HOSTS env setup.")
    else:
        print("❌ Insert marker not found – could not add known_hosts logic.")

    # 4. Commit message
    commit_msg = (
        "fix(sync): provide known_hosts file to bypass SSH host key check\n\n"
        "- Create empty .git/known_hosts and set SSH_KNOWN_HOSTS env variable\n"
        "- Avoids 'unable to find any valid known_hosts file' error during pull/push\n"
        "- Removed broken HostKeyCallback approach that caused compile errors\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()