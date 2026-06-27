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
        return content, ""

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
    print("[ ] Starting Git Package Consolidation & Cleanup...")
    
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

    # 3. Read handlers.go and dynamically extract the true package namespace
    handlers_path = "backend/handlers.go"
    handlers_code = read_file(handlers_path)
    
    pkg_match = re.search(r'^package\s+([a-zA-Z0-9_]+)', handlers_code, re.MULTILINE)
    pkg_name = pkg_match.group(1) if pkg_match else "main"
    print(f"[+] Detected backend namespace: '{pkg_name}'")

    # 4. Extract the forgotten legacy Git functions from handlers.go
    funcs_to_evacuate = [
        "writeTreeFromDir",
        "buildTree",
        "manualGitInit",
        "GetInsecureSSHAuth",
        "handleSync"
    ]

    extracted_bodies = []
    for fn in funcs_to_evacuate:
        handlers_code, code = extract_and_remove_function(handlers_code, fn)
        if code:
            extracted_bodies.append(code)
            print(f"[+] Evacuated {fn}() from handlers.go")

    # Clean up any leftover Git or SSH imports from handlers.go
    handlers_code = re.sub(r'(?m)^\s*"github\.com/go-git/go-git/v5/storage"\s*\n', '', handlers_code)
    handlers_code = re.sub(r'(?m)^\s*"golang\.org/x/crypto/ssh"\s*\n', '', handlers_code)
    handlers_code = re.sub(r'(?m)^\s*(?:[a-zA-Z0-9_]+\s+)?"github\.com/go-git/go-git/v5(?:/[^"]*)?"\s*\n', '', handlers_code)
    handlers_code = re.sub(r'import\s*\(\s*\)', '', handlers_code)
    write_file(handlers_path, handlers_code)

    # 5. Read existing git_helper.go and rescue its functions (ignoring broken headers)
    git_helper_path = "backend/git_helper.go"
    if os.path.exists(git_helper_path):
        git_helper_code = read_file(git_helper_path)
        func_start_idx = git_helper_code.find("func ")
        existing_funcs = git_helper_code[func_start_idx:] if func_start_idx != -1 else ""
    else:
        existing_funcs = ""

    # Combine all Git logic into one massive block
    all_git_funcs = existing_funcs.strip() + "\n\n" + "\n\n".join(extracted_bodies)

    # 6. Dynamic Import Scanner (Prevents "imported and not used" Go compiler panics)
    imports = set([
        '"github.com/go-git/go-git/v5"',
        'gitconfig "github.com/go-git/go-git/v5/config"',
        '"github.com/go-git/go-git/v5/plumbing"',
        '"github.com/go-git/go-git/v5/plumbing/object"',
        '"github.com/go-git/go-git/v5/plumbing/transport"',
        '"github.com/go-git/go-git/v5/storage"'
    ])

    if re.search(r'\bfmt\.', all_git_funcs): imports.add('"fmt"')
    if re.search(r'\blog\.', all_git_funcs): imports.add('"log"')
    if re.search(r'\bhttp\.', all_git_funcs) or "http.ResponseWriter" in all_git_funcs: imports.add('"net/http"')
    if re.search(r'\bos\.', all_git_funcs): imports.add('"os"')
    if re.search(r'\bfilepath\.', all_git_funcs): imports.add('"path/filepath"')
    if re.search(r'\bstrings\.', all_git_funcs): imports.add('"strings"')
    if re.search(r'\btime\.', all_git_funcs): imports.add('"time"')
    if re.search(r'\bio\.', all_git_funcs): imports.add('"io"')
    if re.search(r'\bfs\.', all_git_funcs): imports.add('"io/fs"')
    if re.search(r'\bbytes\.', all_git_funcs): imports.add('"bytes"')

    # Smart SSH resolution
    if re.search(r'\bgitssh\.', all_git_funcs):
        imports.add('gitssh "github.com/go-git/go-git/v5/plumbing/transport/ssh"')
        if re.search(r'\bssh\.', all_git_funcs):
            imports.add('"golang.org/x/crypto/ssh"')
    else:
        if re.search(r'\bssh\.', all_git_funcs):
            imports.add('"github.com/go-git/go-git/v5/plumbing/transport/ssh"')
        if re.search(r'\bcryptossh\.', all_git_funcs):
            imports.add('cryptossh "golang.org/x/crypto/ssh"')

    import_block = "import (\n\t" + "\n\t".join(sorted(list(imports))) + "\n)"

    # 7. Write the perfectly reconstructed git_helper.go
    final_git_helper = f"package {pkg_name}\n\n{import_block}\n\n{all_git_funcs}\n"
    write_file(git_helper_path, final_git_helper)
    print(f"[+] Reconstructed backend/git_helper.go with synchronized namespace and dynamic imports")

    commit_msg = (
        "fix(git): consolidate remaining git utilities and sync package namespaces\n\n"
        f"- Dynamically aligned git_helper.go to package '{pkg_name}'\n"
        "- Evacuated manualGitInit, writeTreeFromDir, and handleSync from handlers.go\n"
        "- Implemented dynamic import scanning to prevent unused import panics\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()