import os
import re
import glob

def update_version():
    """Bumps the application version globally to 1.5.40"""
    for go_file in glob.glob("backend/*.go"):
        if not os.path.isfile(go_file): continue
        with open(go_file, 'r') as f: content = f.read()
        if "1.5.39" in content and ("APP_VERSION" in content or "Version" in content):
            with open(go_file, 'w') as f:
                f.write(content.replace("1.5.39", "1.5.40"))
            print(f"[+] Bumped version in {go_file}")

    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r') as f: content = f.read()
        content = re.sub(r'versionCode 10539', 'versionCode 10540', content)
        content = re.sub(r'versionName "1.5.39"', 'versionName "1.5.40"', content)
        with open(gradle_path, 'w') as f:
            f.write(content)
        print("[+] Bumped version in android/app/build.gradle")

def patch_auto_gitignore():
    """Injects dynamic .gitignore logic exclusively for embedFS caching"""
    for filename in glob.glob("backend/*.go"):
        if not os.path.isfile(filename): continue
        
        with open(filename, 'r') as f: content = f.read()
        original_content = content
        
        # 1. Inject the autoGitIgnore helper function
        if ("serveLazy" in content or "Lazy" in content) and "func autoGitIgnore" not in content:
            content += '''\n
// autoGitIgnore safely appends extracted cache files to .gitignore
func autoGitIgnore(cachePath string) {
\tignoreFile := ".gitignore"
\tcontent, err := os.ReadFile(ignoreFile)
\tif err != nil && !os.IsNotExist(err) {
\t\treturn // Skip if we can't read an existing file due to permissions
\t}
\t
\t// Git requires forward slashes
\tignoreStr := filepath.ToSlash(cachePath)
\t
\t// Only append if the file path is not already in .gitignore
\tif !strings.Contains(string(content), ignoreStr) {
\t\tf, err := os.OpenFile(ignoreFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
\t\tif err == nil {
\t\t\tf.WriteString("\\n" + ignoreStr)
\t\t\tf.Close()
\t\t}
\t}
}'''
            
        # 2. Tightly scope the hook so it ONLY runs inside the lazy extraction logic
        # This guarantees standard user notes are never accidentally ignored!
        if "func autoGitIgnore" in content and "autoGitIgnore(" not in original_content:
            in_lazy_embed = False
            lines = content.split('\n')
            
            for i in range(len(lines)):
                # Detect entry into the extraction function
                if "func serveLazy" in lines[i] or ("func " in lines[i] and "Lazy" in lines[i]):
                    in_lazy_embed = True
                # Detect exit to another function
                elif in_lazy_embed and lines[i].startswith("func "): 
                    in_lazy_embed = False
                    
                # Hook into standard os.WriteFile caching
                if in_lazy_embed and "os.WriteFile(" in lines[i]:
                    match = re.search(r'os\.WriteFile\(\s*([^,]+)\s*,', lines[i])
                    if match:
                        path_var = match.group(1).strip()
                        indent = lines[i][:len(lines[i]) - len(lines[i].lstrip())]
                        lines.insert(i+1, f"{indent}autoGitIgnore({path_var}) // Dynamically ignore extracted asset")
                        break
                # Hook into standard os.Create caching (io.Copy method)
                elif in_lazy_embed and "os.Create(" in lines[i]:
                    match = re.search(r'os\.Create\(\s*([^)]+)\s*\)', lines[i])
                    if match:
                        path_var = match.group(1).strip()
                        indent = lines[i][:len(lines[i]) - len(lines[i].lstrip())]
                        lines.insert(i+1, f"{indent}autoGitIgnore({path_var}) // Dynamically ignore extracted asset")
                        break

            content = '\n'.join(lines)

        # 3. Ensure required packages are imported
        if "autoGitIgnore" in content:
            for pkg in ["os", "strings", "path/filepath"]:
                if f'"{pkg}"' not in content:
                    import_idx = content.find('import (')
                    if import_idx != -1:
                        content = content[:import_idx+8] + f'\n\t"{pkg}"' + content[import_idx+8:]

        if content != original_content:
            with open(filename, 'w') as f: 
                f.write(content)
            print(f"[+] Patched dynamic gitignore logic in {filename}")

if __name__ == '__main__':
    update_version()
    patch_auto_gitignore()
    print("[+] Application upgraded to Version 1.5.40 successfully!")