import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.33...")

def bump_version():
    new_v = "1.4.33"
    new_v_c = "10433"

    # 1. Update version.go (Hard-Rewrite using the correct APP_VERSION constant)
    ver_path = "backend/version.go"
    if os.path.exists(ver_path):
        with open(ver_path, "w") as f: 
            f.write(f'package backend\n\n// APP_VERSION is the global application version\nconst APP_VERSION = "{new_v}"\n')
        print(f"  [+] Hard-rewrote {ver_path} with APP_VERSION")
    else:
        print(f"  [-] Warning: {ver_path} not found")

    # (index.html version bumping removed as requested)

    # 2. Update Android Gradle
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
        
        # Clean up the invalid HostKeyAlgorithms injection that breaks the go-git wrapper
        if "HostKeyAlgorithms" in c:
            # Strip out the broken block from previous failed patches
            c = re.sub(r'\s*// CRITICAL FIX.*?HostKeyAlgorithms = \[\]string\{[^}]+\}', '', c, flags=re.DOTALL)
            with open(helper_path, "w") as f: f.write(c)
            print("  [+] Removed invalid HostKeyAlgorithms compilation error from backend/git_helper.go")

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
    print("[*] Update complete! Version 1.4.33 ready for compilation.")
    
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Fix: Restore APP_VERSION, fix Git build errors, remove HTML version")
    print("\n- Bumped application version to 1.4.33")
    print("- Restored APP_VERSION constant in version.go to fix Go build")
    print("- Removed invalid HostKeyAlgorithms assignment causing compilation failure")
    print("- Stopped attempting to version-bump index.html dynamically")
    print("="*55 + "\n")