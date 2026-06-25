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

    # Fix imports and add fingerprint logging in handlers.go
    h_path = "backend/handlers.go"
    h = read_file(h_path)

    # 1. Remove any bare import of golang.org/x/crypto/ssh (without alias)
    h = re.sub(r'^[ \t]*"golang\.org/x/crypto/ssh"\s*\n', '', h, flags=re.MULTILINE)

    # 2. Ensure realssh import exists after the go-git ssh line
    git_ssh_line = 'ssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"'
    realssh_import = '\trealssh "golang.org/x/crypto/ssh"'
    if git_ssh_line in h and realssh_import not in h:
        # Insert realssh import right after the git ssh line
        h = h.replace(git_ssh_line, git_ssh_line + '\n' + realssh_import)
    elif realssh_import not in h:
        # If git_ssh_line missing, perhaps it's aliased differently, but we expect it's present.
        # If not, we'll add both? For now just ensure realssh import after that line.
        # Another attempt: find any line containing "github.com/go-git/go-git/v5/plumbing/transport/ssh"
        m = re.search(r'^(\s*)(.*"github\.com/go-git/go-git/v5/plumbing/transport/ssh".*)$', h, flags=re.MULTILINE)
        if m:
            indent = m.group(1)
            h = h.replace(m.group(0), m.group(0) + '\n' + indent + 'realssh "golang.org/x/crypto/ssh"')
    write_file(h_path, h)

    # 3. Add fingerprint logging if not present
    old_log = '\t\tlog.Printf("[sync] SSH auth method created successfully")'
    new_log = '\t\tlog.Printf("[sync] SSH auth method created successfully")\n\t\t// Log public key fingerprint for debugging\n\t\tif parsedKey, err := realssh.ParsePrivateKey(keyBytes); err == nil {\n\t\t\tfp := realssh.FingerprintSHA256(parsedKey.PublicKey())\n\t\t\tlog.Printf("[sync] SSH key fingerprint: %s", fp)\n\t\t}'
    h = read_file(h_path)  # re-read after previous write
    if old_log in h and 'realssh.FingerprintSHA256' not in h:
        h = h.replace(old_log, new_log)
        write_file(h_path, h)

    commit_msg = (
        "fix(sync): correct crypto/ssh import alias and add fingerprint logging\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()