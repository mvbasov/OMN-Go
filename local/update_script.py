import os
import re
import glob

def update_version():
    """Bumps the application version globally to 1.5.45"""
    for go_file in glob.glob("backend/*.go"):
        if not os.path.isfile(go_file): continue
        with open(go_file, 'r') as f: content = f.read()
        if "1.5.44" in content and ("APP_VERSION" in content or "Version" in content):
            with open(go_file, 'w') as f:
                f.write(content.replace("1.5.44", "1.5.45"))
            print(f"[+] Bumped version in {go_file}")

    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r') as f: content = f.read()
        content = re.sub(r'versionCode 10544', 'versionCode 10545', content)
        content = re.sub(r'versionName "1.5.44"', 'versionName "1.5.45"', content)
        with open(gradle_path, 'w') as f:
            f.write(content)
        print("[+] Bumped version in android/app/build.gradle")

def remove_garbage_function(content, func_name):
    """AST-style parser to physically rip out redeclared functions"""
    idx = content.find(f"func {func_name}()")
    if idx == -1: return content
    
    # Find the comment block directly above it
    comment_idx = content.rfind("// repairAndroidGitDirs fixes", 0, idx)
    start_idx = comment_idx if comment_idx != -1 and (idx - comment_idx) < 200 else idx
    
    start_brace = content.find("{", idx)
    if start_brace == -1: return content
    
    brace_count = 1
    end_brace = -1
    for i in range(start_brace + 1, len(content)):
        if content[i] == '{': brace_count += 1
        elif content[i] == '}': brace_count -= 1
        if brace_count == 0:
            end_brace = i
            break
            
    if end_brace != -1:
        # Erase the block and leave a clean newline
        return content[:start_idx].rstrip() + "\n\n" + content[end_brace+1:].lstrip()
    return content

def patch_clean_compilation():
    """Removes the duplicate functions and fixes the untyped nil syntax"""
    for filename in glob.glob("backend/*.go"):
        if not os.path.isfile(filename): continue
        
        with open(filename, 'r') as f: content = f.read()
        original_content = content
        
        # 1. Strip the garbage redeclaration from EVERY file EXCEPT git_helper.go
        if not filename.endswith("git_helper.go"):
            content = remove_garbage_function(content, "repairAndroidGitDirs")
            
        # 2. Fix the "untyped nil" syntax error dynamically
        # Converts illegal `err := nil` to a valid `var err error`
        content = re.sub(
            r'(\w+)\s*:=\s*nil\s*//\s*Clear legacy AddWithOptions error state',
            r'var \1 error // Clear legacy AddWithOptions error state',
            content
        )

        if content != original_content:
            with open(filename, 'w') as f: 
                f.write(content)
            print(f"[+] Cleaned garbage and fixed syntax in {filename}")

if __name__ == '__main__':
    update_version()
    patch_clean_compilation()
    print("[+] Application upgraded to Version 1.5.45 successfully!")