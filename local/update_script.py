#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def replace_function(file_path, func_name, new_func_body):
    """Replace the entire top-level function *func_name* with *new_func_body*."""
    content = read_file(file_path)
    # Find the header of the target function
    header_pattern = r'^func ' + re.escape(func_name) + r'\(.*?\)\s*\{'
    start_match = re.search(header_pattern, content, flags=re.MULTILINE)
    if not start_match:
        raise ValueError(f"❌ Function {func_name} not found in {file_path}")

    start_pos = start_match.start()
    # Find the next top-level function after the target function's start
    # Look for "func " at start of a line, not indented
    next_match = re.search(r'^func ', content[start_match.end():], flags=re.MULTILINE)
    if next_match:
        end_pos = start_match.end() + next_match.start()
    else:
        # No more functions, go to end of file
        end_pos = len(content)

    # Replace the old function block with the new one
    new_content = content[:start_pos] + new_func_body.strip() + "\n" + content[end_pos:]
    write_file(file_path, new_content)

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
    cur_vc = int(cur_ver.replace(".", ""))
    new_vc = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {cur_vc}', f'versionCode {new_vc}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Replace handleSync with the heavily logged version (if not already done)
    current = read_file("backend/handlers.go")
    if 'log.Printf("[sync] Request received")' in current:
        print("handleSync already contains debug logging, skipping replacement.")
    else:
        new_handleSync = r'''
func handleSync(w http.ResponseWriter, r *http.Request) {
	log.Printf("[sync] Request received")
	if r.Method != "POST" {
		http.Error(w, "Method Not Allowed", 405)
		return
	}

	if appConfig.SyncRemote == "" {
		log.Printf("[sync] Error: sync_remote not configured")
		http.Error(w, "Sync not configured: missing sync_remote in config.json", 500)
		return
	}

	log.Printf("[sync] Remote: %s", appConfig.SyncRemote)

	// Ensure .gitignore
	gitignorePath := filepath.Join(storageDir, ".gitignore")
	gitignoreBase := "# OMN-Go sync ignore\nconfig.json\n*.html\n"
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		os.WriteFile(gitignorePath, []byte(gitignoreBase), 0644)
		log.Printf("[sync] Created .gitignore")
	}
	// Append SSH key file to .gitignore if inside storageDir
	if appConfig.SyncSSHKey != "" {
		keyPath := appConfig.SyncSSHKey
		if !filepath.IsAbs(keyPath) {
			keyPath = filepath.Join(storageDir, keyPath)
		}
		relKey, err := filepath.Rel(storageDir, keyPath)
		if err == nil && !strings.HasPrefix(relKey, "..") {
			current, _ := os.ReadFile(gitignorePath)
			if !strings.Contains(string(current), relKey) {
				newContent := string(current) + "\n" + relKey + "\n"
				os.WriteFile(gitignorePath, []byte(newContent), 0644)
				log.Printf("[sync] Added %s to .gitignore", relKey)
			}
		}
	}

	// Open or init repo
	log.Printf("[sync] Opening repo at %s", storageDir)
	repo, err := git.PlainOpen(storageDir)
	if err != nil {
		log.Printf("[sync] Repo not found, initializing...")
		repo, err = git.PlainInit(storageDir, false)
		if err != nil {
			log.Printf("[sync] Repo init failed: %v", err)
			http.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)
			return
		}
		log.Printf("[sync] Repo initialized")
		// Check if remote origin exists
		_, err = repo.Remote("origin")
		if err != nil {
			log.Printf("[sync] Remote origin not found, adding")
			_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
				Name: "origin",
				URLs: []string{appConfig.SyncRemote},
			})
			if err != nil {
				log.Printf("[sync] Remote add failed: %v", err)
				http.Error(w, fmt.Sprintf("Remote add failed: %v", err), 500)
				return
			}
		}
	} else {
		log.Printf("[sync] Repo opened successfully")
		// Repo exists, ensure remote
		_, err = repo.Remote("origin")
		if err != nil {
			log.Printf("[sync] Remote origin missing, adding")
			_, err = repo.CreateRemote(&gitconfig.RemoteConfig{
				Name: "origin",
				URLs: []string{appConfig.SyncRemote},
			})
			if err != nil {
				log.Printf("[sync] Remote add failed: %v", err)
				http.Error(w, fmt.Sprintf("Remote add failed: %v", err), 500)
				return
			}
		}
	}

	// Prepare SSH auth
	var auth transport.AuthMethod
	if appConfig.SyncSSHKey != "" {
		keyPath := appConfig.SyncSSHKey
		if !filepath.IsAbs(keyPath) {
			keyPath = filepath.Join(storageDir, keyPath)
		}
		log.Printf("[sync] Using SSH key: %s", keyPath)

		// Check file existence and permissions
		info, err := os.Stat(keyPath)
		if err != nil {
			log.Printf("[sync] SSH key file not accessible: %v", err)
			http.Error(w, fmt.Sprintf("Failed to read SSH key: %v", err), 500)
			return
		}
		log.Printf("[sync] Key file size: %d, mode: %s", info.Size(), info.Mode())

		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			log.Printf("[sync] Read key file error: %v", err)
			http.Error(w, fmt.Sprintf("Failed to read SSH key: %v", err), 500)
			return
		}
		log.Printf("[sync] Read %d bytes from key file", len(keyBytes))

		passphrase := appConfig.SyncSSHPassphrase
		if passphrase != "" {
			log.Printf("[sync] Passphrase provided (length %d)", len(passphrase))
			auth, err = ssh.NewPublicKeys("git", keyBytes, passphrase)
		} else {
			log.Printf("[sync] No passphrase")
			auth, err = ssh.NewPublicKeys("git", keyBytes, "")
		}
		if err != nil {
			log.Printf("[sync] ssh.NewPublicKeys error: %v", err)
			http.Error(w, fmt.Sprintf("SSH auth failed: %v", err), 500)
			return
		}
		log.Printf("[sync] SSH auth method created successfully")
	} else {
		log.Printf("[sync] Error: No SSH key configured")
		http.Error(w, "No SSH key configured", 500)
		return
	}

	wTree, _ := repo.Worktree()

	// Stage and commit local changes first
	log.Printf("[sync] Staging all changes")
	_, err = wTree.Add(".")
	if err != nil {
		log.Printf("[sync] Add error: %v", err)
		http.Error(w, fmt.Sprintf("Add failed: %v", err), 500)
		return
	}
	status, err := wTree.Status()
	if err != nil {
		log.Printf("[sync] Status error: %v", err)
		http.Error(w, fmt.Sprintf("Status failed: %v", err), 500)
		return
	}
	if !status.IsClean() {
		log.Printf("[sync] Uncommitted changes detected, committing")
		_, err = wTree.Commit("Local changes before sync", &git.CommitOptions{
			Author: &object.Signature{
				Name:  "OMN-Go Sync",
				Email: "omngo@localhost",
				When:  time.Now(),
			},
		})
		if err != nil {
			log.Printf("[sync] Commit error: %v", err)
			http.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)
			return
		}
	} else {
		log.Printf("[sync] Nothing to commit")
	}

	// Pull from origin
	log.Printf("[sync] Pulling from origin master")
	err = wTree.Pull(&git.PullOptions{
		RemoteName:    "origin",
		Auth:          auth,
		ReferenceName: plumbing.NewBranchReferenceName("master"),
		SingleBranch:  true,
	})
	if err != nil {
		if err == git.NoErrAlreadyUpToDate || strings.Contains(err.Error(), "couldn't find remote ref") {
			log.Printf("[sync] Pull not needed (no remote ref or up to date): %v", err)
		} else {
			log.Printf("[sync] Pull error: %v", err)
			http.Error(w, fmt.Sprintf("Pull failed: %v", err), 500)
			return
		}
	} else {
		log.Printf("[sync] Pull successful")
	}

	// Stage again after merge
	_, _ = wTree.Add(".")

	// Push
	log.Printf("[sync] Pushing to origin master")
	err = repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Auth:       auth,
		RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
	})
	if err != nil {
		log.Printf("[sync] Push error: %v", err)
		http.Error(w, fmt.Sprintf("Push failed: %v", err), 500)
		return
	}
	log.Printf("[sync] Push successful")

	w.Write([]byte("Synced successfully."))
}
'''
        replace_function("backend/handlers.go", "handleSync", new_handleSync)

    commit_msg = (
        "feat(sync): add extensive server-side logging for sync debugging\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()