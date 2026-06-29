#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*.  Raise ValueError if *old* missing."""
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
    # 1. Auto‑detect current version and bump
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    new_code = int(new_ver.replace(".", ""))

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_code = int(cur_ver.replace(".", ""))
    gradle = gradle.replace(f"versionCode {old_code}", f"versionCode {new_code}")
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Patch git_helper.go – prevent empty commits when tree unchanged
    old_func = (
        'func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) error {\n'
        '\tlog.Printf("[sync] Staging all changes")\n'
        '\terr := wTree.AddWithOptions(&git.AddOptions{All: true})\n'
        '\tif err != nil {\n'
        '\t\treturn err\n'
        '\t}\n'
        '\tstatus, _ := wTree.Status()\n'
        '\tif status.IsClean() {\n'
        '\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\treturn nil\n'
        '\t}\n'
        '\t\n'
        '\tlog.Printf("[sync] Uncommitted changes detected, building commit manually")\n'
        '\tauthorName := GetConfigAuthor()\n'
        '\tauthorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"\n'
        '\tsig := &object.Signature{\n'
        '\t\tName:  authorName,\n'
        '\t\tEmail: authorEmail,\n'
        '\t\tWhen:  time.Now(),\n'
        '\t}\n'
        '\n'
        '\ttreeHash, err := writeTreeFromDir(storageDir, repo.Storer)\n'
        '\tif err != nil {\n'
        '\t\treturn fmt.Errorf("writeTreeFromDir error: %v", err)\n'
        '\t}\n'
        '\t\n'
        '\theadRef, errHead := repo.Head()\n'
        '\tvar parents []plumbing.Hash\n'
        '\tif errHead == nil {\n'
        '\t\tparents = []plumbing.Hash{headRef.Hash()}\n'
        '\t}\n'
        '\tcommit := &object.Commit{\n'
        '\t\tAuthor:       *sig,\n'
        '\t\tCommitter:    *sig,\n'
        '\t\tMessage:      "Local changes before sync",\n'
        '\t\tTreeHash:     treeHash,\n'
        '\t\tParentHashes: parents,\n'
        '\t}\n'
        '\tobj := repo.Storer.NewEncodedObject()\n'
        '\tif err = commit.Encode(obj); err != nil {\n'
        '\t\treturn fmt.Errorf("commit encode error: %v", err)\n'
        '\t}\n'
        '\tcommitHash, err := repo.Storer.SetEncodedObject(obj)\n'
        '\tif err != nil {\n'
        '\t\treturn fmt.Errorf("store commit error: %v", err)\n'
        '\t}\n'
        '\trefPath := filepath.Join(storageDir, ".git", "refs", "heads", "master")\n'
        '\tif err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {\n'
        '\t\treturn fmt.Errorf("mkdirAll ref error: %v", err)\n'
        '\t}\n'
        '\tif err := os.WriteFile(refPath, []byte(commitHash.String()+"\\n"), 0644); err != nil {\n'
        '\t\treturn fmt.Errorf("write ref error: %v", err)\n'
        '\t}\n'
        '\treturn nil\n'
        '}'
    )
    new_func = (
        'func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) error {\n'
        '\tlog.Printf("[sync] Staging all changes")\n'
        '\terr := wTree.AddWithOptions(&git.AddOptions{All: true})\n'
        '\tif err != nil {\n'
        '\t\treturn err\n'
        '\t}\n'
        '\tstatus, _ := wTree.Status()\n'
        '\tif status.IsClean() {\n'
        '\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\treturn nil\n'
        '\t}\n'
        '\t\n'
        '\tlog.Printf("[sync] Uncommitted changes detected, building commit manually")\n'
        '\tauthorName := GetConfigAuthor()\n'
        '\tauthorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"\n'
        '\tsig := &object.Signature{\n'
        '\t\tName:  authorName,\n'
        '\t\tEmail: authorEmail,\n'
        '\t\tWhen:  time.Now(),\n'
        '\t}\n'
        '\n'
        '\ttreeHash, err := writeTreeFromDir(storageDir, repo.Storer)\n'
        '\tif err != nil {\n'
        '\t\treturn fmt.Errorf("writeTreeFromDir error: %v", err)\n'
        '\t}\n'
        '\t\n'
        '\theadRef, errHead := repo.Head()\n'
        '\tif errHead == nil {\n'
        '\t\theadCommit, err := repo.CommitObject(headRef.Hash())\n'
        '\t\tif err == nil && headCommit.TreeHash == treeHash {\n'
        '\t\t\tlog.Printf("[sync] Tree unchanged from HEAD, nothing to commit")\n'
        '\t\t\treturn nil\n'
        '\t\t}\n'
        '\t}\n'
        '\t\n'
        '\tvar parents []plumbing.Hash\n'
        '\tif errHead == nil {\n'
        '\t\tparents = []plumbing.Hash{headRef.Hash()}\n'
        '\t}\n'
        '\tcommit := &object.Commit{\n'
        '\t\tAuthor:       *sig,\n'
        '\t\tCommitter:    *sig,\n'
        '\t\tMessage:      "Local changes before sync",\n'
        '\t\tTreeHash:     treeHash,\n'
        '\t\tParentHashes: parents,\n'
        '\t}\n'
        '\tobj := repo.Storer.NewEncodedObject()\n'
        '\tif err = commit.Encode(obj); err != nil {\n'
        '\t\treturn fmt.Errorf("commit encode error: %v", err)\n'
        '\t}\n'
        '\tcommitHash, err := repo.Storer.SetEncodedObject(obj)\n'
        '\tif err != nil {\n'
        '\t\treturn fmt.Errorf("store commit error: %v", err)\n'
        '\t}\n'
        '\trefPath := filepath.Join(storageDir, ".git", "refs", "heads", "master")\n'
        '\tif err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {\n'
        '\t\treturn fmt.Errorf("mkdirAll ref error: %v", err)\n'
        '\t}\n'
        '\tif err := os.WriteFile(refPath, []byte(commitHash.String()+"\\n"), 0644); err != nil {\n'
        '\t\treturn fmt.Errorf("write ref error: %v", err)\n'
        '\t}\n'
        '\treturn nil\n'
        '}'
    )
    patch_file("backend/git_helper.go", old_func, new_func)

    # 3. Print Git commit message
    commit_msg = (
        "fix(git): prevent empty commits when tree is unchanged\n\n"
        "- Add tree‑hash comparison with HEAD before creating a new commit\n"
        "- If the new tree matches the parent's tree, skip the commit entirely\n"
        "- Avoids pushing empty commits during sync when no real disk changes occurred\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()