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

    # 2. Replace the commit block using writeTreeFromDir with wTree.WriteTree
    old_commit = (
        '\t// Stage and commit local changes first (manual tree & commit to avoid os/user on Android)\n'
        '\tlog.Printf("[sync] Staging all changes")\n'
        '\t_, err = wTree.Add(".")\n'
        '\tif err == nil {\n'
        '\t\tstatus, _ := wTree.Status()\n'
        '\t\tif !status.IsClean() {\n'
        '\t\t\tlog.Printf("[sync] Uncommitted changes detected, building commit manually")\n'
        '\t\t\tauthorName := GetConfigAuthor()\n'
        '\t\t\tauthorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"\n'
        '\t\t\tsig := &object.Signature{\n'
        '\t\t\t\tName:  authorName,\n'
        '\t\t\t\tEmail: authorEmail,\n'
        '\t\t\t\tWhen:  time.Now(),\n'
        '\t\t\t}\n'
        '\n'
        '\t\t\t// Build sorted git tree from the working directory (avoids Worktree.WriteTree)\n'
        '\t\t\ttreeHash, err := writeTreeFromDir(storageDir, repo.Storer)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] writeTreeFromDir error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\t// Build commit object manually\n'
        '\t\t\theadRef, errHead := repo.Head()\n'
        '\t\t\tvar parents []plumbing.Hash\n'
        '\t\t\tif errHead == nil {\n'
        '\t\t\t\tparents = []plumbing.Hash{headRef.Hash()}\n'
        '\t\t\t}\n'
        '\t\t\tcommit := &object.Commit{\n'
        '\t\t\t\tAuthor:       *sig,\n'
        '\t\t\t\tCommitter:    *sig,\n'
        '\t\t\t\tMessage:      "Local changes before sync",\n'
        '\t\t\t\tTreeHash:     treeHash,\n'
        '\t\t\t\tParentHashes: parents,\n'
        '\t\t\t}\n'
        '\t\t\tobj := repo.Storer.NewEncodedObject()\n'
        '\t\t\tif err = commit.Encode(obj); err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Commit encode error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\tcommitHash, err := repo.Storer.SetEncodedObject(obj)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Store commit error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\t// Update master branch\n'
        '\t\t\trefName := plumbing.NewBranchReferenceName("master")\n'
        '\t\t\terr = repo.Storer.SetReference(plumbing.NewHashReference(refName, commitHash))\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] SetReference error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\t}\n'
        '\t}'
    )

    new_commit = (
        '\t// Stage and commit local changes first (manual commit to avoid os/user on Android)\n'
        '\tlog.Printf("[sync] Staging all changes")\n'
        '\t_, err = wTree.Add(".")\n'
        '\tif err == nil {\n'
        '\t\tstatus, _ := wTree.Status()\n'
        '\t\tif !status.IsClean() {\n'
        '\t\t\tlog.Printf("[sync] Uncommitted changes detected, writing tree and committing")\n'
        '\t\t\tauthorName := GetConfigAuthor()\n'
        '\t\t\tauthorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"\n'
        '\t\t\tsig := &object.Signature{\n'
        '\t\t\t\tName:  authorName,\n'
        '\t\t\t\tEmail: authorEmail,\n'
        '\t\t\t\tWhen:  time.Now(),\n'
        '\t\t\t}\n'
        '\n'
        '\t\t\t// Write tree from index (works on Android, no user.Current dependency)\n'
        '\t\t\ttreeHash, err := wTree.WriteTree()\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] WriteTree error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\t// Build commit object manually\n'
        '\t\t\theadRef, errHead := repo.Head()\n'
        '\t\t\tvar parents []plumbing.Hash\n'
        '\t\t\tif errHead == nil {\n'
        '\t\t\t\tparents = []plumbing.Hash{headRef.Hash()}\n'
        '\t\t\t}\n'
        '\t\t\tcommit := &object.Commit{\n'
        '\t\t\t\tAuthor:       *sig,\n'
        '\t\t\t\tCommitter:    *sig,\n'
        '\t\t\t\tMessage:      "Local changes before sync",\n'
        '\t\t\t\tTreeHash:     treeHash,\n'
        '\t\t\t\tParentHashes: parents,\n'
        '\t\t\t}\n'
        '\t\t\tobj := repo.Storer.NewEncodedObject()\n'
        '\t\t\tif err = commit.Encode(obj); err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Commit encode error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\tcommitHash, err := repo.Storer.SetEncodedObject(obj)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Store commit error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\t// Update master branch\n'
        '\t\t\trefName := plumbing.NewBranchReferenceName("master")\n'
        '\t\t\terr = repo.Storer.SetReference(plumbing.NewHashReference(refName, commitHash))\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] SetReference error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\t}\n'
        '\t}'
    )

    try:
        patch_file("backend/handlers.go", old_commit, new_commit)
        print("✅ Replaced writeTreeFromDir commit block with WriteTree version.")
    except ValueError:
        print("⚠️ Could not find old commit block; maybe already updated.")

    # 3. Remove the now-unused writeTreeFromDir function
    handlers = read_file("backend/handlers.go")
    if 'func writeTreeFromDir(' in handlers:
        # Remove the function block (including preceding comment line)
        pattern = r'\n// writeTreeFromDir recursively.*\nfunc writeTreeFromDir\(.*?\n\}'
        handlers = re.sub(pattern, '', handlers, flags=re.DOTALL)
        write_file("backend/handlers.go", handlers)
        print("✅ Removed writeTreeFromDir function.")
    else:
        print("ℹ️ writeTreeFromDir function not found; nothing to remove.")

    # 4. Clean up unused imports (sort, storage)
    handlers = read_file("backend/handlers.go")
    # Remove sort if it's only used in the removed function
    if '"sort"' in handlers and 'sort.' not in handlers:
        handlers = handlers.replace('\t"sort"\n', '')
        write_file("backend/handlers.go", handlers)
        print("✅ Removed unused sort import.")
    # Remove storage if unused
    if '"github.com/go-git/go-git/v5/storage"' in handlers and 'storage.' not in handlers:
        handlers = handlers.replace('\t"github.com/go-git/go-git/v5/storage"\n', '')
        write_file("backend/handlers.go", handlers)
        print("✅ Removed unused storage import.")

    # 5. Commit message
    commit_msg = (
        "fix(sync): use wTree.WriteTree instead of custom tree builder\n\n"
        "- WriteTree is available in go-git v5 and avoids OS‑specific ENOSYS errors\n"
        "- Removes writeTreeFromDir which caused 'function not implemented' on Android\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()