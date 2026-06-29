import os
import re

def patch_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # Normalize newlines to ensure str.replace() matches perfectly
    content = content.replace('\r\n', '\n')

    # 1. Clean up unused imports
    old_imports = r"""	"path/filepath"
	"sort"
	"strings"
	"github.com/go-git/go-git/v5/plumbing/format/gitignore"
	"time\""""
    
    new_imports = r"""	"path/filepath"
	"strings"
	"time\""""
    content = content.replace(old_imports, new_imports)

    # 2. Refactor getOrInitRepo
    old_init = r"""	if err != nil {
		log.Printf("[sync] Repo not found, initializing...")
		if initErr := manualGitInit(storageDir); initErr != nil {
			return nil, fmt.Errorf("manual init failed: %v", initErr)
		}
		repo, err = git.Open(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("failed to open manually created repo: %v", err)
		}
		log.Printf("[sync] Repo initialized")
	} else {"""
    
    new_init = r"""	if err != nil {
		log.Printf("[sync] Repo not found, initializing...")
		repo, err = git.Init(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("git init failed: %v", err)
		}
		log.Printf("[sync] Repo initialized")
	} else {"""
    content = content.replace(old_init, new_init)

    # 3. Refactor commitLocalChanges
    old_commit = r"""	log.Printf("[sync] Uncommitted changes detected, building commit manually")
	authorName := GetConfigAuthor()
	authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
	sig := &object.Signature{
		Name:  authorName,
		Email: authorEmail,
		When:  time.Now(),
	}

	treeHash, err := writeTreeFromDir(storageDir, repo.Storer)
	if err != nil {
		return false, fmt.Errorf("writeTreeFromDir error: %v", err)
	}
	
	headRef, errHead := repo.Head()
	if errHead == nil {
		headCommit, err := repo.CommitObject(headRef.Hash())
		if err == nil && headCommit.TreeHash == treeHash {
			log.Printf("[sync] Tree unchanged from HEAD, nothing to commit")
			return false, nil
		}
	}
	
	var parents []plumbing.Hash
	if errHead == nil {
		parents = []plumbing.Hash{headRef.Hash()}
	}
	commit := &object.Commit{
		Author:       *sig,
		Committer:    *sig,
		Message:      "Local changes before sync",
		TreeHash:     treeHash,
		ParentHashes: parents,
	}
	obj := repo.Storer.NewEncodedObject()
	if err = commit.Encode(obj); err != nil {
		return false, fmt.Errorf("commit encode error: %v", err)
	}
	commitHash, err := repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return false, fmt.Errorf("store commit error: %v", err)
	}
	refPath := filepath.Join(storageDir, ".git", "refs", "heads", "master")
	if err := os.MkdirAll(filepath.Dir(refPath), 0755); err != nil {
		return false, fmt.Errorf("mkdirAll ref error: %v", err)
	}
	if err := os.WriteFile(refPath, []byte(commitHash.String()+"\n"), 0644); err != nil {
		return false, fmt.Errorf("write ref error: %v", err)
	}
	return true, nil
}"""

    new_commit = r"""	log.Printf("[sync] Uncommitted changes detected, committing via go-git")
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
	if err != nil {
		return false, fmt.Errorf("commit error: %v", err)
	}
	log.Printf("[sync] Committed with hash: %s", commitHash.String())

	return true, nil
}"""
    content = content.replace(old_commit, new_commit)

    # 4. Remove the obsolete functions (writeTreeFromDir and manualGitInit) entirely
    # They are sandwiched directly between executeSyncUpload and handleSync.
    pattern = re.compile(r'func writeTreeFromDir.*?func handleSync', re.DOTALL)
    content = pattern.sub('func handleSync', content)

    with open(filepath, 'w', encoding='utf-8', newline='\n') as f:
        f.write(content)
        
    print(f"[+] Successfully patched {filepath}")

if __name__ == "__main__":
    # Handle varying project structures (check backend directory first, then root)
    target_files = ["backend/git_helper.go", "git_helper.go"]
    patched = False
    
    for f in target_files:
        if os.path.exists(f):
            patch_file(f)
            patched = True
            break
            
    if not patched:
        print("[-] Could not find git_helper.go in the current or backend/ directory.")