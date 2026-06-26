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

def extract_and_remove_function(content, func_name):
    """
    AST-style parser that finds a function, extracts its exact body mathematically 
    using brace counting, and seamlessly removes it from the parent source file.
    """
    start_idx = content.find(f"func {func_name}(")
    if start_idx == -1:
        return content, ""

    brace_start = content.find("{", start_idx)
    if brace_start == -1:
        raise ValueError(f"No opening brace found for {func_name}")

    brace_count = 1
    idx = brace_start + 1
    while idx < len(content) and brace_count > 0:
        if content[idx] == '{':
            brace_count += 1
        elif content[idx] == '}':
            brace_count -= 1
        idx += 1

    end_idx = idx
    func_code = content[start_idx:end_idx]
    
    # Remove it completely from the original content, bridging the gap gracefully
    prefix = content[:start_idx].rstrip()
    suffix = content[end_idx:].lstrip()
    new_content = prefix + "\n\n" + suffix
    return new_content, func_code

def update_application():
    print("[ ] Starting migration of Git logic to git_helper.go")
    
    # 1. Auto-detect and bump version to 1.4.62
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

    # 3. Target the specific modularized functions to evacuate from handlers.go
    handlers_path = "backend/handlers.go"
    handlers_code = read_file(handlers_path)

    funcs_to_move = [
        "ensureGitignore",
        "getOrInitRepo",
        "getSSHAuth",
        "commitLocalChanges",
        "executeSyncDownload",
        "executeSyncUpload"
    ]

    extracted_codes = []
    for fn in funcs_to_move:
        handlers_code, code = extract_and_remove_function(handlers_code, fn)
        if code:
            extracted_codes.append(code)
            print(f"[+] Successfully extracted {fn}()")
        else:
            print(f"[-] Function {fn} not found in handlers.go (already moved?)")

    if extracted_codes:
        # 4. Strip unused 'go-git' imports from handlers.go to prevent Go compiler panics
        # This regex safely targets only the go-git/v5 paths regardless of aliases
        handlers_code = re.sub(r'(?m)^\s*(?:[a-zA-Z0-9_]+\s+)?"github\.com/go-git/go-git/v5(?:/[^"]*)?"\s*\n', '', handlers_code)
        
        # Clean up any empty import blocks we might have created
        handlers_code = re.sub(r'import\s*\(\s*\)', '', handlers_code)

        write_file(handlers_path, handlers_code)
        print("[+] Cleaned up unused imports in backend/handlers.go")

        # 5. Hydrate backend/git_helper.go
        git_helper_content = """package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	gitconfig "github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/transport"
)

""" + "\n\n".join(extracted_codes) + "\n"

        write_file("backend/git_helper.go", git_helper_content)
        print("[+] Generated backend/git_helper.go with extracted Git plumbing logic")
    else:
        print("[-] No functions were extracted.")

    commit_msg = (
        "refactor(git): migrate git logic to modular git_helper.go\n\n"
        "- Extracted newly decoupled git functions from handlers.go\n"
        "- Created backend/git_helper.go to isolate go-git library usage\n"
        "- Removed orphaned go-git import paths from handlers.go\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()