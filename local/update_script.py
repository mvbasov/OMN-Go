import os

def patch_file(filepath):
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # Normalize newlines
    content = content.replace('\r\n', '\n')

    # 1. Restore the manual initialization logic in getOrInitRepo
    bad_init = r"""	if err != nil {
		log.Printf("[sync] Repo not found, initializing...")
		repo, err = git.Init(storer, wtFS)
		if err != nil {
			return nil, fmt.Errorf("git init failed: %v", err)
		}
		log.Printf("[sync] Repo initialized")
	} else {"""
    
    good_init = r"""	if err != nil {
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
    
    content = content.replace(bad_init, good_init)

    # 2. Append the manualGitInit function back to the bottom of the file
    if "func manualGitInit" not in content:
        manual_init_code = """
func manualGitInit(dir string) error {
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/master\\n"), 0644); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(gitDir, "objects"), 0755); err != nil {
		return err
	}
	config := []byte("[core]\\n\\trepositoryformatversion = 0\\n\\tfilemode = true\\n\\tbare = false\\n")
	if err := os.WriteFile(filepath.Join(gitDir, "config"), config, 0644); err != nil {
		return err
	}
	return nil
}
"""
        # Append before the final closing brace if it exists in a weird way, otherwise just append.
        content += manual_init_code

    with open(filepath, 'w', encoding='utf-8', newline='\n') as f:
        f.write(content)
        
    print(f"[+] Successfully patched {filepath} (Restored manualGitInit)")

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