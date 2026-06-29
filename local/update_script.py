import os
import re
import glob

def update_version():
    """Bumps the application version globally to 1.5.39"""
    for go_file in glob.glob("backend/*.go"):
        if not os.path.isfile(go_file): continue
        with open(go_file, 'r') as f: content = f.read()
        if "1.5.38" in content and ("APP_VERSION" in content or "Version" in content):
            with open(go_file, 'w') as f:
                f.write(content.replace("1.5.38", "1.5.39"))
            print(f"[+] Bumped version in {go_file}")

    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r') as f: content = f.read()
        content = re.sub(r'versionCode 10538', 'versionCode 10539', content)
        content = re.sub(r'versionName "1.5.38"', 'versionName "1.5.39"', content)
        with open(gradle_path, 'w') as f:
            f.write(content)
        print("[+] Bumped version in android/app/build.gradle")

def patch_android_sync():
    """Fixes Android FUSE index bugs and prevents local commits from halting Force Pulls"""
    for filename in glob.glob("backend/*.go"):
        if not os.path.isfile(filename): continue
        
        with open(filename, 'r') as f: content = f.read()
        original_content = content
        
        # 1. Manual Index pruning to fix FUSE "entry not found"
        # We find the AddWithOptions line and inject a manual Worktree Status cleanup right above it
        pattern_add = r'([ \t]*)([^\n]+?)([a-zA-Z0-9_]+)\.AddWithOptions\(&git\.AddOptions\{All:\s*true\}\)'
        def replace_add(match):
            indent = match.group(1)
            full_prefix = match.group(1) + match.group(2)
            wtree_var = match.group(3)
            
            return f'''{indent}// FIX: Android FUSE filesystem bug causes AddWithOptions to miss deleted files.
{indent}wkStatus, _ := {wtree_var}.Status()
{indent}for path, fileStatus := range wkStatus {{
{indent}\tif fileStatus.Worktree == git.Deleted {{
{indent}\t\t{wtree_var}.Remove(path)
{indent}\t}} else if fileStatus.Worktree != git.Unmodified && fileStatus.Worktree != git.Untracked {{
{indent}\t\t{wtree_var}.Add(path)
{indent}\t}}
{indent}}}
{full_prefix}{wtree_var}.AddWithOptions(&git.AddOptions{{All: true}})'''

        content = re.sub(pattern_add, replace_add, content)

        # 2. Make local Commit non-fatal so sync can seamlessly proceed to Force Pull
        # Converts the hard `return fmt.Errorf(...)` into a simple bypassed warning log
        content = re.sub(
            r'return\s+fmt\.Errorf\([^)]*Commit failed[^)]*\)',
            r'log.Printf("[LOG] [GO] [sync] Local commit skipped (Ignored, proceeding to pull): %v", err)',
            content
        )

        # Ensure "log" package is imported if we injected a Printf
        if "log.Printf" in content and '"log"' not in content:
            import_idx = content.find('import (')
            if import_idx != -1:
                content = content[:import_idx+8] + '\n\t"log"' + content[import_idx+8:]

        if content != original_content:
            with open(filename, 'w') as f: 
                f.write(content)
            print(f"[+] Patched robust staging and non-fatal commits in {filename}")

if __name__ == '__main__':
    update_version()
    patch_android_sync()
    print("[+] Application upgraded to Version 1.5.39 successfully!")