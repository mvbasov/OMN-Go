import os
import re

def update_application():
    print("Fixing appConfig scope in Desktop main (Version 1.0.23)")

    # 1. Bump Global Application Version
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.0.22"', 'APP_VERSION = "1.0.23"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.0.22";', 'const APP_VERSION = "1.0.23";'),
        ("android/app/build.gradle", 'versionCode 10022', 'versionCode 10023'),
        ("android/app/build.gradle", 'versionName "1.0.22"', 'versionName "1.0.23"')
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

    # 2. Patch backend/server.go to expose ServerPort and initialize storage synchronously
    if os.path.exists("backend/server.go"):
        with open("backend/server.go", "r") as f:
            content = f.read()
        
        old_start_server = """func StartServer() {
\tgo func() {
\t\tinitStorage()"""
        
        new_start_server = """func StartServer() {
\tinitStorage() // Execute synchronously to ensure config is loaded instantly
\tgo func() {"""
        
        if old_start_server in content:
            content = content.replace(old_start_server, new_start_server)

        old_end_server = """\t\thttp.ListenAndServe(port, connectionMiddleware(mux))
\t}()
}"""
        
        new_end_server = """\t\thttp.ListenAndServe(port, connectionMiddleware(mux))
\t}()
}

// GetServerPort safely exposes the configured port for frontend wrappers
func GetServerPort() int {
\treturn appConfig.ServerPort
}"""
        
        if old_end_server in content:
            content = content.replace(old_end_server, new_end_server)

        with open("backend/server.go", "w") as f:
            f.write(content)
        print("Patched backend/server.go to expose GetServerPort().")

    # 3. Patch main_desktop.go to use the exposed GetServerPort
    if os.path.exists("main_desktop.go"):
        with open("main_desktop.go", "r") as f:
            content = f.read()
        
        if "appConfig.ServerPort" in content:
            content = content.replace("appConfig.ServerPort", "backend.GetServerPort()")
            with open("main_desktop.go", "w") as f:
                f.write(content)
            print("Patched main_desktop.go to use backend.GetServerPort().")

    # 4. Output Standardized Git Commit Message
    commit_msg = """fix(desktop): resolve undefined appConfig compilation error
    
Moving the server logic into the `backend` package isolated the `appConfig` 
variable, causing `main_desktop.go` to fail compilation when trying to read 
the ServerPort for the browser auto-launch sequence.

This patch modifies `backend/server.go` to expose a `GetServerPort()` 
function and moves `initStorage()` out of the asynchronous goroutine block 
so the configuration is parsed immediately upon startup. `main_desktop.go` 
was updated to query this new exposed function.

Version bumped to 1.0.23"""
    
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")
    print("\nPatch applied successfully! Re-run your docker build command.")

if __name__ == "__main__":
    update_application()