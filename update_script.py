import os

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.19"', 'APP_VERSION = "1.0.20"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.19";', 'const APP_VERSION = "1.0.20";')
    ]
    
    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, 'r') as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, 'w') as f:
                    f.write(content)
            elif new_val not in content:
                print(f"Warning: Could not find '{old_val}' in {filepath}")

    # 2. Patch main_android.go to auto-launch the web browser using the Android Activity Manager
    if os.path.exists("main_android.go"):
        with open("main_android.go", "r") as f:
            content = f.read()
            
        old_block = """import (
\t"fmt"
\t"golang.org/x/mobile/app"
\t"golang.org/x/mobile/event/lifecycle"
\t"golang.org/x/mobile/event/paint"
\t"golang.org/x/mobile/gl"
)

func main() {
\tgo runServer()"""

        new_block = """import (
\t"fmt"
\t"os/exec"
\t"time"
\t"golang.org/x/mobile/app"
\t"golang.org/x/mobile/event/lifecycle"
\t"golang.org/x/mobile/event/paint"
\t"golang.org/x/mobile/gl"
)

func main() {
\tgo runServer()

\t// Automatically launch the Android default browser to view the UI
\tgo func() {
\t\ttime.Sleep(1 * time.Second)
\t\turl := fmt.Sprintf("http://localhost:%d", appConfig.ServerPort)
\t\texec.Command("am", "start", "-a", "android.intent.action.VIEW", "-d", url).Start()
\t}()"""

        if old_block in content:
            content = content.replace(old_block, new_block)
            with open("main_android.go", "w") as f:
                f.write(content)
            print("Successfully patched main_android.go with auto-launch intent.")
        elif "os/exec" in content and "am start" in content:
            print("main_android.go is already patched with the auto-launch intent.")
        else:
            print("Warning: Could not find target block in main_android.go.")

    # 3. Patch Dockerfile to increment versionCode logically to 10020
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        if "10019" in content:
            content = content.replace("10019", "10020")
            with open("Dockerfile", "w") as f:
                f.write(content)
            print("Successfully bumped Dockerfile versionCode to 10020.")
        elif "10020" in content:
            print("Dockerfile is already updated to versionCode 10020.")

    # 4. Output Standardized Git Commit Message
    commit_msg = """feat(android): implement automatic browser launch via activity manager
    
Because the app uses a pure NativeActivity without WebView/AppCompat to stay 
under 5MB, users were left on a green placeholder screen while the HTTP 
server ran invisibly in the background. 

This patch mirrors the desktop behavior by utilizing os/exec to call Android's 
native Activity Manager (`am start`). One second after the server binds, the 
app automatically fires an ACTION_VIEW intent, seamlessly popping open the 
user's default mobile web browser directly to the local GoOMN UI.

Version bumped to 1.0.20"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()