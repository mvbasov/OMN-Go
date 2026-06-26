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

    # Log the base64 of the public key exactly as it will be sent
    old_log = '\t\tkeyType := signer.PublicKey().Type()\n\t\tlog.Printf("[sync] SSH key type: %s", keyType)'
    new_log = '\t\tkeyType := signer.PublicKey().Type()\n\t\tlog.Printf("[sync] SSH key type: %s", keyType)\n\t\tpubKeyBlob := signer.PublicKey().Marshal()\n\t\tlog.Printf("[sync] SSH public key blob (base64): %s", realgobase64.StdEncoding.EncodeToString(pubKeyBlob))'
    apply_fix("backend/handlers.go", old_log, new_log, 'pubKeyBlob := signer.PublicKey().Marshal()')

    # Ensure encoding/base64 is imported (as realgobase64 to avoid conflict)
    h_path = "backend/handlers.go"
    h = read_file(h_path)
    if '"encoding/base64"' not in h:
        # Insert after "time" import or similar
        h = h.replace('"time"', '"time"\n\t"encoding/base64"')
        write_file(h_path, h)

    # Also add an alias for base64 if needed
    if 'realgobase64' not in h and '"encoding/base64"' in h:
        h = h.replace('"encoding/base64"', 'realgobase64 "encoding/base64"')
        write_file(h_path, h)

    commit_msg = (
        "feat(sync): log SSH public key blob for server-side comparison\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()