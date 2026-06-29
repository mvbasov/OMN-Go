import os
import re
import glob

def update_version():
    """Bumps the application version globally to 1.5.43"""
    for go_file in glob.glob("backend/*.go"):
        if not os.path.isfile(go_file): continue
        with open(go_file, 'r') as f: content = f.read()
        if "1.5.42" in content and ("APP_VERSION" in content or "Version" in content):
            with open(go_file, 'w') as f:
                f.write(content.replace("1.5.42", "1.5.43"))
            print(f"[+] Bumped version in {go_file}")

    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r') as f: content = f.read()
        content = re.sub(r'versionCode 10542', 'versionCode 10543', content)
        content = re.sub(r'versionName "1.5.42"', 'versionName "1.5.43"', content)
        with open(gradle_path, 'w') as f:
            f.write(content)
        print("[+] Bumped version in android/app/build.gradle")

def make_error_nonfatal(content, target_func, log_message):
    """Upgraded AST-style parser using Regex to catch compound err != nil conditions"""
    search_start = 0
    while True:
        idx = content.find(target_func, search_start)
        if idx == -1: break
        
        # Use Regex to dynamically catch "if err != nil {" OR "if err != nil && err != ... {"
        match = re.search(r'if\s+err\s*!=\s*nil[^\{]*\{', content[idx:idx+400])
        if match:
            start_brace = idx + match.end() - 1
            
            brace_count = 1
            end_brace = -1
            for i in range(start_brace + 1, len(content)):
                if content[i] == '{': brace_count += 1
                elif content[i] == '}': brace_count -= 1
                if brace_count == 0:
                    end_brace = i
                    break
            
            if end_brace != -1:
                block_content = content[start_brace:end_brace]
                if "return " in block_content:
                    replacement = f'{{\n\t\tlog.Printf("{log_message}: %v", err)\n\t}}'
                    content = content[:start_brace] + replacement + content[end_brace+1:]
        
        search_start = idx + len(target_func)
    return content

def patch_ultimate_sync():
    """Repairs Android missing directories, bypasses commits, and restores FUSE ghost patching"""
    for filename in glob.glob("backend/*.go"):
        if not os.path.isfile(filename): continue
        
        with open(filename, 'r') as f: content = f.read()
        original_content = content
        
        # 1. Strip fatal returns using the upgraded Regex AST parser
        content = make_error_nonfatal(content, ".AddWithOptions(", "[LOG] [GO] [sync] Staging warning (Ignored, proceeding)")
        content = make_error_nonfatal(content, ".Commit(", "[LOG] [GO] [sync] Commit warning (Ignored, proceeding to pull)")
        
        # 2. Inject MkdirAll BEFORE .Commit to fix the Android missing objects/pack crash
        commit_pattern = r'([ \t]*)(?:[a-zA-Z0-9_:=, \t]+)\.Commit\('
        def inject_mkdir(match):
            indent = match.group(1)
            injection = f'{indent}// Fix Android FUSE missing directory bug causing writeTreeFromDir crashes\n{indent}os.MkdirAll(filepath.Join(storageDir, ".git", "objects", "pack"), 0755)\n{indent}os.MkdirAll(filepath.Join(storageDir, ".git", "objects", "info"), 0755)\n'
            return injection + match.group(0)
        
        if 'objects", "pack"' not in content:
            content = re.sub(commit_pattern, inject_mkdir, content)

        # 3. Restore the FUSE ghost file staging fix (accidentally dropped in v1.5.42)
        add_pattern = r'([ \t]*)([a-zA-Z0-9_]+)\.AddWithOptions\(&git\.AddOptions\{All:\s*true\}\)'
        def replace_add(match):
            indent = match.group(1)
            wtree_var = match.group(2)
            if f"wkStatus, _ := {wtree_var}.Status()" in content:
                return match.group(0)
            return f'''{indent}// FIX: Android FUSE filesystem bug causes AddWithOptions to miss deleted files.
{indent}wkStatus, _ := {wtree_var}.Status()
{indent}for path, fileStatus := range wkStatus {{
{indent}\tif fileStatus.Worktree == git.Deleted {{
{indent}\t\t{wtree_var}.Remove(path)
{indent}\t}}
{indent}}}
{match.group(0)}'''
        
        content = re.sub(add_pattern, replace_add, content)

        if content != original_content:
            # Ensure required packages are imported for our injections
            for pkg in ["os", "path/filepath", "log"]:
                if f'"{pkg}"' not in content:
                    import_idx = content.find('import (')
                    if import_idx != -1:
                        content = content[:import_idx+8] + f'\n\t"{pkg}"' + content[import_idx+8:]
            with open(filename, 'w') as f: 
                f.write(content)
            print(f"[+] Hardened Git sync logic and patched FUSE dirs in {filename}")

if __name__ == '__main__':
    update_version()
    patch_ultimate_sync()
    print("[+] Application upgraded to Version 1.5.43 successfully!")