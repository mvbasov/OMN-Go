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

def replace_function_ast(content, func_name, new_code):
    start_idx = content.find(f"func {func_name}(")
    if start_idx == -1:
        return content, False
    
    brace_start = content.find("{", start_idx)
    if brace_start == -1:
        return content, False

    brace_count = 1
    idx = brace_start + 1
    while idx < len(content) and brace_count > 0:
        if content[idx] == '{':
            brace_count += 1
        elif content[idx] == '}':
            brace_count -= 1
        idx += 1

    end_idx = idx
    return content[:start_idx] + new_code + "\n" + content[end_idx:], True

def update_application():
    print("[ ] Starting Android FS flock() ENOSYS Bypass & Import Fix...")
    
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

    # 3. Read git_helper.go
    git_helper_path = "backend/git_helper.go"
    git_helper_code = read_file(git_helper_path)
    
    pkg_match = re.search(r'^package\s+([a-zA-Z0-9_]+)', git_helper_code, re.MULTILINE)
    pkg_decl = pkg_match.group(0) if pkg_match else "package backend"

    # Remove existing import blocks safely so we can reconstruct them with go-billy
    body_code = re.sub(r'import\s*\([\s\S]*?\)', '', git_helper_code)
    body_code = re.sub(r'import\s+"[^"]+"\n', '', body_code)
    body_code = body_code.replace(pkg_decl, "").strip()

    # 4. Inject the Custom Android FS Wrapper if it doesn't exist
    if "type NoLockFS struct" not in body_code:
        no_lock_impl = """
// --- Android Flock() Bypass ---
// Android's sdcardfs does not implement file locking (ENOSYS / function not implemented).
// This wrapper neutralizes Lock() calls gracefully across go-git operations.

type NoLockFS struct {
\tbilly.Filesystem
}

func (fs *NoLockFS) Create(filename string) (billy.File, error) {
\tf, err := fs.Filesystem.Create(filename)
\tif err != nil { return nil, err }
\treturn &NoLockFile{f}, nil
}

func (fs *NoLockFS) Open(filename string) (billy.File, error) {
\tf, err := fs.Filesystem.Open(filename)
\tif err != nil { return nil, err }
\treturn &NoLockFile{f}, nil
}

func (fs *NoLockFS) OpenFile(filename string, flag int, perm os.FileMode) (billy.File, error) {
\tf, err := fs.Filesystem.OpenFile(filename, flag, perm)
\tif err != nil { return nil, err }
\treturn &NoLockFile{f}, nil
}

func (fs *NoLockFS) TempFile(dir, prefix string) (billy.File, error) {
\tf, err := fs.Filesystem.TempFile(dir, prefix)
\tif err != nil { return nil, err }
\treturn &NoLockFile{f}, nil
}

func (fs *NoLockFS) Chroot(path string) (billy.Filesystem, error) {
\tc, err := fs.Filesystem.Chroot(path)
\tif err != nil { return nil, err }
\treturn &NoLockFS{c}, nil
}

type NoLockFile struct {
\tbilly.File
}

func (f *NoLockFile) Lock() error {
\treturn nil // Safely bypass Android flock ENOSYS
}

func (f *NoLockFile) Unlock() error {
\treturn nil // Safely bypass Android flock ENOSYS
}
"""
        body_code += no_lock_impl
        print("[+] Injected Android NoLockFS middleware wrapper")

    # 5. Overwrite getOrInitRepo to utilize the new NoLockFS Wrapper
    repo_init_code = """func getOrInitRepo() (*git.Repository, error) {
\tlog.Printf("[sync] Opening repo at %s", storageDir)
\t
\tbaseFS := osfs.New(storageDir)
\twtFS := &NoLockFS{baseFS}
\tdotFS, err := wtFS.Chroot(".git")
\tif err != nil {
\t\treturn nil, fmt.Errorf("chroot .git failed: %v", err)
\t}
\t
\tstorer := filesystem.NewStorage(dotFS, cache.NewObjectLRUDefault())
\trepo, err := git.Open(storer, wtFS)
\t
\tif err != nil {
\t\tlog.Printf("[sync] Repo not found, initializing...")
\t\tif initErr := manualGitInit(storageDir); initErr != nil {
\t\t\treturn nil, fmt.Errorf("manual init failed: %v", initErr)
\t\t}
\t\trepo, err = git.Open(storer, wtFS)
\t\tif err != nil {
\t\t\treturn nil, fmt.Errorf("failed to open manually created repo: %v", err)
\t\t}
\t\tlog.Printf("[sync] Repo initialized")
\t} else {
\t\tlog.Printf("[sync] Repo opened successfully")
\t}

\t_, err = repo.Remote("origin")
\tif err != nil {
\t\tlog.Printf("[sync] Remote origin missing, adding")
\t\t_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
\t\t\tName: "origin",
\t\t\tURLs: []string{appConfig.SyncRemote},
\t\t})
\t\tif err != nil {
\t\t\treturn nil, fmt.Errorf("remote add failed: %v", err)
\t\t}
\t}
\treturn repo, nil
}"""
    
    body_code, success = replace_function_ast(body_code, "getOrInitRepo", repo_init_code)
    if success:
        print("[+] Wired getOrInitRepo() to route through NoLockFS interceptor")
    else:
        print("[-] getOrInitRepo not found!")

    # 6. Dynamic Import Scanner (Adding storage, go-billy, cache)
    imports = set([
        '"github.com/go-git/go-git/v5"',
        'gitconfig "github.com/go-git/go-git/v5/config"',
        '"github.com/go-git/go-git/v5/plumbing"',
        '"github.com/go-git/go-git/v5/plumbing/object"',
        '"github.com/go-git/go-git/v5/plumbing/transport"',
        '"github.com/go-git/go-billy/v5"',
        '"github.com/go-git/go-billy/v5/osfs"',
        '"github.com/go-git/go-git/v5/storage"',             # <-- Restored missing storage
        '"github.com/go-git/go-git/v5/storage/filesystem"',
        '"github.com/go-git/go-git/v5/plumbing/cache"'
    ])

    if re.search(r'\bfmt\.', body_code): imports.add('"fmt"')
    if re.search(r'\blog\.', body_code): imports.add('"log"')
    if re.search(r'\bhttp\.', body_code) or "http.ResponseWriter" in body_code: imports.add('"net/http"')
    if re.search(r'\bos\.', body_code): imports.add('"os"')
    if re.search(r'\bfilepath\.', body_code): imports.add('"path/filepath"')
    if re.search(r'\bstrings\.', body_code): imports.add('"strings"')
    if re.search(r'\btime\.', body_code): imports.add('"time"')
    if re.search(r'\bio\.', body_code): imports.add('"io"')
    # Removed the buggy io/fs scanner line
    if re.search(r'\bbytes\.', body_code): imports.add('"bytes"')
    if re.search(r'\bsort\.', body_code): imports.add('"sort"')

    if re.search(r'\bgitssh\.', body_code):
        imports.add('gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"')
    if re.search(r'\bcryptossh\.', body_code):
        imports.add('cryptossh "golang.org/x/crypto/ssh"')

    import_block = "import (\n\t" + "\n\t".join(sorted(list(imports))) + "\n)"

    # 7. Write the finalized file
    final_git_helper = f"{pkg_decl}\n\n{import_block}\n\n{body_code}\n"
    write_file(git_helper_path, final_git_helper)
    print(f"[+] Reconstructed backend/git_helper.go with correct imports")

    commit_msg = (
        "fix(git): rectify import generation bugs in flock() bypass patch\n\n"
        "- Restored missing go-git/v5/storage package required by Storer\n"
        "- Removed overzealous io/fs regex that triggered unused import panic\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()