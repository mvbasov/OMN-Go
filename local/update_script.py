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

    # 2. Remove any leftover buildTreeFromWorktree function and its import
    handlers = read_file("backend/handlers.go")
    if "func buildTreeFromWorktree(" in handlers:
        # Delete the whole function block (including preceding comment)
        pattern = r'\n// buildTreeFromWorktree.*\nfunc buildTreeFromWorktree\(.*?\n\}'
        handlers = re.sub(pattern, '', handlers, flags=re.DOTALL)
        write_file("backend/handlers.go", handlers)
        print("✅ Removed old buildTreeFromWorktree function.")

    # Also remove unused storage import if present
    handlers = read_file("backend/handlers.go")
    if '"github.com/go-git/go-git/v5/storage"' in handlers and 'storage.' not in handlers:
        handlers = handlers.replace('\t"github.com/go-git/go-git/v5/storage"\n', '')
        write_file("backend/handlers.go", handlers)
        print("✅ Removed unused storage import.")

    # 3. Replace the problematic commit block (which may contain WriteTree)
    old_block = (
        '\t// Stage and commit local changes first (manual tree / commit to avoid os/user on Android)\n'
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
        '\t\t\t// Write tree from index (safe, doesn\'t need os/user)\n'
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

    new_block = (
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

    if old_block in read_file("backend/handlers.go"):
        patch_file("backend/handlers.go", old_block, new_block)
        print("✅ Replaced commit block with writeTreeFromDir version.")
    else:
        print("⚠️ Exact commit block not found; attempting to fix manually.")
        # Fallback: if the old block contains WriteTree but differs slightly, we can still try to replace
        handlers = read_file("backend/handlers.go")
        if 'wTree.WriteTree()' in handlers:
            # replace the whole section with the new one using a broader match
            # we'll locate the start and end patterns
            start = '\t// Stage and commit local changes first'
            end = '\t}\n\n\tif action == "download"'
            idx_start = handlers.find(start)
            idx_end = handlers.find(end)
            if idx_start != -1 and idx_end != -1:
                # extract the block
                block = handlers[idx_start:idx_end]
                # ensure it contains WriteTree
                if 'wTree.WriteTree()' in block:
                    # replace block with new block (trim leading tabs appropriately)
                    # The new block already has matching indentation
                    handlers = handlers[:idx_start] + new_block + "\n\n" + handlers[idx_end:]
                    write_file("backend/handlers.go", handlers)
                    print("✅ Fallback replacement succeeded.")
                else:
                    print("❌ Block doesn't contain WriteTree, skipping.")
            else:
                print("❌ Could not locate commit block boundaries.")
        else:
            print("ℹ️ No WriteTree call found, commit block may be correct already.")

    # 4. Add the new writeTreeFromDir helper function before handleSync
    helper_code = (
        '// writeTreeFromDir recursively creates a sorted git tree object from the given directory.\n'
        '// It skips .git directory and returns the tree hash.\n'
        'func writeTreeFromDir(dir string, storer storage.Storer) (plumbing.Hash, error) {\n'
        '\tentries := []object.TreeEntry{}\n'
        '\tfiles, err := os.ReadDir(dir)\n'
        '\tif err != nil {\n'
        '\t\treturn plumbing.Hash{}, err\n'
        '\t}\n'
        '\tfor _, f := range files {\n'
        '\t\tif f.Name() == ".git" || f.Name() == ".gitignore" {\n'
        '\t\t\tcontinue\n'
        '\t\t}\n'
        '\t\tfullPath := filepath.Join(dir, f.Name())\n'
        '\t\tif f.IsDir() {\n'
        '\t\t\tsubTreeHash, err := writeTreeFromDir(fullPath, storer)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tentries = append(entries, object.TreeEntry{\n'
        '\t\t\t\tName: f.Name(),\n'
        '\t\t\t\tMode: 0040000, // directory\n'
        '\t\t\t\tHash: subTreeHash,\n'
        '\t\t\t})\n'
        '\t\t} else {\n'
        '\t\t\tdata, err := os.ReadFile(fullPath)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tblobObj := storer.NewEncodedObject()\n'
        '\t\t\tblobObj.SetType(plumbing.BlobObject)\n'
        '\t\t\tblobObj.SetSize(int64(len(data)))\n'
        '\t\t\tw, err := blobObj.Writer()\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tif _, err = w.Write(data); err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tw.Close()\n'
        '\t\t\tblobHash, err := storer.SetEncodedObject(blobObj)\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\treturn plumbing.Hash{}, err\n'
        '\t\t\t}\n'
        '\t\t\tentries = append(entries, object.TreeEntry{\n'
        '\t\t\t\tName: f.Name(),\n'
        '\t\t\t\tMode: 0100644, // regular file\n'
        '\t\t\t\tHash: blobHash,\n'
        '\t\t\t})\n'
        '\t\t}\n'
        '\t}\n'
        '\t// Sort entries by name (required by git)\n'
        '\tsort.Slice(entries, func(i, j int) bool {\n'
        '\t\treturn entries[i].Name < entries[j].Name\n'
        '\t})\n'
        '\t// Create tree object\n'
        '\ttreeObj := object.Tree{Entries: entries}\n'
        '\tencoded := storer.NewEncodedObject()\n'
        '\tif err := treeObj.Encode(encoded); err != nil {\n'
        '\t\treturn plumbing.Hash{}, err\n'
        '\t}\n'
        '\treturn storer.SetEncodedObject(encoded)\n'
        '}\n'
        '\n'
        'func handleSync(w http.ResponseWriter, r *http.Request) {'
    )

    slot = 'func handleSync(w http.ResponseWriter, r *http.Request) {'
    if helper_code not in read_file("backend/handlers.go"):
        if slot in read_file("backend/handlers.go"):
            patch_file("backend/handlers.go", slot, helper_code)
            print("✅ Inserted writeTreeFromDir function.")
        else:
            print("❌ Could not find handleSync slot to insert helper.")
    else:
        print("ℹ️ Helper function already present.")

    # 5. Ensure required imports: storage and sort
    imports_needed = []
    handlers = read_file("backend/handlers.go")
    if '"sort"' not in handlers:
        imports_needed.append('sort')
    if '"github.com/go-git/go-git/v5/storage"' not in handlers:
        imports_needed.append('"github.com/go-git/go-git/v5/storage"')
    if imports_needed:
        # Insert after the existing "github.com/go-git/go-git/v5/plumbing/transport/ssh" import
        target = '\t"github.com/go-git/go-git/v5/plumbing/transport/ssh"'
        if target in handlers:
            new_imports = target
            for imp in imports_needed:
                if imp == 'sort':
                    new_imports += '\n\t"sort"'
                else:
                    new_imports += f'\n\t{imp}'
            handlers = handlers.replace(target, new_imports)
            write_file("backend/handlers.go", handlers)
            print(f"✅ Added imports: {', '.join(imports_needed)}")
        else:
            print("⚠️ Could not find import anchor to add required imports; manual check needed.")

    # 6. Commit message
    commit_msg = (
        "fix(sync): implement recursive sorted tree builder for commit\n\n"
        "- Replace flawed flat tree builder with writeTreeFromDir that recursively builds subtrees\n"
        "- Sort tree entries to satisfy git requirements (\"entries in tree are not sorted\")\n"
        "- Avoids worktree.Commit and WriteTree, works on Android\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()