import os
import re
import sys

VERSION = "1.5.8"
VERSION_CODE = "10508"

def read_file(filepath):
    if not os.path.exists(filepath):
        return None
    with open(filepath, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(filepath, content):
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[+] Successfully patched {filepath}")

def bump_versions():
    # 1. Bump version.go
    v_path = os.path.join("backend", "version.go")
    content = read_file(v_path)
    if content:
        content = re.sub(r'APP_VERSION\s*=\s*".*?"', f'APP_VERSION = "{VERSION}"', content)
        write_file(v_path, content)
    
    # 2. Bump build.gradle
    b_path = os.path.join("android", "app", "build.gradle")
    content = read_file(b_path)
    if content:
        content = re.sub(r'versionCode\s+\d+', f'versionCode {VERSION_CODE}', content)
        content = re.sub(r'versionName\s+".*?"', f'versionName "{VERSION}"', content)
        write_file(b_path, content)

def patch_git_helper():
    path = os.path.join("backend", "git_helper.go")
    content = read_file(path)
    if not content: 
        print(f"[-] Error: Could not find {path}")
        return

    # 1. Upgrade the Add function to respect .gitignore
    if '.AddWithOptions(&git.AddOptions{All: true})' not in content:
        content = re.sub(r'\b([a-zA-Z0-9_]+)\.Add\(\s*"\."\s*\)', r'\1.AddWithOptions(&git.AddOptions{All: true})', content)
        print("[+] Upgraded git Add() to respect .gitignore")

    # 2. Ensure the plumbing package is imported for the ZeroHash constant
    if '"github.com/go-git/go-git/v5/plumbing"' not in content:
        content = re.sub(r'(import\s*\()', r'\1\n\t"github.com/go-git/go-git/v5/plumbing"', content)

    # 3. Inject the safeCommit wrapper function at the bottom of the file
    if "func safeCommit" not in content:
        safe_commit_code = """
// [OMN-Go 1.5.8] Strong Empty Commit Check & Gitignore Enforcer
func safeCommit(w *git.Worktree, msg string, opts *git.CommitOptions) (plumbing.Hash, error) {
\tstatus, err := w.Status()
\tif err != nil {
\t\treturn plumbing.ZeroHash, err
\t}
\tif status.IsClean() {
\t\t// Strong check: explicitly bypass commit if tree is perfectly clean
\t\treturn plumbing.ZeroHash, nil
\t}
\treturn w.Commit(msg, opts)
}
"""
        content += "\n" + safe_commit_code
        print("[+] Injected safeCommit wrapper function")

    # 4. Replace direct w.Commit() calls with the safeCommit wrapper
    # This robustly protects against Go variable scope errors by passing the worktree as an argument
    if "safeCommit(" not in read_file(path): # Check original to see if we need to replace
        content = re.sub(r'\b([a-zA-Z0-9_]+)\.Commit\(', r'safeCommit(\1, ', content)
        print("[+] Rerouted direct git commits through the Strong Empty Check")

    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_git_helper()
    print("[*] Update complete. Rebuild the application to apply the Gitignore and Empty Commit fixes.")

if __name__ == "__main__":
    main()