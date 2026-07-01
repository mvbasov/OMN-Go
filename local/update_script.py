import os
import re

def patch_file(filepath, processor):
    if not os.path.exists(filepath):
        return False
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()
    new_content = processor(content)
    if new_content != content:
        with open(filepath, 'w', encoding='utf-8') as f:
            f.write(new_content)
        print(f"[+] DI Patched: {filepath}")
        return True
    return False

def bump_versions():
    # Bump backend version
    def process_version(content):
        return re.sub(r'APP_VERSION\s*=\s*".*?"', 'APP_VERSION = "1.6.0"', content)
    patch_file("backend/version.go", process_version)

    # Bump Android gradle version
    def process_gradle(content):
        content = re.sub(r'versionCode\s+\d+', 'versionCode 10600', content)
        content = re.sub(r'versionName\s+".*?"', 'versionName "1.6.0"', content)
        return content
    patch_file("android/app/build.gradle", process_gradle)

def refactor_backend():
    backend_dir = "backend"
    if not os.path.exists(backend_dir):
        print(f"[-] Directory {backend_dir} not found. Ensure you are in the project root.")
        return

    go_files = [f for f in os.listdir(backend_dir) if f.endswith(".go")]

    # 1. Seed known backend methods & utilities
    target_funcs = {
        "getConfigPageBody", "ensureHeaderModified", "loadConfig", "saveConfig",
        "connectionCounter", "getOrInitRepo", "commitLocalChanges", "syncRepo",
        "SyncRepo", "repairAndroidGitDirs"
    }

    # 2. Dynamically discover all HTTP handlers across the backend
    for filename in go_files:
        filepath = os.path.join(backend_dir, filename)
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
        
        # Discover functions with http.ResponseWriter
        handlers = re.findall(r'^func\s+([a-zA-Z0-9_]+)\([^)]*http\.ResponseWriter', content, re.MULTILINE)
        target_funcs.update(handlers)
        
        # Discover functions prefixed with handle/serve
        action_funcs = re.findall(r'^func\s+(handle[A-Za-z0-9_]+|serve[A-Za-z0-9_]+)\(', content, re.MULTILINE)
        target_funcs.update(action_funcs)

    # Exclude Go entrypoints
    target_funcs -= {"StartServer", "init", "main"}

    for filename in go_files:
        filepath = os.path.join(backend_dir, filename)
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()

        # 3. Eliminate Global Variables natively
        content = re.sub(r'(?m)^\s*appConfig\s+\*?Config\s*\n?', '', content)
        content = re.sub(r'(?m)^\s*storageDir\s+string\s*\n?', '', content)
        content = re.sub(r'(?m)^\s*activeConns\s+(?:int64|int32|int)\s*\n?', '', content)
        content = re.sub(r'(?m)^\s*connMutex\s+sync\.Mutex\s*\n?', '', content)
        content = re.sub(r'(?m)^\s*gitMutex\s+sync\.Mutex\s*\n?', '', content)
        content = re.sub(r'(?m)^var\s+appConfig\s+\*?Config\s*\n?', '', content)
        content = re.sub(r'(?m)^var\s+storageDir\s+string\s*\n?', '', content)
        content = re.sub(r'(?m)^var\s*\(\s*\)\s*\n?', '', content) # Clean up empty var blocks

        # 4. Inject App Struct into server.go
        if filename == "server.go":
            if "type App struct" not in content:
                app_struct = "\n\n// App encapsulates the global state for the backend\ntype App struct {\n\tConfig      *Config\n\tStorageDir  string\n\tActiveConns int64\n\tConnMutex   sync.Mutex\n\tGitMutex    sync.Mutex\n\tRouter      *http.ServeMux\n}\n"
                content = re.sub(r'(import \(\n(?:.*?\n)*?\)\n)', r'\1' + app_struct, content, count=1)
                
                # Ensure missing core dependencies are imported
                if '"sync"' not in content:
                    content = re.sub(r'import \(\n', 'import (\n\t"sync"\n', content, count=1)
                if '"net/http"' not in content:
                    content = re.sub(r'import \(\n', 'import (\n\t"net/http"\n', content, count=1)

            # Prevent parameter shadowing during refactor
            content = re.sub(r'func StartServer\((.*?)\bstorageDir\b(.*?)\)', r'func StartServer(\1initStorageDir\2)', content)

            # Inject localized `a := &App{}` initialization
            content = re.sub(r'func StartServer\((.*?)\)\s*\{',
                             r'func StartServer(\1) {\n\ta := &App{\n\t\tRouter: http.NewServeMux(),\n\t}\n', content)

        # 5. Transform global usages into App struct fields (Whole word replacements)
        content = re.sub(r'\bappConfig\b', 'a.Config', content)
        content = re.sub(r'\bstorageDir\b', 'a.StorageDir', content)
        content = re.sub(r'\bactiveConns\b', 'a.ActiveConns', content)
        content = re.sub(r'\bconnMutex\b', 'a.ConnMutex', content)
        content = re.sub(r'\bgitMutex\b', 'a.GitMutex', content)

        # Revert parameter names if a struct field blindly modified a method signature
        content = re.sub(r'\(a\.StorageDir string\)', '(storageDir string)', content)
        content = re.sub(r'a\.StorageDir\s+string', 'storageDir string', content)

        # 6. Apply `(a *App)` receivers to all target handlers & utilities
        for func_name in target_funcs:
            content = re.sub(r'^func ' + func_name + r'\(', r'func (a *App) ' + func_name + '(', content, flags=re.MULTILINE)

        # 7. Route internal function calls securely through the struct instance
        for func_name in target_funcs:
            content = re.sub(r'(?<!func )(?<!func \(a \*App\) )(?<!a\.)\b' + func_name + r'\b', r'a.' + func_name, content)

        # 8. Re-wire DefaultServeMux patterns directly into App.Router
        content = re.sub(r'\bhttp\.HandleFunc\(', 'a.Router.HandleFunc(', content)
        content = re.sub(r'\bhttp\.Handle\(', 'a.Router.Handle(', content)
        content = re.sub(r'http\.ListenAndServe\(([^,]+),\s*nil\)', r'http.ListenAndServe(\1, a.Router)', content)

        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)

if __name__ == "__main__":
    print("[ ] Starting Dependency Injection Refactor (Version 1.6.0)...")
    bump_versions()
    refactor_backend()
    print("[+] Architecture successfully upgraded! Global variables eradicated.")
    print("[+] Run 'go build ./...' in the backend folder to verify the DI tree.")