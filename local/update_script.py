import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.30...")

def bump_version():
    new_v = "1.4.30"
    new_v_c = "10430"

    # 1. Update config.go
    cfg_path = "backend/config.go"
    if os.path.exists(cfg_path):
        with open(cfg_path, "r") as f: c = f.read()
        c = re.sub(r'Version\s*=\s*"[^"]+"', f'Version = "{new_v}"', c)
        with open(cfg_path, "w") as f: f.write(c)
        print("  [+] Bumped version in backend/config.go")

    # 2. Update version.go (Explicit regex override)
    ver_path = "backend/version.go"
    if os.path.exists(ver_path):
        with open(ver_path, "r") as f: c = f.read()
        c = re.sub(r'Version\s*=\s*"[^"]+"', f'Version = "{new_v}"', c)
        with open(ver_path, "w") as f: f.write(c)
        print("  [+] Bumped version in backend/version.go")

    # 3. Update frontend
    html_path = "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, "r") as f: c = f.read()
        c = re.sub(r'1\.4\.\d+', new_v, c)
        with open(html_path, "w") as f: f.write(c)
        print("  [+] Bumped version in backend/frontend/index.html")

    # 4. Update Android Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r") as f: c = f.read()
        c = re.sub(r'versionCode\s+\d+', f'versionCode {new_v_c}', c)
        c = re.sub(r'versionName\s+"[^"]+"', f'versionName "{new_v}"', c)
        with open(gradle_path, "w") as f: f.write(c)
        print("  [+] Bumped version in android/app/build.gradle")

def fix_git_auth_options_robust():
    print("  [*] Performing robust injection for go-git Auth fields...")
    for go_file in glob.glob("backend/*.go"):
        with open(go_file, "r") as f:
            content = f.read()

        # Dynamically find the auth variable name
        m = re.search(r'([a-zA-Z0-9_]+)(?:,\s*[a-zA-Z0-9_]+)?\s*:=\s*(?:backend\.)?GetInsecureSSHAuth', content)
        if not m:
            m = re.search(r'([a-zA-Z0-9_]+),\s*[a-zA-Z0-9_]+\s*:=\s*ssh\.NewPublicKeysFromFile', content)
        
        if m:
            auth_var = m.group(1)
            modified = False
            
            for opt in ['PullOptions', 'PushOptions', 'CloneOptions', 'FetchOptions']:
                # Bulletproof block regex that finds the struct regardless of & or spacing
                pattern = r'(git\.' + opt + r'\s*\{)(.*?)\}'
                
                def replacer(match):
                    nonlocal modified
                    modified = True
                    start = match.group(1)
                    inner = match.group(2)
                    # Strip any existing/broken Auth declaration
                    inner = re.sub(r'\s*Auth:\s*[a-zA-Z0-9_.]+,\s*', '\n\t\t', inner)
                    # Force inject the exact auth variable right after the opening brace
                    return start + f'\n\t\tAuth: {auth_var},' + inner + '}'
                
                content = re.sub(pattern, replacer, content, flags=re.DOTALL)
            
            if modified:
                with open(go_file, "w") as f:
                    f.write(content)
                print(f"  [+] Forced 'Auth: {auth_var}' into all network operations in {go_file}")

if __name__ == "__main__":
    bump_version()
    fix_git_auth_options_robust()
    print("[*] Update complete! Version 1.4.30 ready for compilation.")
    
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Fix: Robust regex injection for go-git SSH Auth options")
    print("\n- Bumped application version to 1.4.30 (including version.go)")
    print("- Replaced brittle target matching with block-level overrides")
    print("- Guaranteed Auth injection into Pull/Push structs to fix none fallback")
    print("="*55 + "\n")