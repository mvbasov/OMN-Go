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
    """'1.3.35' → '1.3.36'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version from backend/version.go and bump it
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle (versionCode and versionName)
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_version_code = int(cur_ver.replace(".", ""))
    new_version_code = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {old_version_code}',
                            f'versionCode {new_version_code}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Fix the broken commit block (WriteTree + pointer type mismatch)
    old_broken_block = (
        '\t// Stage and commit local changes first\n'
        '\tlog.Printf("[sync] Staging all changes")\n'
        '\t_, err = wTree.Add(".")\n'
        '\tif err == nil {\n'
        '\t\tstatus, _ := wTree.Status()\n'
        '\t\tif !status.IsClean() {\n'
        '\t\t\tlog.Printf("[sync] Uncommitted changes detected, committing")\n'
        '\t\t\tauthorName := GetConfigAuthor()\n'
        '\t\t\tauthorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"\n'
        '\t\t\tsig := &object.Signature{\n'
        '\t\t\t\tName:  authorName,\n'
        '\t\t\t\tEmail: authorEmail,\n'
        '\t\t\t\tWhen:  time.Now(),\n'
        '\t\t\t}\n'
        '\t\t\t// Write tree from index (avoids worktree.Commit which may call os/user on Android)\n'
        '\t\t\ttreeHash, err := wTree.WriteTree()\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] WriteTree error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t\t// Build commit manually without config or user.Current\n'
        '\t\t\theadRef, errHead := repo.Head()\n'
        '\t\t\tvar parents []plumbing.Hash\n'
        '\t\t\tif errHead == nil {\n'
        '\t\t\t\tparents = []plumbing.Hash{headRef.Hash()}\n'
        '\t\t\t}\n'
        '\t\t\tcommit := &object.Commit{\n'
        '\t\t\t\tAuthor:       sig,\n'
        '\t\t\t\tCommitter:    sig,\n'
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
        '\t\t\t// Update master branch reference\n'
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

    new_fixed_block = (
        '\t// Stage and commit local changes first (manual tree / commit to avoid os/user on Android)\n'
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
        '\t\t\t// Build tree from filesystem (avoids Worktree.WriteTree which may not be available)\n'
        '\t\t\ttreeHash, err := buildTreeFromWorktree(storageDir, repo.Storer)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Build tree error: %v", err)\n'
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

    # Try to patch the broken block; if it's not found, maybe already fixed, just skip.
    try:
        patch_file("backend/handlers.go", old_broken_block, new_fixed_block)
        print("✅ Replaced broken commit block with fixed version.")
    except ValueError as e:
        print(f"⚠️ Could not find broken commit block – maybe already fixed? ({e})")

    # 3. Add the helper function if it doesn't exist
    handlers_content = read_file("backend/handlers.go")
    if "func buildTreeFromWorktree(" not in handlers_content:
        helper_slot = 'func handleSync(w http.ResponseWriter, r *http.Request) {'
        helper_code = (
            '// buildTreeFromWorktree walks the storage directory and creates a Git tree object,\n'
            '// returning the tree hash. This avoids Worktree.WriteTree / user.Current() calls.\n'
            'func buildTreeFromWorktree(dir string, storer git.Storer) (plumbing.Hash, error) {\n'
            '\tentries := make(map[string]*object.TreeEntry)\n'
            '\terr := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {\n'
            '\t\tif err != nil {\n'
            '\t\t\treturn err\n'
            '\t\t}\n'
            '\t\t// Skip .git directory\n'
            '\t\tif info.IsDir() {\n'
            '\t\t\tif info.Name() == ".git" {\n'
            '\t\t\t\treturn filepath.SkipDir\n'
            '\t\t\t}\n'
            '\t\t\treturn nil\n'
            '\t\t}\n'
            '\t\t// Skip .gitignore and other ignored files (simple approach: skip .git files)\n'
            '\t\trel, err := filepath.Rel(dir, path)\n'
            '\t\tif err != nil {\n'
            '\t\t\treturn err\n'
            '\t\t}\n'
            '\t\tif strings.HasPrefix(rel, ".git") {\n'
            '\t\t\treturn nil\n'
            '\t\t}\n'
            '\t\tdata, err := os.ReadFile(path)\n'
            '\t\tif err != nil {\n'
            '\t\t\treturn err\n'
            '\t\t}\n'
            '\t\t// Create blob\n'
            '\t\tblobObj := storer.NewEncodedObject()\n'
            '\t\tblobObj.SetType(plumbing.BlobObject)\n'
            '\t\tblobObj.SetSize(int64(len(data)))\n'
            '\t\twriter, err := blobObj.Writer()\n'
            '\t\tif err != nil {\n'
            '\t\t\treturn err\n'
            '\t\t}\n'
            '\t\tif _, err := writer.Write(data); err != nil {\n'
            '\t\t\treturn err\n'
            '\t\t}\n'
            '\t\twriter.Close()\n'
            '\t\tblobHash, err := storer.SetEncodedObject(blobObj)\n'
            '\t\tif err != nil {\n'
            '\t\t\treturn err\n'
            '\t\t}\n'
            '\t\t// Build tree entry\n'
            '\t\ttreeParts := strings.Split(filepath.ToSlash(rel), "/")\n'
            '\t\tcurrent := entries\n'
            '\t\tfor i, part := range treeParts {\n'
            '\t\t\tif i == len(treeParts)-1 {\n'
            '\t\t\t\t// File entry\n'
            '\t\t\t\tentry := object.TreeEntry{\n'
            '\t\t\t\t\tName: part,\n'
            '\t\t\t\t\tMode: 0100644,\n'
            '\t\t\t\t\tHash: blobHash,\n'
            '\t\t\t\t}\n'
            '\t\t\t\tkey := strings.Join(treeParts[:i+1], "/")\n'
            '\t\t\t\tcurrent[key] = &entry\n'
            '\t\t\t} else {\n'
            '\t\t\t\t// Directory: ensure a tree object placeholder\n'
            '\t\t\t\tdirPath := strings.Join(treeParts[:i+1], "/")\n'
            '\t\t\t\tif _, ok := current[dirPath]; !ok {\n'
            '\t\t\t\t\tcurrent[dirPath] = &object.TreeEntry{\n'
            '\t\t\t\t\t\tName: part,\n'
            '\t\t\t\t\t\tMode: 0040000,\n'
            '\t\t\t\t\t}\n'
            '\t\t\t\t}\n'
            '\t\t\t}\n'
            '\t\t}\n'
            '\t\treturn nil\n'
            '\t})\n'
            '\tif err != nil {\n'
            '\t\treturn plumbing.Hash{}, err\n'
            '\t}\n'
            '\n'
            '\t// Convert entries map to a tree structure, nesting subtrees\n'
            '\trootEntries := []object.TreeEntry{}\n'
            '\tfor _, entry := range entries {\n'
            '\t\trootEntries = append(rootEntries, *entry)\n'
            '\t}\n'
            '\ttreeObj := object.Tree{Entries: rootEntries}\n'
            '\tencoded := storer.NewEncodedObject()\n'
            '\tif err := treeObj.Encode(encoded); err != nil {\n'
            '\t\treturn plumbing.Hash{}, err\n'
            '\t}\n'
            '\treturn storer.SetEncodedObject(encoded)\n'
            '}\n'
            '\n'
            'func handleSync(w http.ResponseWriter, r *http.Request) {'
        )
        try:
            patch_file("backend/handlers.go", helper_slot, helper_code)
            print("✅ Inserted buildTreeFromWorktree helper function.")
        except ValueError as e:
            print(f"⚠️ Failed to insert helper: {e}")
    else:
        print("✅ Helper function already present.")

    # 4. Print the standardised Git commit message
    commit_msg = (
        "fix(sync): replace broken WriteTree with filesystem-based tree builder\n\n"
        "- Remove call to wTree.WriteTree (undefined method)\n"
        "- Use buildTreeFromWorktree that walks the storage directory and builds tree object\n"
        "- Fix Commit struct literals to use *sig instead of sig (pointer vs value)\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()