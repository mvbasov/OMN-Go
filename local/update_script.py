#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Bump version
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_code = int(cur_ver.replace(".", ""))
    new_code = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {old_code}', f'versionCode {new_code}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Activate JSLogger and /api/logs in StartServer
    old_server_block = (
        '\tgo func() {\n'
        '\t\tdefer func() {\n'
        '\t\t\tif r := recover(); r != nil {\n'
        '\t\t\t\tlog.Printf("Recovered from panic in server: %v", r)\n'
        '\t\t\t}\n'
        '\t\t}()\n'
        '\n'
        '\t\tmux := http.NewServeMux()\n'
        '\t\tmux.HandleFunc("/", serveFrontend)'
    )
    new_server_block = (
        '\tgo func() {\n'
        '\t\tdefer func() {\n'
        '\t\t\tif r := recover(); r != nil {\n'
        '\t\t\t\tlog.Printf("Recovered from panic in server: %v", r)\n'
        '\t\t\t}\n'
        '\t\t}()\n'
        '\n'
        '\t\tmux := http.NewServeMux()\n'
        '\t\t// Initialize logger to stream Go logs to the frontend via SSE\n'
        '\t\tlog.SetOutput(&JSLogger{})\n'
        '\t\tmux.HandleFunc("/api/logs", HandleLogsSSE)\n'
        '\t\tmux.HandleFunc("/", serveFrontend)'
    )
    try:
        patch_file("backend/server.go", old_server_block, new_server_block)
        print("✅ Activated JSLogger and /api/logs endpoint in server.go.")
    except ValueError as e:
        print(f"⚠️ Server patch failed: {e}")

    # 3. Enhance commit error logging in handleSync
    old_commit_error = (
        '\t\t\t\tlog.Printf("[sync] Commit error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)'
    )
    new_commit_error = (
        '\t\t\t\tlog.Printf("[sync] Commit error: %v (type: %T)", err, err)\n'
        '\t\t\t\tlog.Printf("[sync] Commit options – Author: %v, Committer: %v", sig, sig)\n'
        '\t\t\t\tlog.Printf("[sync] Worktree status – IsClean: %v", status.IsClean())\n'
        '\t\t\t\t// Dump the underlying system error if it\'s a syscall error\n'
        '\t\t\t\tif errno, ok := err.(syscall.Errno); ok {\n'
        '\t\t\t\t\tlog.Printf("[sync] System call error number: %d", errno)\n'
        '\t\t\t\t}\n'
        '\t\t\t\tlog.Printf("[sync] Full error: %#v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)'
    )
    try:
        patch_file("backend/handlers.go", old_commit_error, new_commit_error)
        print("✅ Enhanced commit error logging in handleSync.")
    except ValueError as e:
        print(f"⚠️ Error logging patch failed: {e}")

    # 4. Add import for "syscall" in handlers.go
    import_slot = '\t"strings"\n'
    import_addition = '\t"strings"\n\t"syscall"\n'
    try:
        patch_file("backend/handlers.go", import_slot, import_addition)
        print("✅ Added syscall import.")
    except ValueError:
        print("⚠️ Could not add syscall import (maybe already present or strings not found).")

    # 5. Log before commit attempt
    old_commit_call = (
        '\t\t\t_, err = wTree.Commit("Local changes before sync", &git.CommitOptions{\n'
        '\t\t\t\tAuthor:    sig,\n'
        '\t\t\t\tCommitter: sig, // CRITICAL: both set to avoid os/user.Current() on Android\n'
        '\t\t\t})'
    )
    new_commit_call = (
        '\t\t\tlog.Printf("[sync] Attempting commit with author=%q, email=%q", sig.Name, sig.Email)\n'
        '\t\t\t_, err = wTree.Commit("Local changes before sync", &git.CommitOptions{\n'
        '\t\t\t\tAuthor:    sig,\n'
        '\t\t\t\tCommitter: sig, // CRITICAL: both set to avoid os/user.Current() on Android\n'
        '\t\t\t})'
    )
    try:
        patch_file("backend/handlers.go", old_commit_call, new_commit_call)
        print("✅ Added log before commit attempt.")
    except ValueError as e:
        print(f"⚠️ Pre-commit log patch failed: {e}")

    # 6. Commit message
    commit_msg = (
        "feat(logging): stream Go logs to WebView and add detailed debug info\n\n"
        "- Activate JSLogger in StartServer and expose /api/logs SSE endpoint\n"
        "- Log commit error type, syscall number, and commit options\n"
        "- Helps diagnose 'function not implemented' on Android\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()