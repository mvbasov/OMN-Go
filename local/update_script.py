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

def safe_patch(path, old, new):
    content = read_file(path)
    if old in content:
        content = content.replace(old, new, 1)
        write_file(path, content)
    elif new not in content:
        raise ValueError(f"❌ Neither old nor new string found in {path}")

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
    cur_vc = int(cur_ver.replace(".", ""))
    new_vc = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {cur_vc}', f'versionCode {new_vc}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Enhance .gitignore creation to include sync key file
    # Find the block that ensures .gitignore and replace it with a new one that also appends the key path
    old_gitignore_block = (
        '\t// Ensure .gitignore\n'
        '\tgitignorePath := filepath.Join(storageDir, ".gitignore")\n'
        '\tif _, err := os.Stat(gitignorePath); os.IsNotExist(err) {\n'
        '\t\tos.WriteFile(gitignorePath, []byte("# OMN-Go sync ignore\\nconfig.json\\n*.html\\n"), 0644)\n'
        '\t}'
    )
    new_gitignore_block = (
        '\t// Ensure .gitignore\n'
        '\tgitignorePath := filepath.Join(storageDir, ".gitignore")\n'
        '\tgitignoreBase := "# OMN-Go sync ignore\\nconfig.json\\n*.html\\n"\n'
        '\tif _, err := os.Stat(gitignorePath); os.IsNotExist(err) {\n'
        '\t\tos.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)\n'
        '\t}\n'
        '\t// Append SSH key file to .gitignore if inside storageDir\n'
        '\tif appConfig.SyncSSHKey != "" {\n'
        '\t\tkeyPath := appConfig.SyncSSHKey\n'
        '\t\tif !filepath.IsAbs(keyPath) {\n'
        '\t\t\tkeyPath = filepath.Join(storageDir, keyPath)\n'
        '\t\t}\n'
        '\t\trelKey, err := filepath.Rel(storageDir, keyPath)\n'
        '\t\tif err == nil && !strings.HasPrefix(relKey, "..") {\n'
        '\t\t\tcurrent, _ := os.ReadFile(gitignorePath)\n'
        '\t\t\tif !strings.Contains(string(current), relKey) {\n'
        '\t\t\t\tnewContent := string(current) + "\\n" + relKey + "\\n"\n'
        '\t\t\t\tos.WriteFile(gitignorePath, []byte(newContent), 0644)\n'
        '\t\t\t}\n'
        '\t\t}\n'
        '\t}'
    )
    safe_patch("backend/handlers.go", old_gitignore_block, new_gitignore_block)

    commit_msg = (
        "fix(sync): automatically add sync_ssh_key to .gitignore\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()