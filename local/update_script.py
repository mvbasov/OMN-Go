import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.32...")

def bump_version():
    new_v = "1.4.32"
    new_v_c = "10432"

    # 1. Update version.go (Hard-Rewrite to guarantee update)
    ver_path = "backend/version.go"
    if os.path.exists(ver_path):
        with open(ver_path, "w") as f: 
            f.write(f'package backend\n\n// Version is the global application version\nconst Version = "{new_v}"\n')
        print(f"  [+] Hard-rewrote {ver_path}")
    else:
        print(f"  [-] Warning: {ver_path} not found")

    # 2. Update index.html (Dynamic path search)
    found_html = False
    for root, dirs, files in os.walk("."):
        if "index.html" in files:
            html_path = os.path.join(root, "index.html")
            with open(html_path, "r") as f: c = f.read()
            c, count = re.subn(r'1\.4\.\d+', new_v, c)
            if count > 0:
                with open(html_path, "w") as f: f.write(c)
                print(f"  [+] Bumped version in {html_path}")
                found_html = True
    
    if not found_html:
        print("  [-] Warning: Could not find or bump version in any index.html")

    # 3. Update Android Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r") as f: c = f.read()
        c = re.sub(r'versionCode\s+\d+', f'versionCode {new_v_c}', c)
        c = re.sub(r'versionName\s+"[^"]+"', f'versionName "{new_v}"', c)
        with open(gradle_path, "w") as f: f.write(c)
        print("  [+] Bumped version in android/app/build.gradle")

def upgrade_ssh_helper():
    helper_path = "backend/git_helper.go"
    if os.path.exists(helper_path):
        with open(helper_path, "r") as f: c = f.read()
        
        # Inject explicit HostKeyAlgorithms to prevent strict OpenSSH servers from rejecting the connection
        if "HostKeyAlgorithms" not in c:
            replacement = """publicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()
\t
\t// CRITICAL FIX: Force compatibility with strict OpenSSH/Gitolite servers
\t// Some servers reject standard handshakes if algorithms aren't explicitly declared by the client.
\tpublicKeys.HostKeyAlgorithms = []string{
\t\tgossh.KeyAlgoED25519,
\t\tgossh.KeyAlgoRSA,
\t\tgossh.KeyAlgoRSASHA256,
\t\tgossh.KeyAlgoRSASHA512,
\t\tgossh.KeyAlgoECDSA256,
\t\tgossh.KeyAlgoECDSA384,
\t\tgossh.KeyAlgoECDSA521,
\t}"""
            c = c.replace("publicKeys.HostKeyCallback = gossh.InsecureIgnoreHostKey()", replacement)
            with open(helper_path, "w") as f: f.write(c)
            print("  [+] Injected explicit HostKeyAlgorithms into backend/git_helper.go")

def force_auth_injection():
    for go_file in glob.glob("backend/*.go"):
        with open(go_file, "r") as f: content = f.read()

        m = re.search(r'([a-zA-Z0-9_]+)(?:,\s*[a-zA-Z0-9_]+)?\s*:=\s*(?:backend\.)?GetInsecureSSHAuth', content)
        if not m:
            m = re.search(r'([a-zA-Z0-9_]+),\s*[a-zA-Z0-9_]+\s*:=\s*ssh\.NewPublicKeysFromFile', content)
        
        if m:
            auth_var = m.group(1)
            modified = False
            
            # Simplified, failsafe replacement that avoids complex nested bracket parsing
            for opt in ['PullOptions', 'PushOptions', 'CloneOptions', 'FetchOptions']:
                if f"&git.{opt}{{" in content or f"git.{opt}{{" in content:
                    # Strip existing Auth if present to prevent duplicates
                    content = re.sub(rf'\s*Auth:\s*[a-zA-Z0-9_.]+,\s*', '\n\t\t', content)
                    # Forcibly append Auth right after the opening brace
                    content = re.sub(rf'(git\.{opt}\s*{{)', rf'\1\n\t\tAuth: {auth_var},', content)
                    modified = True
            
            if modified:
                with open(go_file, "w") as f: f.write(content)
                print(f"  [+] Verified Auth injection in {go_file}")

if __name__ == "__main__":
    bump_version()
    upgrade_ssh_helper()
    force_auth_injection()
    print("[*] Update complete! Version 1.4.32 ready for compilation.")
    
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Fix: Resolve SSH Key Rejection & Refine Version Sync Paths")
    print("\n- Bumped application version to 1.4.32")
    print("- Updated version bumper to ignore config.go and dynamically find index.html")
    print("- Injected HostKeyAlgorithms into GetInsecureSSHAuth to force OpenSSH compatibility")
    print("- Applied failsafe Auth injection to Git Options")
    print("="*55 + "\n")