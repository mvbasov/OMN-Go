import os
import re
import glob

print("[*] Upgrading OMN-Go to Version 1.4.29...")

def bump_version():
    old_v = "1.4.28"
    new_v = "1.4.29"
    old_v_c = "10428"
    new_v_c = "10429"

    files_to_update = [
        ("backend/config.go", old_v, new_v),
        ("backend/version.go", old_v, new_v), # Explicitly requested
        ("backend/frontend/index.html", old_v, new_v),
    ]

    for path, old, new in files_to_update:
        if os.path.exists(path):
            with open(path, "r") as f: 
                c = f.read()
            c = c.replace(old, new)
            with open(path, "w") as f: 
                f.write(c)
            print(f"  [+] Bumped version in {path}")

    # Update Android Gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r") as f: 
            c = f.read()
        c = c.replace(old_v_c, new_v_c)
        c = c.replace(old_v, new_v)
        with open(gradle_path, "w") as f: 
            f.write(c)
        print("  [+] Bumped version in android/app/build.gradle")

def fix_git_auth_options():
    print("  [*] Verifying go-git Options have Auth configured...")
    for go_file in glob.glob("backend/*.go"):
        with open(go_file, "r") as f:
            content = f.read()

        # Dynamically find the auth variable name (e.g., "auth, err := GetInsecureSSHAuth...")
        m = re.search(r'([a-zA-Z0-9_]+)(?:,\s*[a-zA-Z0-9_]+)?\s*:=\s*(?:backend\.)?GetInsecureSSHAuth', content)
        if not m:
            m = re.search(r'([a-zA-Z0-9_]+),\s*[a-zA-Z0-9_]+\s*:=\s*ssh\.NewPublicKeysFromFile', content)
        
        if m:
            auth_var = m.group(1)
            modified = False
            
            # Scan Pull, Push, Clone, and Fetch options
            for opt in ['PullOptions', 'PushOptions', 'CloneOptions', 'FetchOptions']:
                pattern = r'(&git\.' + opt + r'\s*\{)'
                parts = re.split(pattern, content)
                
                if len(parts) > 1:
                    new_content = parts[0]
                    for i in range(1, len(parts), 2):
                        block_start = parts[i]
                        block_rest = parts[i+1]
                        
                        # Check if Auth: is already in the struct literal
                        check_zone = block_rest[:250]
                        if 'Auth:' not in check_zone:
                            new_content += block_start + f'\n\t\tAuth: {auth_var},' + block_rest
                            modified = True
                        else:
                            new_content += block_start + block_rest
                    content = new_content
            
            if modified:
                with open(go_file, "w") as f:
                    f.write(content)
                print(f"  [+] Injected missing 'Auth: {auth_var}' into go-git network operations in {go_file}")

if __name__ == "__main__":
    bump_version()
    fix_git_auth_options()
    print("[*] Update complete! Version 1.4.29 ready for compilation.")
    
    # 2) Print requested commit message at the end
    print("\n" + "="*55)
    print("COMMIT MESSAGE TO USE:")
    print("Fix: Inject missing SSH Auth into go-git sync operations")
    print("\n- Bumped application version to 1.4.29")
    print("- Updated backend/version.go per requirements")
    print("- Injected Auth parameter into go-git PullOptions and PushOptions")
    print("  to prevent anonymous SSH fallback rejections.")
    print("="*55 + "\n")