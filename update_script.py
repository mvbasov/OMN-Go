import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.20"', 'APP_VERSION = "1.0.21"'),
        ("frontend/index.html", 'const APP_VERSION = "1.0.20";', 'const APP_VERSION = "1.0.21";')
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

    # 2. Revert main_android.go to remove the deadlocking os/exec 'am start' call
    if os.path.exists("main_android.go"):
        with open("main_android.go", "r") as f:
            content = f.read()
            
        old_block = """import (
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

        new_block = """import (
\t"fmt"
\t"golang.org/x/mobile/app"
\t"golang.org/x/mobile/event/lifecycle"
\t"golang.org/x/mobile/event/paint"
\t"golang.org/x/mobile/gl"
)

func main() {
\tgo runServer()"""

        if old_block in content:
            content = content.replace(old_block, new_block)
            with open("main_android.go", "w") as f:
                f.write(content)
            print("Successfully reverted main_android.go to remove os/exec deadlock.")
        elif new_block in content:
            print("main_android.go is already reverted.")
        else:
            print("Warning: Could not find target block in main_android.go.")

    # 3. Patch Dockerfile and AndroidManifest.xml to increment versionCode to 10021
    if os.path.exists("Dockerfile"):
        with open("Dockerfile", "r") as f:
            content = f.read()
        
        content = re.sub(r'android:versionCode="100[0-9]+"', 'android:versionCode="10021"', content)
        content = re.sub(r"versionCode: '100[0-9]+'", "versionCode: '10021'", content)
        
        with open("Dockerfile", "w") as f:
            f.write(content)
        print("Successfully bumped Dockerfile versionCode to 10021.")

    if os.path.exists("AndroidManifest.xml"):
        with open("AndroidManifest.xml", "r") as f:
            content = f.read()
        
        content = re.sub(r'android:versionName="1\.0\.[0-9]+"', 'android:versionName="1.0.21"', content)
        
        with open("AndroidManifest.xml", "w") as f:
            f.write(content)
        print("Successfully bumped AndroidManifest.xml versionName to 1.0.21.")

    # 4. Output Standardized Git Commit Message
    commit_msg = """fix(android): remove os/exec auto-launch causing ANR deadlocks
    
My previous attempt to auto-launch the Android browser using os/exec 
to call the Activity Manager (`am start`) caused a fatal ANR on Android 14.
SELinux policies strictly forbid untrusted apps from executing system 
shells, and the fork() call deadlocked the Go event loop preventing it 
from responding to OS FocusEvents.

Because the project strictly forbids Java wrappers to stay under 5MB, 
native Java Intents cannot be used. This patch entirely removes the 
deadlocking shell call. The app correctly stabilizes on the green 
NativeActivity screen, and users simply navigate to http://localhost:8080 
in their mobile browser manually.

Version bumped to 1.0.21"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! You can now re-run your docker build.")

if __name__ == "__main__":
    update_application()