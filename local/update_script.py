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

    # 2. Replace manual SSH auth with GetInsecureSSHAuth (bypasses known_hosts)
    old_auth_block = (
        '\t\t// Parse the private key with crypto/ssh (more reliable than go-git\'s parser)\n'
        '\t\tvar signer realssh.Signer\n'
        '\t\tif appConfig.SyncSSHPassphrase != "" {\n'
        '\t\t\tlog.Printf("[sync] Passphrase provided (length %d)", len(appConfig.SyncSSHPassphrase))\n'
        '\t\t\tsigner, err = realssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(appConfig.SyncSSHPassphrase))\n'
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] No passphrase")\n'
        '\t\t\tsigner, err = realssh.ParsePrivateKey(keyBytes)\n'
        '\t\t}\n'
        '\t\tif err != nil {\n'
        '\t\t\tlog.Printf("[sync] Failed to parse private key: %v", err)\n'
        '\t\t\thttp.Error(w, fmt.Sprintf("SSH key parse error: %v", err), 500)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
        '\t\tfp := realssh.FingerprintSHA256(signer.PublicKey())\n'
        '\t\tkeyType := signer.PublicKey().Type()\n'
        '\t\tlog.Printf("[sync] SSH key type: %s", keyType)\n'
        '\t\tpubKeyBlob := signer.PublicKey().Marshal()\n'
        '\t\tlog.Printf("[sync] SSH public key blob (base64): %s", realgobase64.StdEncoding.EncodeToString(pubKeyBlob))\n'
        '\t\tlog.Printf("[sync] SSH key fingerprint: %s", fp)\n'
        '\n'
        '\t\t// Use go-git\'s ssh.PublicKeys with the signer\n'
        '\t\tauth = &ssh.PublicKeys{User: sshUser, Signer: signer}\n'
        '\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer")'
    )
    new_auth_block = (
        '\t\t// Use GetInsecureSSHAuth to bypass host key checking\n'
        '\t\tauth, err = GetInsecureSSHAuth(sshUser, keyPath, appConfig.SyncSSHPassphrase)\n'
        '\t\tif err != nil {\n'
        '\t\t\tlog.Printf("[sync] GetInsecureSSHAuth error: %v", err)\n'
        '\t\t\thttp.Error(w, fmt.Sprintf("SSH auth failed: %v", err), 500)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
        '\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer (insecure host key)")'
    )
    patch_file("backend/handlers.go", old_auth_block, new_auth_block)
    print("✅ Replaced manual SSH auth with GetInsecureSSHAuth call.")

    # 3. Remove the now-useless known_hosts file workaround
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
    new_marker = (
        '\t// Stage and commit local changes first (manual tree & commit to avoid os/user on Android)'
    )
    if known_hosts_block in read_file("backend/handlers.go"):
        patch_file("backend/handlers.go", known_hosts_block, new_marker)
        print("✅ Removed useless known_hosts block.")
    else:
        print("⚠️ known_hosts block not found (maybe already removed).")

    # 4. Remove unused imports: realssh and realgobase64
    handlers = read_file("backend/handlers.go")
    imports_to_remove = [
        '\trealgobase64 "encoding/base64"\n',
        '\trealssh "golang.org/x/crypto/ssh"\n'
    ]
    for imp in imports_to_remove:
        if imp in handlers:
            handlers = handlers.replace(imp, '')
            write_file("backend/handlers.go", handlers)
            print(f"✅ Removed unused import: {imp.strip()}")
    # Clean any extra blank lines left by import removals
    handlers = re.sub(r'\n\n\n+', '\n\n', read_file("backend/handlers.go"))
    write_file("backend/handlers.go", handlers)

    # 5. Print commit message
    commit_msg = (
        "fix(sync): use GetInsecureSSHAuth to ignore host key checking\n\n"
        "- Replace manual crypto/ssh parsing with existing helper that sets HostKeyCallback\n"
        "- Removes the empty known_hosts workaround which caused 'key is unknown' errors\n"
        "- Cleans up now‑unused realssh and realgobase64 imports\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()