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

    # 2. Replace the entire commit block with the known-good wTree.Commit call
    # We identify the block from the comment "// Stage and commit local changes first" to
    # the closing brace before "if action == "download" {".
    # This replaces any previous custom tree builder logic.

    target_old = (
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

    target_new = (
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
        '\t\t\t_, err = wTree.Commit("Local changes before sync", &git.CommitOptions{\n'
        '\t\t\t\tAuthor:    sig,\n'
        '\t\t\t\tCommitter: sig, // CRITICAL: both set to avoid os/user.Current() on Android\n'
        '\t\t\t})\n'
        '\t\t\tif err != nil {\n'
        '\t\t\t\tlog.Printf("[sync] Commit error: %v", err)\n'
        '\t\t\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)\n'
        '\t\t\t\treturn\n'
        '\t\t\t}\n'
        '\t\t} else {\n'
        '\t\t\tlog.Printf("[sync] Nothing to commit")\n'
        '\t\t}\n'
        '\t}'
    )

    # Apply patch only if the old block exists; if not, maybe the target_new is already in place
    content = read_file("backend/handlers.go")
    if target_old in content:
        patch_file("backend/handlers.go", target_old, target_new)
        print("✅ Replaced custom tree builder commit block with wTree.Commit.")
    else:
        print("ℹ️ Custom tree builder block not found. Checking if wTree.Commit already present...")
        if 'wTree.Commit(' in content:
            print("ℹ️ Commit block already uses wTree.Commit. No change needed.")
        else:
            # Fallback: use a broader replacement based on comment and action
            start_marker = '\t// Stage and commit local changes first'
            end_marker = '\tif action == "download" {'
            start_idx = content.find(start_marker)
            end_idx = content.find(end_marker)
            if start_idx != -1 and end_idx != -1:
                # Replace everything from start_marker to just before end_marker
                original_block = content[start_idx:end_idx]
                if original_block.strip() != target_new.strip():
                    new_content = content[:start_idx] + target_new + "\n\n" + content[end_idx:]
                    write_file("backend/handlers.go", new_content)
                    print("✅ Fallback replacement applied using markers.")
                else:
                    print("ℹ️ Block already matches target.")
            else:
                print("❌ Could not locate commit block boundaries. No patch applied.")

    # 3. Remove the unused writeTreeFromDir function (if still present)
    handlers = read_file("backend/handlers.go")
    if 'func writeTreeFromDir(' in handlers:
        pattern = r'\n// writeTreeFromDir recursively creates.*\nfunc writeTreeFromDir\(.*?\n\}'
        handlers = re.sub(pattern, '', handlers, flags=re.DOTALL)
        write_file("backend/handlers.go", handlers)
        print("✅ Removed writeTreeFromDir function.")

    # 4. Clean up unused imports (sort, storage)
    handlers = read_file("backend/handlers.go")
    if '"sort"' in handlers and 'sort.' not in handlers:
        handlers = handlers.replace('\t"sort"\n', '')
        write_file("backend/handlers.go", handlers)
        print("✅ Removed unused sort import.")
    if '"github.com/go-git/go-git/v5/storage"' in handlers and 'storage.' not in handlers:
        handlers = handlers.replace('\t"github.com/go-git/go-git/v5/storage"\n', '')
        write_file("backend/handlers.go", handlers)
        print("✅ Removed unused storage import.")

    # 5. Print commit message
    commit_msg = (
        "fix(sync): revert to wTree.Commit with explicit Author/Committer\n\n"
        "- Drop custom tree builder in favor of stable go-git Commit method\n"
        "- Both Author and Committer set to avoid os/user.Current() on Android\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()