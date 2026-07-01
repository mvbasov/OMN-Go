import os
import re

SYNC_REPO_GO = """func SyncRepo(action string) error {
	r, err := getOrInitRepo()
	if err != nil {
		return err
	}
	w, err := r.Worktree()
	if err != nil {
		return err
	}
	auth := getSSHAuth()

	if action == "push" {
		err = r.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       auth,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			return err
		}
		return nil
	}

	if strings.HasPrefix(action, "pull") {
		// Force fetch to guarantee we have the remote tree locally
		err = r.Fetch(&git.FetchOptions{
			RemoteName: "origin",
			Auth:       auth,
			Force:      true,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			log.Printf("[sync] Fetch info: %v", err)
		}

		remoteRef, err := r.Reference(plumbing.ReferenceName("refs/remotes/origin/master"), true)
		if err != nil {
			return fmt.Errorf("remote master not found: %v", err)
		}

		if action == "pull" || action == "pull_ff" {
			err = w.Pull(&git.PullOptions{
				RemoteName: "origin",
				Auth:       auth,
				Force:      false,
			})
			if err == git.ErrNonFastForwardUpdate || err == git.ErrUnstagedChanges {
				return fmt.Errorf("CONFLICT_DETECTED")
			}
			if err != nil && err != git.NoErrAlreadyUpToDate {
				return err
			}
			return nil
		}

		if action == "pull_force" {
			// Force Merge to Remote State (Safely overwrites tracked, ignores/keeps untracked)
			ref := plumbing.NewHashReference(plumbing.ReferenceName("refs/heads/master"), remoteRef.Hash())
			err = r.Storer.SetReference(ref)
			if err != nil {
				return err
			}

			err = w.Checkout(&git.CheckoutOptions{
				Branch: plumbing.ReferenceName("refs/heads/master"),
				Force:  true,
			})
			return err
		}

		if action == "pull_mark" {
			// Inject Merge Conflict Markers
			remoteCommit, _ := r.CommitObject(remoteRef.Hash())
			remoteTree, _ := remoteCommit.Tree()
			status, _ := w.Status()

			for path, fileStatus := range status {
				if fileStatus.Worktree == git.Modified || fileStatus.Staging == git.Modified {
					file, err := w.Filesystem.Open(path)
					if err != nil {
						continue
					}
					localContent, _ := io.ReadAll(file)
					file.Close()

					remoteFile, err := remoteTree.File(path)
					if err == nil {
						remoteContentStr, _ := remoteFile.Contents()
						if string(localContent) != remoteContentStr {
							conflictText := fmt.Sprintf("<<<<<<< LOCAL (Your changes)\\n%s=======\\n%s>>>>>>> REMOTE (Incoming from origin)\\n", string(localContent), remoteContentStr)
							outFile, err := w.Filesystem.OpenFile(path, os.O_RDWR|os.O_TRUNC, 0644)
							if err == nil {
								outFile.Write([]byte(conflictText))
								outFile.Close()
							}
						}
					}
				}
			}
			// Reset index to remote hash to allow user to resolve and commit
			w.Reset(&git.ResetOptions{Commit: remoteRef.Hash(), Mode: git.MixedReset})
			return nil
		}
	}
	return fmt.Errorf("unknown sync action: %s", action)
}"""

HANDLE_SYNC_GO = """func handleSync(w http.ResponseWriter, r *http.Request) {
	action := r.URL.Query().Get("action")
	if action == "" {
		action = "pull"
	}

	// Ensure local changes are committed before attempting to sync
	repo, err := getOrInitRepo()
	if err == nil {
		wt, err := repo.Worktree()
		if err == nil {
			commitLocalChanges(repo, wt, "Auto-commit before sync")
		}
	}

	err = SyncRepo(action)
	if err != nil {
		if err.Error() == "CONFLICT_DETECTED" {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"conflict","message":"Merge conflict detected."}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(fmt.Sprintf(`{"status":"error","message":"%v"}`, err)))
		return
	}

	if action == "pull" || action == "pull_force" || action == "pull_mark" {
		err = SyncRepo("push")
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"status":"error","message":"Pull succeeded, but Push failed: %v"}`, err)))
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"success"}`))
}"""

HTML_MODAL_INJECTION = """
<!-- OMN-Go Sync Conflict Modal -->
<div id="conflict-modal" class="fixed inset-0 bg-black bg-opacity-50 hidden flex items-center justify-center z-50" style="background-color: rgba(0,0,0,0.7);">
    <div class="bg-white dark:bg-gray-800 p-6 rounded-lg shadow-xl text-black dark:text-white max-w-sm w-full mx-4 border border-gray-600">
        <h3 class="text-xl font-bold mb-4 border-b border-gray-300 dark:border-gray-600 pb-2">⚠️ Merge Conflict</h3>
        <p class="mb-4 text-sm text-gray-700 dark:text-gray-300">Local changes conflict with the remote repository. Please choose how to resolve this:</p>
        <button onclick="window.performSync('pull_force')" class="w-full mb-3 bg-red-500 hover:bg-red-600 text-white font-bold p-3 rounded transition-colors shadow">
            Force Merge to Remote<br><span class="text-xs font-normal opacity-80">(Overwrites tracked files, keeps untracked safe)</span>
        </button>
        <button onclick="window.performSync('pull_mark')" class="w-full mb-3 bg-yellow-500 hover:bg-yellow-600 text-black font-bold p-3 rounded transition-colors shadow">
            Mark Conflicts in Files<br><span class="text-xs font-normal opacity-80">(Injects &lt;&lt;&lt; ==== &gt;&gt;&gt; for manual fixing)</span>
        </button>
        <button onclick="window.performSync('abort')" class="w-full bg-gray-500 hover:bg-gray-600 text-white font-bold p-3 rounded transition-colors shadow">
            Abort Operation
        </button>
    </div>
</div>
<script>
window.performSync = async function(action = 'pull') {
    const modal = document.getElementById('conflict-modal');
    if (action === 'abort') {
        if (modal) modal.classList.add('hidden');
        return;
    }
    if (modal) modal.classList.add('hidden');
    
    console.log('[sync] Executing:', action);
    try {
        const res = await fetch(`/api/sync?action=${action}`);
        const data = await res.json();
        
        if (data.status === 'conflict') {
            if (modal) {
                modal.classList.remove('hidden');
            } else {
                const choice = confirm("Conflict! OK to Force Pull (Keep Untracked), Cancel to Mark Files.");
                if (choice) window.performSync('pull_force');
                else window.performSync('pull_mark');
            }
        } else if (data.status === 'success') {
            console.log('[sync] Success');
            if (action === 'pull_force' || action === 'pull_mark') {
                location.reload();
            } else if (action === 'pull' || action === 'pull_ff') {
                console.log('Sync Complete!');
            }
        } else {
            alert('Sync failed: ' + data.message);
        }
    } catch (e) {
        alert('Sync Error: ' + e);
    }
};
</script>
</body>"""

def replace_func_block(content, func_name, new_code):
    # Added re.IGNORECASE to mathematically match syncRepo or SyncRepo
    pattern = re.compile(r'func\s+' + func_name + r'\s*\([^)]*\)[^{]*{', re.IGNORECASE)
    match = pattern.search(content)
    if not match: return content
    
    start_idx = match.end() - 1
    brace_count = 0
    end_idx = -1
    in_string = in_char = in_backtick = escape = False
    
    for i in range(start_idx, len(content)):
        char = content[i]
        if escape:
            escape = False
            continue
        if char == '\\':
            escape = True
            continue
        if char == '"' and not in_char and not in_backtick:
            in_string = not in_string
        elif char == "'" and not in_string and not in_backtick:
            in_char = not in_char
        elif char == '`' and not in_string and not in_char:
            in_backtick = not in_backtick
            
        if in_string or in_char or in_backtick: continue
            
        if char == '{': brace_count += 1
        elif char == '}':
            brace_count -= 1
            if brace_count == 0:
                end_idx = i
                break
                
    if end_idx != -1:
        return content[:match.start()] + new_code + content[end_idx+1:]
    return content

def ensure_imports(content, imports_to_add):
    if not imports_to_add: return content
    import_match = re.search(r'import\s+\((.*?)\)', content, re.DOTALL)
    if import_match:
        existing = import_match.group(1)
        for imp in imports_to_add:
            if f'"{imp}"' not in existing:
                existing += f'\n\t"{imp}"'
        content = content[:import_match.start(1)] + existing + content[import_match.end(1):]
    return content

def run_patch():
    print("[*] Starting backend AST patching for Sync Engine (Fixing compile errors)...")
    
    # 1. Patch Go Files
    for root, dirs, files in os.walk('.'):
        for file in files:
            if file.endswith('.go'):
                path = os.path.join(root, file)
                with open(path, 'r', encoding='utf-8') as f:
                    content = f.read()
                
                orig = content
                
                # Using re.search with ignore case to check if function exists before replacing
                if re.search(r'func\s+(?i)SyncRepo', content):
                    content = replace_func_block(content, 'SyncRepo', SYNC_REPO_GO)
                    content = ensure_imports(content, ['io', 'os', 'strings', 'path/filepath', 'fmt', 'log', 'github.com/go-git/go-git/v5/plumbing', 'github.com/go-git/go-git/v5/plumbing/object'])
                    content = re.sub(r'(?i)SyncRepo\(\)', 'SyncRepo("pull")', content)

                if re.search(r'func\s+(?i)handleSync', content):
                    content = replace_func_block(content, 'handleSync', HANDLE_SYNC_GO)

                if content != orig:
                    with open(path, 'w', encoding='utf-8') as f:
                        f.write(content)
                    print(f"[+] Successfully patched Go logic in {path}")
                    
            # 2. Patch HTML Frontend for Modal
            if file == 'index.html':
                path = os.path.join(root, file)
                with open(path, 'r', encoding='utf-8') as f:
                    content = f.read()
                if 'id="conflict-modal"' not in content:
                    content = content.replace('</body>', HTML_MODAL_INJECTION)
                    with open(path, 'w', encoding='utf-8') as f:
                        f.write(content)
                    print(f"[+] Successfully injected Modal UI into {path}")

            # 3. Version Bump
            if file == 'version.go' or file == 'build.gradle':
                path = os.path.join(root, file)
                with open(path, 'r', encoding='utf-8') as f:
                    content = f.read()
                orig = content
                # Catching previous un-bumped versions as well
                content = re.sub(r'1\.5\.69|1\.5\.70', '1.5.71', content)
                content = re.sub(r'10569|10570', '10571', content)
                if orig != content:
                    with open(path, 'w', encoding='utf-8') as f:
                        f.write(content)
                    print(f"[+] Bumped version in {path} to 1.5.71 (10571)")

if __name__ == "__main__":
    run_patch()
    print("\n[=] Patching complete! Compile errors should now be resolved.")