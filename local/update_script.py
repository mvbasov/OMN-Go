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

    # 2. Replace the reference update block from SetReference to direct filesystem write
    old_ref_update = (
        '\t\t\t// Update master branch\n'
        '\t\t\trefName := plumbing.NewBranchReferenceName("master")\n'
        '\t\t\terr = repo.Storer.SetReference(plumbing.NewHashReference(refName, commitHash))\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] SetReference error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}'
    )
    new_ref_update = (
        '\t\t\t// Update master branch (write ref directly to filesystem to avoid unimplemented syscalls)\n'
        '\t\t\trefPath := filepath.Join(storageDir, ".git", "refs", "heads", "master")\n'
        '\t\t\tif err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] MkdirAll ref error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\tif err := os.WriteFile(refPath, []byte(commitHash.String()+"\\n"), 0644); err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Write ref error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}'
    )
    try:
        patch_file("backend/handlers.go", old_ref_update, new_ref_update)
        print("✅ Replaced SetReference with direct ref file write.")
    except ValueError as e:
        print(f"⚠️ Could not replace SetReference: {e}")

    # 3. (Optional) Remove unused "refName" variable if any leftover (should be gone)
    # No other parts use refName, so we're good.

    # 4. Commit message
    commit_msg = (
        "fix(sync): replace SetReference with direct filesystem write to avoid ENOSYS\n\n"
        "- On Android, SetReference calls an unimplemented function (syscall 38)\n"
        "- Write the reference directly to .git/refs/heads/master using os.WriteFile\n"
        "- This bypasses the problematic call and should allow commits on Android\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()