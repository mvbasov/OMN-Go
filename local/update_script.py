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

def replace_string_in_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Target string not found in {path}")
    content = content.replace(old, new, 1)
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

    # 2. Fix SSH user: extract from SyncRemote instead of using hardcoded "git"
    # Replace the two occurrences of ssh.NewPublicKeys("git", ...) with the resolved user.
    # We'll compute the user once before the auth block and then use it.
    # Target line: `log.Printf("[sync] Using SSH key: %s", keyPath)` - we can insert user resolution before.
    # Better: replace the whole "Prepare SSH auth" block with a version that resolves the user.

    old_auth_block = """\t// Prepare SSH auth
\tvar auth transport.AuthMethod
\tif appConfig.SyncSSHKey != "" {
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
\t\t\tauth, err = ssh.NewPublicKeys("git", keyBytes, passphrase)
\t\t} else {
\t\t\tlog.Printf("[sync] No passphrase")
\t\t\tauth, err = ssh.NewPublicKeys("git", keyBytes, "")
\t\t}
\t\tif err != nil {
\t\t\tlog.Printf("[sync] ssh.NewPublicKeys error: %v", err)
\t\t\thttp.Error(w, fmt.Sprintf("SSH auth failed: %v", err), 500)
\t\t\treturn
\t\t}
\t\tlog.Printf("[sync] SSH auth method created successfully")
\t} else {
\t\tlog.Printf("[sync] Error: No SSH key configured")
\t\thttp.Error(w, "No SSH key configured", 500)
\t\treturn
\t}"""

    new_auth_block = """\t// Prepare SSH auth
\t// Extract user from remote URL (e.g., gitolite3@host:path -> gitolite3)
\tsshUser := "git"
\tif idx := strings.Index(appConfig.SyncRemote, "@"); idx != -1 {
\t\tsshUser = appConfig.SyncRemote[:idx]
\t}
\tlog.Printf("[sync] SSH user: %s", sshUser)

\tvar auth transport.AuthMethod
\tif appConfig.SyncSSHKey != "" {
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
\t} else {
\t\tlog.Printf("[sync] Error: No SSH key configured")
\t\thttp.Error(w, "No SSH key configured", 500)
\t\treturn
\t}"""

    try:
        replace_string_in_file("backend/handlers.go", old_auth_block, new_auth_block)
    except ValueError:
        # maybe the old block is slightly different due to previous patches; try to find it via a partial match
        content = read_file("backend/handlers.go")
        if 'ssh.NewPublicKeys("git"' not in content:
            print("Already using dynamic user (no hardcoded 'git' found).")
        else:
            raise

    commit_msg = (
        "fix(sync): extract SSH user from remote URL instead of hardcoding 'git'\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()