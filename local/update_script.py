import os
import re
import glob

def update_version():
    """Bumps the application version globally to 1.5.41"""
    for go_file in glob.glob("backend/*.go"):
        if not os.path.isfile(go_file): continue
        with open(go_file, 'r') as f: content = f.read()
        if "1.5.40" in content and ("APP_VERSION" in content or "Version" in content):
            with open(go_file, 'w') as f:
                f.write(content.replace("1.5.40", "1.5.41"))
            print(f"[+] Bumped version in {go_file}")

    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r') as f: content = f.read()
        content = re.sub(r'versionCode 10540', 'versionCode 10541', content)
        content = re.sub(r'versionName "1.5.40"', 'versionName "1.5.41"', content)
        with open(gradle_path, 'w') as f:
            f.write(content)
        print("[+] Bumped version in android/app/build.gradle")

def patch_sync_resilience():
    """Fixes FUSE ghost files, bypasses local commit crashes, and ignores .tmp"""
    for filename in glob.glob("backend/*.go"):
        if not os.path.isfile(filename): continue
        
        with open(filename, 'r') as f: content = f.read()
        original_content = content
        
        # 1. Manual Index pruning to fix FUSE "entry not found"
        pattern_add = r'([ \t]*)([a-zA-Z0-9_]+)\.AddWithOptions\(&git\.AddOptions\{All:\s*true\}\)'
        def replace_add(match):
            indent = match.group(1)
            wtree_var = match.group(2)
            # Avoid double patching if already applied
            if f"wkStatus, _ := {wtree_var}.Status()" in content:
                return match.group(0)
                
            return f'''{indent}// FIX: Android FUSE filesystem bug causes AddWithOptions to miss deleted files.
{indent}wkStatus, _ := {wtree_var}.Status()
{indent}for path, fileStatus := range wkStatus {{
{indent}\tif fileStatus.Worktree == git.Deleted {{
{indent}\t\t{wtree_var}.Remove(path)
{indent}\t}}
{indent}}}
{indent}{wtree_var}.AddWithOptions(&git.AddOptions{{All: true}})'''

        content = re.sub(pattern_add, replace_add, content)

        # 2. Make local Commit non-fatal so it proceeds to Force Pull on error
        content = re.sub(
            r'return\s+fmt\.Errorf\([^)]*Commit failed[^)]*\)',
            r'log.Printf("[LOG] [GO] [sync] Local commit skipped (Ignored, proceeding to pull): %v", err)',
            content
        )

        # 3. Ensure .tmp is ignored so it doesn't trigger "entry not found"
        if 'os.Setenv("TMPDIR", tmpDir)' in content and 'autoGitIgnore(".tmp")' not in content:
            content = content.replace(
                'os.Setenv("TMPDIR", tmpDir)', 
                'os.Setenv("TMPDIR", tmpDir)\n\t\tautoGitIgnore(".tmp") // Lock out temporary git files'
            )

        if content != original_content:
            # Ensure log import exists if we injected a warning
            if "log.Printf" in content and '"log"' not in content:
                import_idx = content.find('import (')
                if import_idx != -1:
                    content = content[:import_idx+8] + '\n\t"log"' + content[import_idx+8:]
            with open(filename, 'w') as f: 
                f.write(content)
            print(f"[+] Patched FUSE staging, non-fatal commits, and .tmp ignore in {filename}")

if __name__ == '__main__':
    update_version()
    patch_sync_resilience()
    print("[+] Application upgraded to Version 1.5.41 successfully!")