import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.36...")

def bump_version():
    new_v = "1.4.36"
    new_v_c = "10436"

    # 1. Update version.go
    ver_path = "backend/version.go"
    if os.path.exists(ver_path):
        with open(ver_path, "w") as f: 
            f.write(f'package backend\n\n// APP_VERSION is the global application version\nconst APP_VERSION = "{new_v}"\n')
        print(f"  [+] Hard-rewrote {ver_path} with APP_VERSION = {new_v}")
    else:
        print(f"  [-] Warning: {ver_path} not found")

    # 2. Update Android Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r") as f: c = f.read()
        c = re.sub(r'versionCode\s+\d+', f'versionCode {new_v_c}', c)
        c = re.sub(r'versionName\s+"[^"]+"', f'versionName "{new_v}"', c)
        with open(gradle_path, "w") as f: f.write(c)
        print("  [+] Bumped version in android/app/build.gradle")

def rebuild_git_helper_with_key_logger():
    # Keep our exact public key extractor
    helper_path = "backend/git_helper.go"
    helper_code = """package backend

import (
\t"log"
\t"os"
\t"strings"
\t"github.com/go-git/go-git/v5/plumbing/transport/ssh"
\tgossh "golang.org/x/crypto/ssh"
)

// GetInsecureSSHAuth bypasses strict host key checking which blocks Android from connecting to gitolite
func GetInsecureSSHAuth(sshUser, privateKeyPath, password string) (*ssh.PublicKeys, error) {
\t_, err := os.Stat(privateKeyPath)
\tif err != nil {
\t\treturn nil, err
\t}
\tpublicKeys, err := ssh.NewPublicKeysFromFile(sshUser, privateKeyPath, password)
\tif err != nil {
\t\treturn nil, err
\t}
\t
\t// EXPLICIT PUBKEY EXTRACTION
\tsigner := publicKeys.Signer
\tpubKeyBytes := gossh.MarshalAuthorizedKey(signer.PublicKey())
\tpubKeyStr := strings.TrimSpace(string(pubKeyBytes))
\t
\tlog.Printf("\\n[CRITICAL] To fix 'unable to authenticate', add THIS EXACT KEY to your gitolite-admin repo:")
\tlog.Printf("[CRITICAL] %s", pubKeyStr)
\tlog.Printf("[CRITICAL] Your desktop CLI likely succeeded by silently falling back to ~/.ssh/id_rsa!\\n")

\t// CRITICAL FIX: Ignore host key verification for gitolite3 servers
\tpublicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
\treturn publicKeys, nil
}
"""
    with open(helper_path, "w") as f: 
        f.write(helper_code)
    print("  [+] Cleanly rebuilt backend/git_helper.go")

def fix_android_commit_enosys():
    # Android throws "function not implemented" when go-git tries to lookup the OS User for commits.
    # We must explicitly bypass os/user.Current() by hardcoding the Author Signature.
    for go_file in glob.glob("backend/*.go"):
        if not os.path.exists(go_file): continue
        with open(go_file, "r") as f: content = f.read()

        if "git.CommitOptions" in content:
            modified = False
            # Ensure required packages are imported
            if '"time"' not in content:
                content = re.sub(r'import \(\n', 'import (\n\t"time"\n', content, count=1)
                modified = True
            if 'plumbing/object' not in content:
                content = re.sub(r'import \(\n', 'import (\n\t"github.com/go-git/go-git/v5/plumbing/object"\n', content, count=1)
                modified = True

            # Safely inject the Author explicit block
            def inject_author(match):
                inner = match.group(2)
                if "Author:" not in inner:
                    author_block = """
\t\tAuthor: &object.Signature{
\t\t\tName:  "OMN-Go App",
\t\t\tEmail: "sync@omn-go.local",
\t\t\tWhen:  time.Now(),
\t\t},"""
                    return match.group(1) + author_block + inner + "}"
                return match.group(0)

            new_content = re.sub(r'(git\.CommitOptions\s*\{)(.*?)\}', inject_author, content, flags=re.DOTALL)
            if new_content != content:
                content = new_content
                modified = True

            if modified:
                with open(go_file, "w") as f: f.write(content)
                print(f"  [+] Patched Android ENOSYS Commit bug in {go_file}")

def force_auth_injection_and_initial_sync():
    for go_file in glob.glob("backend/*.go"):
        with open(go_file, "r") as f: content = f.read()

        m = re.search(r'([a-zA-Z0-9_]+)(?:,\s*[a-zA-Z0-9_]+)?\s*:=\s*(?:backend\.)?GetInsecureSSHAuth', content)
        if not m:
            m = re.search(r'([a-zA-Z0-9_]+),\s*[a-zA-Z0-9_]+\s*:=\s*ssh\.NewPublicKeysFromFile', content)
        
        if m:
            auth_var = m.group(1)
            modified = False
            
            for opt in ['PullOptions', 'PushOptions', 'CloneOptions', 'FetchOptions']:
                if f"&git.{opt}{{" in content or f"git.{opt}{{" in content:
                    content = re.sub(rf'\s*Auth:\s*[a-zA-Z0-9_.]+,\s*', '\n\t\t', content)
                    content = re.sub(rf'\s*Force:\s*(true|false),\s*', '\n\t\t', content)
                    
                    # Force: true ensures initial empty-directory syncs succeed against remote histories
                    force_str = r'\n\t\tForce: true,' if opt in ['PullOptions', 'PushOptions', 'FetchOptions'] else ''
                    injection = r'\1\n\t\tAuth: ' + auth_var + r',' + force_str
                    
                    content = re.sub(r'(git\.' + opt + r'\s*\{)', injection, content)
                    modified = True
            
            if modified:
                with open(go_file, "w") as f: f.write(content)
                print(f"  [+] Injected Auth & Initial Sync Force flags in {go_file}")

if __name__ == "__main__":
    bump_version()
    rebuild_git_helper_with_key_logger()
    fix_android_commit_enosys()
    force_auth_injection_and_initial_sync()
    print("[*] Update complete! Version 1.4.36 ready for compilation.")
    
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Fix: Resolve Android ENOSYS crash and enable Initial Force Pull")
    print("\n- Bumped application version to 1.4.36")
    print("- Injected explicit Author Signatures into git.CommitOptions to bypass")
    print("  Android os/user.Current() \"function not implemented\" crash.")
    print("- Enabled Force flag in PullOptions/PushOptions to guarantee smooth")
    print("  synchronization on completely fresh, empty Android installs.")
    print("="*55 + "\n")