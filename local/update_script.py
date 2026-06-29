import os
import re
import glob

def update_version():
    """Bumps the application version globally to 1.5.38"""
    # Bump Go Backend version
    for go_file in glob.glob("backend/*.go"):
        if not os.path.isfile(go_file): continue
        with open(go_file, 'r') as f: content = f.read()
        if "1.5.37" in content and ("APP_VERSION" in content or "Version" in content):
            with open(go_file, 'w') as f:
                f.write(content.replace("1.5.37", "1.5.38"))
            print(f"[+] Bumped version in {go_file}")

    # Bump Android Gradle version
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r') as f: content = f.read()
        content = re.sub(r'versionCode \d+', 'versionCode 10538', content)
        content = re.sub(r'versionName "[\d\.]+"', 'versionName "1.5.38"', content)
        with open(gradle_path, 'w') as f:
            f.write(content)
        print("[+] Bumped version in android/app/build.gradle")

def ensure_imports(content, packages):
    """Safely injects missing Go packages"""
    for pkg in packages:
        if f'"{pkg}"' not in content:
            import_idx = content.find('import (')
            if import_idx != -1:
                content = content[:import_idx+8] + f'\n\t"{pkg}"' + content[import_idx+8:]
    return content

def patch_sync_logic():
    """Fixes the untracked file deletion and Android TMPDIR bugs"""
    for filename in glob.glob("backend/*.go"):
        if not os.path.isfile(filename): continue
        
        with open(filename, 'r') as f: content = f.read()
        changed = False
        
        # 1. Replace git.HardReset with Safe Force Checkout (preserves untracked files)
        while "git.HardReset" in content:
            idx = content.find("Reset(&git.ResetOptions")
            if idx == -1: break
            
            # Count braces to capture the exact struct
            start_brace = content.find("{", idx)
            if start_brace == -1: break
            
            brace_count = 1
            end_brace = start_brace
            for i in range(start_brace + 1, len(content)):
                if content[i] == '{': brace_count += 1
                elif content[i] == '}': brace_count -= 1
                if brace_count == 0:
                    end_brace = i
                    break
            
            close_paren = content.find(")", end_brace)
            if close_paren == -1: close_paren = end_brace + 1
            reset_block = content[idx:close_paren+1]
            
            # Extract the commit hash they were trying to reset to
            commit_match = re.search(r'Commit:\s*([^,}\n]+)', reset_block)
            commit_hash = commit_match.group(1).strip() if commit_match else "plumbing.ZeroHash"
            
            # Dynamically detect the repo variable name (e.g. r, repo, etc.)
            wt_decl = re.search(r'([a-zA-Z0-9_]+)\s*,\s*([a-zA-Z0-9_]+)\s*(:=|=)\s*([a-zA-Z0-9_]+)\.Worktree\(\)', content)
            repo_var = wt_decl.group(4) if wt_decl else "r"
            
            replacement = f'''Checkout(&git.CheckoutOptions{{
\t\t\tHash:  {commit_hash},
\t\t\tForce: true,
\t\t}})
\t\t// Fix detached HEAD and update branch ref safely
\t\t{repo_var}.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/main"), {commit_hash}))
\t\t{repo_var}.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName("refs/heads/main")))'''

            content = content[:idx] + replacement + content[close_paren+1:]
            changed = True

        # 2. Inject TMPDIR for Android before Fetch (fixes broken sync on Android)
        if ("Fetch(&git.FetchOptions" in content or "Pull(&git.PullOptions" in content) and "TMPDIR" not in content:
            lines = content.split('\n')
            for i in range(len(lines)):
                if (".Fetch(" in lines[i] or ".Pull(" in lines[i]) and not ("//" in lines[i]):
                    indent = len(lines[i]) - len(lines[i].lstrip())
                    tab = lines[i][:indent]
                    injection = [
                        f"{tab}// Fix Android TMPDIR constraint for go-git packfiles",
                        f"{tab}if runtime.GOOS == \"android\" {{",
                        f"{tab}\ttmpDir := \"/storage/emulated/0/Android/media/net.basov.omngo/.tmp\"",
                        f"{tab}\tos.MkdirAll(tmpDir, 0755)",
                        f"{tab}\tos.Setenv(\"TMPDIR\", tmpDir)",
                        f"{tab}}}"
                    ]
                    lines = lines[:i] + injection + lines[i:]
                    content = '\n'.join(lines)
                    changed = True
                    break

        # Save if modifications occurred
        if changed:
            content = ensure_imports(content, ["runtime", "os", "github.com/go-git/go-git/v5/plumbing"])
            with open(filename, 'w') as f: 
                f.write(content)
            print(f"[+] Patched sync and TMPDIR logic in {filename}")

if __name__ == '__main__':
    update_version()
    patch_sync_logic()
    print("[+] Application upgraded to Version 1.5.38 successfully!")