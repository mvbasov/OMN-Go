import os
import re

def patch_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # Normalize newlines
    content = content.replace('\r\n', '\n')

    # 1. Inject Imports needed for BLOB manipulation
    imports = [
        '"io"',
        '"github.com/go-git/go-git/v5/plumbing/format/index"',
        '"github.com/go-git/go-git/v5/plumbing/filemode"'
    ]
    for imp in imports:
        if imp not in content:
            content = re.sub(r'import \(', f'import (\\n\\t{imp}', content, count=1)

    # 2. Define the BLOB Injection function and the updated commit handler
    new_code = """func manualStageFile(repo *git.Repository, name string) error {
	fullPath := filepath.Join(storageDir, name)
	stat, err := os.Lstat(fullPath)
	if err != nil {
		return err
	}
	if stat.IsDir() {
		return nil
	}

	f, err := os.Open(fullPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Stream file to object database directly (Bypasses OOM risk)
	obj := repo.Storer.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return err
	}
	if _, err := io.Copy(w, f); err != nil {
		w.Close()
		return err
	}
	w.Close()

	hash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return err
	}

	// Forcibly inject the new BLOB hash into the Git Index
	idx, err := repo.Storer.Index()
	if err != nil {
		return err
	}
	var target *index.Entry
	for _, e := range idx.Entries {
		if e.Name == name {
			target = e
			break
		}
	}
	if target == nil {
		target = &index.Entry{Name: name}
		idx.Entries = append(idx.Entries, target)
	}
	target.Hash = hash
	target.Size = uint32(stat.Size())
	target.ModifiedAt = stat.ModTime()
	target.Mode = filemode.Regular

	return repo.Storer.SetIndex(idx)
}

func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) (bool, error) {
	log.Printf("[sync] Checking worktree status")
	status, err := wTree.Status()
	if err != nil {
		return false, fmt.Errorf("status check error: %v", err)
	}

	if status.IsClean() {
		log.Printf("[sync] Nothing to commit")
		return false, nil
	}

	log.Printf("[sync] Uncommitted changes detected. Manually staging files...")
	hasRealChanges := false
	
	for name, fileStat := range status {
		if fileStat.Worktree == git.Deleted {
			log.Printf("[sync] Staging deletion: %s", name)
			_, _ = wTree.Remove(name)
			hasRealChanges = true
		} else if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			log.Printf("[sync] Staging file: %s", name)
			_, err := wTree.Add(name)
			if err != nil {
				log.Printf("[sync] Warning: go-git Add failed: %v. Using BLOB injection...", err)
				manErr := manualStageFile(repo, name)
				if manErr != nil {
					log.Printf("[sync] Error: BLOB injection failed for %s: %v", name, manErr)
				} else {
					log.Printf("[sync] BLOB injection successful for %s", name)
					hasRealChanges = true
				}
			} else {
				hasRealChanges = true
			}
		}
	}

	if !hasRealChanges {
		log.Printf("[sync] No real changes could be staged.")
		return false, nil
	}

	log.Printf("[sync] Committing staged changes via go-git")
	authorName := GetConfigAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	commitHash, err := wTree.Commit("Local changes before sync", &git.CommitOptions{
		Author:    sig,
		Committer: sig,
	})
	if err == git.ErrEmptyCommit {
		log.Printf("[sync] Commit aborted: git.ErrEmptyCommit")
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}

	log.Printf("[sync] Committed with hash: %s", commitHash.String())
	return true, nil
}"""

    # 3. Clean out old commitLocalChanges (and manualStageFile if re-running)
    if 'func manualStageFile' in content:
        pattern = re.compile(r'func manualStageFile\(.*?func commitLocalChanges\(repo \*git\.Repository, wTree \*git\.Worktree\) \(bool, error\) \{.*?\n\}', re.DOTALL)
    else:
        pattern = re.compile(r'func commitLocalChanges\(repo \*git\.Repository, wTree \*git\.Worktree\) \(bool, error\) \{.*?\n\}', re.DOTALL)

    if pattern.search(content):
        content = pattern.sub(new_code, content)
        with open(filepath, 'w', encoding='utf-8', newline='\n') as f:
            f.write(content)
        print(f"[+] Successfully patched {filepath} (Applied BLOB Injection Fallback)")
    else:
        print(f"[-] Could not find commitLocalChanges in {filepath}.")

if __name__ == "__main__":
    target_files = ["backend/git_helper.go", "git_helper.go"]
    patched = False
    
    for f in target_files:
        if os.path.exists(f):
            patch_file(f)
            patched = True
            break
            
    if not patched:
        print("[-] Could not find git_helper.go in the current or backend/ directory.")