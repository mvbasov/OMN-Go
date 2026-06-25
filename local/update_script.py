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

def add_import(path, pkg, after_pattern):
    """Insert import line for *pkg* after the line matching *after_pattern*.
    Idempotent: does nothing if *pkg* already exists in the file.
    """
    content = read_file(path)
    if pkg in content:
        return
    # Find the line that matches after_pattern
    pattern = r'^(\s*).*' + re.escape(after_pattern) + r'.*\n'
    m = re.search(pattern, content, flags=re.MULTILINE)
    if not m:
        raise ValueError(f"❌ Could not find anchor import '{after_pattern}'")
    indent = m.group(1) if m.group(1) else '\t'
    insert_line = f'{indent}"{pkg}"\n'
    pos = m.end()
    new_content = content[:pos] + insert_line + content[pos:]
    write_file(path, new_content)

def replace_or_insert_block(path, old_block, new_block, idem_marker):
    """Replace *old_block* with *new_block*; if *idem_marker* already exists, do nothing.
    If *old_block* not found, raise error.
    """
    content = read_file(path)
    if idem_marker in content:
        return
    if old_block not in content:
        raise ValueError(f"❌ Old block not found in {path}")
    content = content.replace(old_block, new_block, 1)
    write_file(path, content)

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
    cur_vc = int(cur_ver.replace(".", ""))
    new_vc = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {cur_vc}', f'versionCode {new_vc}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Add realssh import after the go-git/ssh import
    add_import("backend/handlers.go",
               "golang.org/x/crypto/ssh",
               "github.com/go-git/go-git/v5/plumbing/transport/ssh")

    # 3. Add fingerprint logging after "SSH auth method created successfully"
    old_log = '\t\tlog.Printf("[sync] SSH auth method created successfully")'
    new_log = '\t\tlog.Printf("[sync] SSH auth method created successfully")\n\t\t// Log public key fingerprint for debugging\n\t\tif parsedKey, err := realssh.ParsePrivateKey(keyBytes); err == nil {\n\t\t\tfp := realssh.FingerprintSHA256(parsedKey.PublicKey())\n\t\t\tlog.Printf("[sync] SSH key fingerprint: %s", fp)\n\t\t}'
    replace_or_insert_block("backend/handlers.go", old_log, new_log, 'realssh.FingerprintSHA256')

    commit_msg = (
        "feat(sync): log SSH public key fingerprint for debugging\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()