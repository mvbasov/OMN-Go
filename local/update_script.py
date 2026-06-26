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

    # 2. Replace the commit block that uses wTree.Commit with a manual commit using writeTreeFromDir
    old_commit_block = (
        '\t// Stage and commit local changes first (manual commit to avoid os/user on Android)\n'
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
        '\t\t\tlog.Printf("[sync] Attempting commit with author=%q, email=%q", sig.Name, sig.Email)\n'
        '\t\t\t_, err = wTree.Commit("Local changes before sync", &git.CommitOptions{\n'
        '\t\t\t\tAuthor:    sig,\n'
        '\t\t\t\tCommitter: sig, // CRITICAL: both set to avoid os/user.Current() on Android\n'
        '\t\t\t})\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Commit error: %v (type: %T)", err, err)\n'
        '\t\t\t\tlog.Printf("[sync] Commit options – Author: %v, Committer: %v", sig, sig)\n'
        '\t\t\t\tlog.Printf("[sync] Worktree status – IsClean: %v", status.IsClean())\n'
        '\t\t\t\t// Dump the underlying system error if it\'s a syscall error\n'
        '\t\t\t\tif errno, ok := err.(syscall.Errno); ok {\n'
        '\t\t\t\t\tlog.Printf("[sync] System call error number: %d", errno)\n'
        '\t\t\t\t}\n'
        '\t\t\t\tlog.Printf("[sync] Full error: %#v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\t}\n'
        '\t}'
    )

    new_commit_block = (
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
        '\t\t\t// Build sorted git tree from the working directory\n'
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

    try:
        patch_file("backend/handlers.go", old_commit_block, new_commit_block)
        print("✅ Replaced wTree.Commit block with manual tree/commit.")
    except ValueError as e:
        print(f"⚠️ Could not replace commit block: {e}")

    # 3. Ensure the writeTreeFromDir function exists and is correct (sorted, recursive).
    # Remove any previous broken version, then insert the fixed one.
    handlers = read_file("backend/handlers.go")
    # Remove any existing writeTreeFromDir function (including comment)
    if 'func writeTreeFromDir(' in handlers:
        pattern = r'\n// writeTreeFromDir recursively.*\nfunc writeTreeFromDir\(.*?\n\}'
        handlers = re.sub(pattern, '', handlers, flags=re.DOTALL)
        # Also remove the "storage.Storer" import if no longer used, but we'll keep it for now.
        write_file("backend/handlers.go", handlers)
        print("✅ Removed old writeTreeFromDir function.")
    # Insert correct writeTreeFromDir before handleSync
    slot = 'func handleSync(w http.ResponseWriter, r *http.Request) {'
    if slot in handlers:
        helper_code = (
            '// writeTreeFromDir recursively creates a sorted git tree object from the given directory.\n'
            '// It skips .git and .gitignore, and ensures entries are sorted by name.\n'
            'func writeTreeFromDir(dir string, storer storage.Storer) (plumbing.Hash, error) {\n'
            '\tfiles, err := os.ReadDir(dir)\n'
            '\tif err != nil {\n'
            '\t\treturn plumbing.Hash{}, err\n'
            '\t}\n'
            '\t// Sort directory entries for deterministic order\n'
            '\tsort.Slice(files, func(i, j int) bool { return files[i].Name() < files[j].Name() })\n'
            '\tentries := []object.TreeEntry{}\n'
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
            '\t\t\t\tMode: 0040000,\n'
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
            '\t\t\t\tMode: 0100644,\n'
            '\t\t\t\tHash: blobHash,\n'
            '\t\t\t})\n'
            '\t\t}\n'
            '\t}\n'
            '\t// Build tree object\n'
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
        patch_file("backend/handlers.go", slot, helper_code)
        print("✅ Inserted fixed writeTreeFromDir function.")
    else:
        print("❌ Could not find handleSync slot to insert helper.")

    # 4. Ensure required imports: sort and storage are present
    handlers = read_file("backend/handlers.go")
    imports_to_add = []
    if '"sort"' not in handlers:
        imports_to_add.append('\t"sort"\n')
    if '"github.com/go-git/go-git/v5/storage"' not in handlers:
        imports_to_add.append('\t"github.com/go-git/go-git/v5/storage"\n')
    if imports_to_add:
        # Insert after "strings" line
        old_import = '\t"strings"\n'
        new_import = '\t"strings"\n' + ''.join(imports_to_add)
        if old_import in handlers:
            handlers = handlers.replace(old_import, new_import, 1)
            write_file("backend/handlers.go", handlers)
            print(f"✅ Added imports: {', '.join(imports_to_add)}")
        else:
            print("⚠️ Could not find 'strings' import line to add imports.")

    # 5. Remove unused syscall import if it was added earlier but now no longer needed
    if '"syscall"' in handlers and 'syscall.' not in handlers:
        handlers = handlers.replace('\t"syscall"\n', '')
        write_file("backend/handlers.go", handlers)
        print("✅ Removed unused syscall import.")

    # 6. Commit message
    commit_msg = (
        "fix(sync): use manual tree/commit to avoid ENOSYS on Android\n\n"
        "- Replace wTree.Commit (calls unimplemented function) with custom writeTreeFromDir\n"
        "- Ensures sorted tree entries to satisfy git requirements\n"
        "- Avoids any syscall that triggers \"function not implemented\"\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()