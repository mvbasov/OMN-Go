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

def apply_idempotent_replace(path, old, new, idem_marker):
    content = read_file(path)
    if idem_marker in content:
        return
    if old not in content:
        raise ValueError(f"❌ Old block not found in {path}")
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

    # Replace the SSH auth block to use realssh signer directly
    old_auth_block = """\tif appConfig.SyncSSHKey != "" {
\t\tkeyPath := appConfig.SyncSSHKey
\t\tif !filepath.IsAbs(keyPath) {
\t\t\tkeyPath = filepath.Join(storageDir, keyPath)
\t\t}
\t\tlog.Printf("[sync] Using SSH key: %s", keyPath)

\t\t// Check file existence and permissions
\t\tinfo, err := os.Stat(keyPath)
\t\tif err != nil {
\t\t\tlog.Printf("[sync] SSH key file not accessible: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("Failed to read SSH key: %v", err), 500)
\t\t\treturn
\t\t}
\t\tlog.Printf("[sync] Key file size: %d, mode: %s", info.Size(), info.Mode())

\t\tkeyBytes, err := os.ReadFile(keyPath)
\t\tif err != nil {
\t\t\tlog.Printf("[sync] Read key file error: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("Failed to read SSH key: %v", err), 500)
\t\t\treturn
\t\t}
\t\tlog.Printf("[sync] Read %d bytes from key file", len(keyBytes))

\t\tpassphrase := appConfig.SyncSSHPassphrase
\t\tif passphrase != "" {
\t\t\tlog.Printf("[sync] Passphrase provided (length %d)", len(passphrase))
\t\t\tauth, err = ssh.NewPublicKeys(sshUser, keyBytes, passphrase)
\t\t} else {
\t\t\tlog.Printf("[sync] No passphrase")
\t\t\tauth, err = ssh.NewPublicKeys(sshUser, keyBytes, "")
\t\t}
\t\tif err != nil {
\t\t\tlog.Printf("[sync] ssh.NewPublicKeys error: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("SSH auth failed: %v", err), 500)
\t\t\treturn
\t\t}
\t\tlog.Printf("[sync] SSH auth method created successfully")
\t\t// Log public key fingerprint for debugging
\t\tif parsedKey, err := realssh.ParsePrivateKey(keyBytes); err == nil {
\t\t\tfp := realssh.FingerprintSHA256(parsedKey.PublicKey())
\t\t\tlog.Printf("[sync] SSH key fingerprint: %s", fp)
\t\t}
\t}"""

    new_auth_block = """\tif appConfig.SyncSSHKey != "" {
\t\tkeyPath := appConfig.SyncSSHKey
\t\tif !filepath.IsAbs(keyPath) {
\t\t\tkeyPath = filepath.Join(storageDir, keyPath)
\t\t}
\t\tlog.Printf("[sync] Using SSH key: %s", keyPath)

\t\tinfo, err := os.Stat(keyPath)
\t\tif err != nil {
\t\t\tlog.Printf("[sync] SSH key file not accessible: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("Failed to read SSH key: %v", err), 500)
\t\t\treturn
\t\t}
\t\tlog.Printf("[sync] Key file size: %d, mode: %s", info.Size(), info.Mode())

\t\tkeyBytes, err := os.ReadFile(keyPath)
\t\tif err != nil {
\t\t\tlog.Printf("[sync] Read key file error: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("Failed to read SSH key: %v", err), 500)
\t\t\treturn
\t\t}
\t\tlog.Printf("[sync] Read %d bytes from key file", len(keyBytes))

\t\t// Parse the private key with crypto/ssh (more reliable than go-git's parser)
\t\tvar signer realssh.Signer
\t\tif appConfig.SyncSSHPassphrase != "" {
\t\t\tlog.Printf("[sync] Passphrase provided (length %d)", len(appConfig.SyncSSHPassphrase))
\t\t\tsigner, err = realssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(appConfig.SyncSSHPassphrase))
\t\t} else {
\t\t\tlog.Printf("[sync] No passphrase")
\t\t\tsigner, err = realssh.ParsePrivateKey(keyBytes)
\t\t}
\t\tif err != nil {
\t\t\tlog.Printf("[sync] Failed to parse private key: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("SSH key parse error: %v", err), 500)
\t\t\treturn
\t\t}
\t\tfp := realssh.FingerprintSHA256(signer.PublicKey())
\t\tlog.Printf("[sync] SSH key fingerprint: %s", fp)

\t\t// Use go-git's ssh.PublicKeys with the signer
\t\tauth = ssh.PublicKeys(signer)
\t\tlog.Printf("[sync] SSH auth method created using crypto/ssh signer")
\t}"""

    apply_idempotent_replace("backend/handlers.go", old_auth_block, new_auth_block,
                             'auth = ssh.PublicKeys(signer)')

    commit_msg = (
        "fix(sync): use crypto/ssh signer directly instead of ssh.NewPublicKeys\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()