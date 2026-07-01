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
        return re.sub(r'APP_VERSION\s*=\s*".*?"', 'APP_VERSION = "1.6.1"', content)
    patch_file("backend/version.go", process_version)

    # Bump Android gradle version
    def process_gradle(content):
        content = re.sub(r'versionCode\s+\d+', 'versionCode 10601', content)
        content = re.sub(r'versionName\s+".*?"', 'versionName "1.6.1"', content)
        return content
    patch_file("android/app/build.gradle", process_gradle)

def refactor_backend():
    backend_dir = "backend"
    if not os.path.exists(backend_dir):
        print(f"[-] Directory {backend_dir} not found. Ensure you are in the project root.")
        return

    go_files = [f for f in os.listdir(backend_dir) if f.endswith(".go")]

    # 1. Dynamically discover ALL package-level functions across the entire backend
    target_funcs = set()
    for filename in go_files:
        filepath = os.path.join(backend_dir, filename)
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()
        
        # Matches standard `func FuncName(` while strictly ignoring methods `func (s *Struct) FuncName(`
        funcs = re.findall(r'(?m)^func\s+([A-Za-z0-9_]+)\s*\(', content)
        target_funcs.update(funcs)

    # 2. Exclude entrypoints that the OS or Go HTTP Standard Library must call directly
    target_funcs -= {"StartServer", "init", "main"}

    for filename in go_files:
        filepath = os.path.join(backend_dir, filename)
        with open(filepath, "r", encoding="utf-8") as f:
            content = f.read()

        # 3. Eliminate Global Variables natively
        for var_name in ["appConfig", "storageDir", "activeConns", "connMutex", "gitMutex"]:
            content = re.sub(r'(?m)^var\s+' + var_name + r'\b.*$', '', content)
        
        content = re.sub(r'(?m)^\s*appConfig\s+\*?Config.*$', '', content)
        content = re.sub(r'(?m)^\s*storageDir\s+string.*$', '', content)
        content = re.sub(r'(?m)^\s*activeConns\s+(?:int64|int32|int).*$', '', content)
        content = re.sub(r'(?m)^\s*connMutex\s+sync\.Mutex.*$', '', content)
        content = re.sub(r'(?m)^\s*gitMutex\s+sync\.Mutex.*$', '', content)
        content = re.sub(r'var\s*\(\s*\)', '', content)

        # 4. Inject App Struct into server.go (With Value-Type Config Fix)
        if filename == "server.go":
            if "type App struct" not in content:
                app_struct = "\n\n// App encapsulates the global state for the backend\ntype App struct {\n\tConfig      Config\n\tStorageDir  string\n\tActiveConns int64\n\tConnMutex   sync.Mutex\n\tGitMutex    sync.Mutex\n\tRouter      *http.ServeMux\n}\n"
                content = re.sub(r'(import \(\n(?:.*?\n)*?\)\n)', r'\1' + app_struct, content, count=1)
                if '"sync"' not in content:
                    content = re.sub(r'import \(\n', 'import (\n\t"sync"\n', content, count=1)
                if '"net/http"' not in content:
                    content = re.sub(r'import \(\n', 'import (\n\t"net/http"\n', content, count=1)
            else:
                # Fix Config type from previous run: *Config -> Config (Resolves the assignment panic)
                content = re.sub(r'Config\s+\*Config', 'Config Config', content)

            content = re.sub(r'func StartServer\((.*?)\bstorageDir\b(.*?)\)', r'func StartServer(\1initStorageDir\2)', content)
            
            # Ensure safe localized initialization
            if "a := &App{" not in content:
                content = re.sub(r'func StartServer\((.*?)\)\s*\{',
                                 r'func StartServer(\1) {\n\ta := &App{\n\t\tRouter: http.NewServeMux(),\n\t}\n', content)

        # 5. Transform global usages into App struct fields
        content = re.sub(r'\bappConfig\b', 'a.Config', content)
        content = re.sub(r'\bstorageDir\b', 'a.StorageDir', content)
        content = re.sub(r'\bactiveConns\b', 'a.ActiveConns', content)
        content = re.sub(r'\bconnMutex\b', 'a.ConnMutex', content)
        content = re.sub(r'\bgitMutex\b', 'a.GitMutex', content)

        # Fix accidental parameter mutations
        content = re.sub(r'\(a\.StorageDir\s+string\)', '(storageDir string)', content)
        content = re.sub(r'a\.StorageDir\s+string', 'storageDir string', content)
        content = re.sub(r'a\.Config\s+\*?Config', 'appConfig *Config', content)
        content = re.sub(r'a\.ActiveConns\s+(?:int64|int32|int)', 'activeConns int64', content)
        content = re.sub(r'a\.ConnMutex\s+sync\.Mutex', 'connMutex sync.Mutex', content)
        content = re.sub(r'a\.GitMutex\s+sync\.Mutex', 'gitMutex sync.Mutex', content)

        # 6. Apply `(a *App)` receivers to ALL discovered package-level functions
        for func_name in target_funcs:
            # Matches any indentation or spaces around the opening parenthesis
            content = re.sub(r'(?m)^func\s+' + func_name + r'\s*\(', r'func (a *App) ' + func_name + '(', content)

        # 7. Route internal function calls securely through the struct instance
        for func_name in target_funcs:
            # Multiple fixed-width lookbehinds to securely match function calls/references without double-prefixing
            content = re.sub(r'(?<!func )(?<!func \(a \*App\) )(?<!\.)\b' + func_name + r'\b', r'a.' + func_name, content)

        # 8. Re-wire DefaultServeMux patterns directly into App.Router
        content = re.sub(r'\bhttp\.HandleFunc\(', 'a.Router.HandleFunc(', content)
        content = re.sub(r'\bhttp\.Handle\(', 'a.Router.Handle(', content)
        content = re.sub(r'http\.ListenAndServe\(([^,]+),\s*nil\)', r'http.ListenAndServe(\1, a.Router)', content)

        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)

if __name__ == "__main__":
    print("[ ] Starting Dependency Injection Refactor (Version 1.6.1)...")
    bump_versions()
    refactor_backend()
    print("[+] Architecture successfully upgraded! Global variables eradicated.")
    print("[+] Run 'go build ./...' in the backend folder to verify the DI tree.")