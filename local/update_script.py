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
    print("\n[VERSION BUMP] Upgrading to 1.4.2")
    
    # Note: Since the refactoring, APP_VERSION is now housed in backend/config.go
    versions = [
        ("backend/config.go", 'APP_VERSION = "1.4.1"', 'APP_VERSION = "1.4.2"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.4.1";', 'const APP_VERSION = "1.4.2";'),
        ("android/app/build.gradle", 'versionCode 10401', 'versionCode 10402'),
        ("android/app/build.gradle", 'versionName "1.4.1"', 'versionName "1.4.2"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            if old not in content:
                print(f"  [~] {fp}: Exact old version string not found. Trying dynamic Regex bump...")
                if "build.gradle" in fp:
                    content = re.sub(r'versionCode\s+\d+', 'versionCode 10402', content)
                    content = re.sub(r'versionName\s+"1\.4\.\d+"', 'versionName "1.4.2"', content)
                else:
                    content = re.sub(r'APP_VERSION = "1\.4\.\d+"', 'APP_VERSION = "1.4.2"', content)
            else:
                content = content.replace(old, new)
                
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"  [+] Bumped version in {fp}")
        else:
            print(f"  [-] Skipped {fp} (File not found)")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.4.2)")
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

    # 2. Wrap the Android Java StartServer call in a background Thread
    old_java = r"""        // Start the Go Backend Server from the gomobile .aar
        Backend.startServer();"""
    
    new_java = r"""        // Start the Go Backend Server from the gomobile .aar in a background thread
        new Thread(new Runnable() {
            @Override
            public void run() {
                Backend.startServer();
            }
        }).start();"""
    
    apply_patch("android/app/src/main/java/net/basov/omngo/MainActivity.java", old_java, new_java, "Execute StartServer as a background thread to unblock Android UI")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "fix(android): run Backend.startServer() in a background thread to prevent ANR and unblock WebView\n\nVersion bumped to 1.4.2"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()