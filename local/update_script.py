import os
import re

def apply_patch(filepath, old_str, new_str, description):
    print(f"\n[PATCH] {description}")
    print(f"  Target: {filepath}")
    
    if not os.path.exists(filepath):
        print(f"  [-] ERROR: File {filepath} not found!")
        return False
        
    with open(filepath, "r", encoding="utf-8") as f:
        content = f.read()
        
    if new_str in content:
        print("  [+] SUCCESS: Patch appears to be already applied from a previous run.")
        return True
        
    if old_str in content:
        content = content.replace(old_str, new_str)
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)
        print("  [+] SUCCESS: Exact string match replaced.")
        return True
        
    old_normalized = old_str.replace('\r\n', '\n')
    content_normalized = content.replace('\r\n', '\n')
    
    if old_normalized in content_normalized:
        content_normalized = content_normalized.replace(old_normalized, new_str.replace('\r\n', '\n'))
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content_normalized)
        print("  [+] SUCCESS: Normalized match replaced.")
        return True
        
    print("  [-] ERROR: Target string NOT FOUND!")
    return False

def bump_versions():
    print("\n[VERSION BUMP] Upgrading to 1.4.1")
    
    # Note: Since the refactoring, APP_VERSION is now housed in backend/config.go
    versions = [
        ("backend/config.go", 'APP_VERSION = "1.4.0"', 'APP_VERSION = "1.4.1"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.4.0";', 'const APP_VERSION = "1.4.1";'),
        ("android/app/build.gradle", 'versionCode 10400', 'versionCode 10401'),
        ("android/app/build.gradle", 'versionName "1.4.0"', 'versionName "1.4.1"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            if old not in content:
                print(f"  [~] {fp}: Exact old version string not found. Trying dynamic Regex bump...")
                if "build.gradle" in fp:
                    content = re.sub(r'versionCode\s+\d+', 'versionCode 10401', content)
                    content = re.sub(r'versionName\s+"1\.4\.\d+"', 'versionName "1.4.1"', content)
                else:
                    content = re.sub(r'APP_VERSION = "1\.4\.\d+"', 'APP_VERSION = "1.4.1"', content)
            else:
                content = content.replace(old, new)
                
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"  [+] Bumped version in {fp}")
        else:
            print(f"  [-] Skipped {fp} (File not found)")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.4.1)")
    print("==================================================")
    
    bump_versions()

    # 1. Wrap the StartServer call in a Goroutine so it doesn't block the browser intent
    old_main = r"""func main() {
	backend.StartServer()
	
	// Wait for server to bind"""
    
    new_main = r"""func main() {
	go backend.StartServer()
	
	// Wait for server to bind"""
    
    apply_patch("main_desktop.go", old_main, new_main, "Execute StartServer as a Goroutine to unblock browser auto-launch")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "fix(desktop): run backend.StartServer() in a goroutine to unblock native browser auto-launch\n\nVersion bumped to 1.4.1"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()