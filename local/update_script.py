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

def apply_fix(path, old, new, idem_marker):
    content = read_file(path)
    if idem_marker in content:
        return
    if old not in content:
        raise ValueError(f"❌ Old string not found in {path}")
    content = content.replace(old, new, 1)
    write_file(path, content)

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

    h_path = "backend/handlers.go"
    h = read_file(h_path)

    # 2. Insert manualGitInit helper function right before func handleSync
    helper_func = r'''func manualGitInit(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/master\n"), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		return err
	}
	config := []byte("[core]\n\trepositoryformatversion = 0\n\tfilemode = true\n\tbare = false\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), config, 0644); err != nil {
		return err
	}
	return nil
}

'''
    # Check if function already present
    if "func manualGitInit(dir string) error" not in h:
        # Insert before "func handleSync"
        idx = h.find("\nfunc handleSync(")
        if idx == -1:
            raise ValueError("Could not locate handleSync function")
        h = h[:idx] + "\n" + helper_func + h[idx:]
        write_file(h_path, h)
        h = read_file(h_path)  # refresh

    # 3. Modify repo initialization to use manual fallback on PlainInit failure
    old_block = (
        '\tif err != nil {\n'
        '\t\tlog.Printf("[sync] Repo not found, initializing...")\n'
        '\t\trepo, err = git.PlainInit(storageDir, false)\n'
        '\t\tif err != nil {\n'
        '\t\t\tlog.Printf("[sync] Repo init failed: %v", err)\n'
        '\t\t\thttp.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)\n'
        '\t\t\treturn\n'
        '\t\t}\n'
    )
    new_block = (
        '\tif err != nil {\n'
        '\t\tlog.Printf("[sync] Repo not found, initializing...")\n'
        '\t\trepo, err = git.PlainInit(storageDir, false)\n'
        '\t\tif err != nil {\n'
        '\t\t\tlog.Printf("[sync] git.PlainInit failed: %v; attempting manual init", err)\n'
        '\t\t\tif initErr := manualGitInit(storageDir); initErr != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Manual init also failed: %v", initErr)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Repo init failed: %v", initErr), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\t// Try opening again\n'
        '\t\t\trepo, err = git.PlainOpen(storageDir)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Failed to open manually created repo: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t}\n'
    )
    apply_fix(h_path, old_block, new_block, "manualGitInit(storageDir)")

    commit_msg = (
        "fix(sync): add manual Git repo init fallback for Android/restricted environments\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()