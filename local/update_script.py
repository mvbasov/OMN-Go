#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    print("[ ] Starting Final Dependency Sync & Build Fix...")
    
    # 1. Auto-detect and bump version
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    if not match:
        raise ValueError("Version string not found in version.go")
        
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)
    print(f"[+] Bumped version in {ver_path} to {new_ver}")

    # 2. Bump Android configs
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}', f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)
    print(f"[+] Bumped version in {gradle_path}")

    # 3. Clean up orphaned "sort" import from handlers.go
    handlers_path = "backend/handlers.go"
    handlers_code = read_file(handlers_path)
    handlers_code = re.sub(r'(?m)^\s*"sort"\s*\n', '', handlers_code)
    handlers_code = re.sub(r'(?m)^\s*import\s+"sort"\s*\n', '', handlers_code)
    handlers_code = re.sub(r'import\s*\(\s*\)', '', handlers_code) # Clean empty blocks
    write_file(handlers_path, handlers_code)
    print("[+] Cleared unused 'sort' import from handlers.go")

    # 4. Read git_helper.go and parse its namespace
    git_helper_path = "backend/git_helper.go"
    git_helper_code = read_file(git_helper_path)
    
    pkg_match = re.search(r'^package\s+([a-zA-Z0-9_]+)', git_helper_code, re.MULTILINE)
    pkg_decl = pkg_match.group(0) if pkg_match else "package backend"

    # Remove existing import blocks safely so we can perfectly reconstruct them
    body_code = re.sub(r'import\s*\([\s\S]*?\)', '', git_helper_code)
    body_code = re.sub(r'import\s+"[^"]+"\n', '', body_code)
    body_code = body_code.replace(pkg_decl, "").strip()

    # 5. Inject missing GetConfigAuthor if it doesn't exist
    if "func GetConfigAuthor" not in body_code:
        author_func = """
func GetConfigAuthor() string {
\tif appConfig.Author != "" {
\t\treturn appConfig.Author
\t}
\treturn "OMN-Go User"
}
"""
        body_code += author_func
        print("[+] Injected missing GetConfigAuthor() fallback")

    # 6. Inject bulletproof GetInsecureSSHAuth if it doesn't exist
    if "func GetInsecureSSHAuth" not in body_code:
        ssh_auth_impl = """
func GetInsecureSSHAuth(user, keyPath, passphrase string) (transport.AuthMethod, error) {
\tpublicKeys, err := gitssh.NewPublicKeysFromFile(user, keyPath, passphrase)
\tif err != nil {
\t\treturn nil, err
\t}
\tpublicKeys.HostKeyCallbackHelper = gitssh.HostKeyCallbackHelper{
\t\tHostKeyCallback: cryptossh.InsecureIgnoreHostKey(),
\t}
\treturn publicKeys, nil
}
"""
        body_code += ssh_auth_impl
        print("[+] Injected robust GetInsecureSSHAuth() implementation")

    # 7. Dynamic Import Scanner (Now strictly enforcing 'sort' and 'crypto/ssh')
    imports = set([
        '"github.com/go-git/go-git/v5"',
        'gitconfig "github.com/go-git/go-git/v5/config"',
        '"github.com/go-git/go-git/v5/plumbing"',
        '"github.com/go-git/go-git/v5/plumbing/object"',
        '"github.com/go-git/go-git/v5/plumbing/transport"',
        '"github.com/go-git/go-git/v5/storage"'
    ])

    if re.search(r'\bfmt\.', body_code): imports.add('"fmt"')
    if re.search(r'\blog\.', body_code): imports.add('"log"')
    if re.search(r'\bhttp\.', body_code) or "http.ResponseWriter" in body_code: imports.add('"net/http"')
    if re.search(r'\bos\.', body_code): imports.add('"os"')
    if re.search(r'\bfilepath\.', body_code): imports.add('"path/filepath"')
    if re.search(r'\bstrings\.', body_code): imports.add('"strings"')
    if re.search(r'\btime\.', body_code): imports.add('"time"')
    if re.search(r'\bio\.', body_code): imports.add('"io"')
    if re.search(r'\bfs\.', body_code): imports.add('"io/fs"')
    if re.search(r'\bbytes\.', body_code): imports.add('"bytes"')
    if re.search(r'\bsort\.', body_code): imports.add('"sort"')

    # Smart SSH resolution (Guaranteed to trigger now because of the injected function)
    if re.search(r'\bgitssh\.', body_code):
        imports.add('gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"')
    if re.search(r'\bcryptossh\.', body_code):
        imports.add('cryptossh "golang.org/x/crypto/ssh"')

    import_block = "import (\n\t" + "\n\t".join(sorted(list(imports))) + "\n)"

    # 8. Write the finalized file
    final_git_helper = f"{pkg_decl}\n\n{import_block}\n\n{body_code}\n"
    write_file(git_helper_path, final_git_helper)
    print(f"[+] Reconstructed backend/git_helper.go with missing functions and correct imports")

    commit_msg = (
        "fix(git): resolve missing function dependencies and unused imports\n\n"
        "- Removed orphaned 'sort' import from handlers.go\n"
        "- Injected GetConfigAuthor safety fallback into git_helper.go\n"
        "- Injected robust GetInsecureSSHAuth implementation for go-git\n"
        "- Dynamically hydrated missing 'sort' and 'crypto/ssh' imports\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()