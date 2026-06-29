import os
import re

def patch_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # Normalize newlines
    content = content.replace('\r\n', '\n')

    # Use regex to completely encapsulate and replace the commit function
    pattern = re.compile(r'func commitLocalChanges\(repo \*git\.Repository, wTree \*git\.Worktree\) \(bool, error\) \{.*?\n\}', re.DOTALL)
    
    new_func = """func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) (bool, error) {
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
	
	// Explicitly add/remove files to bypass Android AddWithOptions bug
	for name, fileStat := range status {
		if fileStat.Worktree == git.Deleted {
			log.Printf("[sync] Staging deletion: %s", name)
			_, _ = wTree.Remove(name)
			hasRealChanges = true
		} else if fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
			log.Printf("[sync] Staging file: %s", name)
			_, err := wTree.Add(name)
			if err != nil {
				log.Printf("[sync] Warning: failed to add %s: %v", name, err)
			} else {
				hasRealChanges = true
			}
		}
	}

	if !hasRealChanges {
		log.Printf("[sync] No real changes could be staged (FUSE false-dirty).")
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

    if pattern.search(content):
        content = pattern.sub(new_func, content)
        with open(filepath, 'w', encoding='utf-8', newline='\n') as f:
            f.write(content)
        print(f"[+] Successfully patched {filepath} (Applied Explicit Staging)")
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