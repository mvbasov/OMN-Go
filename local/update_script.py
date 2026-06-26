#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*. Raise ValueError if *old* missing."""
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    """'1.4.42' → '1.4.43'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto-detect current version and bump it
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Patch backend/handlers.go (Fix Committer & Split Sync)
    old_handlers = """\
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
				Name:  GetConfigAuthor(),
				Email: "sync@omn-go.local",
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
}"""

    new_handlers = """\
	wTree, _ := repo.Worktree()

	action := r.FormValue("action")
	force := r.FormValue("force") == "true"

	// Stage and commit local changes first
	log.Printf("[sync] Staging all changes")
	_, err = wTree.Add(".")
	if err == nil {
		status, _ := wTree.Status()
		if !status.IsClean() {
			log.Printf("[sync] Uncommitted changes detected, committing")
			authorName := GetConfigAuthor()
			authorEmail := strings.ReplaceAll(strings.ToLower(authorName), " ", ".") + "@omn-go.local"
			sig := &object.Signature{
				Name:  authorName,
				Email: authorEmail,
				When:  time.Now(),
			}
			_, err = wTree.Commit("Local changes before sync", &git.CommitOptions{
				Author:    sig,
				Committer: sig, // CRITICAL: Fixes 'function not implemented'
			})
			if err != nil {
				log.Printf("[sync] Commit error: %v", err)
				http.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)
				return
			}
		} else {
			log.Printf("[sync] Nothing to commit")
		}
	}

	if action == "download" {
		if force {
			log.Printf("[sync] Force Download: Fetching and Hard Resetting")
			err = repo.Fetch(&git.FetchOptions{RemoteName: "origin", Auth: auth})
			if err != nil && err != git.NoErrAlreadyUpToDate {
				http.Error(w, fmt.Sprintf("Fetch failed: %v", err), 500)
				return
			}
			ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", "master"), true)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to find origin/master: %v", err), 500)
				return
			}
			err = wTree.Reset(&git.ResetOptions{Commit: ref.Hash(), Mode: git.HardReset})
			if err != nil {
				http.Error(w, fmt.Sprintf("Hard reset failed: %v", err), 500)
				return
			}
		} else {
			log.Printf("[sync] Pulling from origin master")
			err = wTree.Pull(&git.PullOptions{
				RemoteName:    "origin",
				Auth:          auth,
				ReferenceName: plumbing.NewBranchReferenceName("master"),
				SingleBranch:  true,
			})
			if err != nil && err != git.NoErrAlreadyUpToDate && !strings.Contains(err.Error(), "couldn't find remote ref") {
				log.Printf("[sync] Pull error: %v", err)
				http.Error(w, fmt.Sprintf("Pull failed: %v", err), 500)
				return
			}
		}
	} else if action == "upload" {
		log.Printf("[sync] Pushing to origin master (Force: %v)", force)
		err = repo.Push(&git.PushOptions{
			RemoteName: "origin",
			Auth:       auth,
			RefSpecs:   []gitconfig.RefSpec{"refs/heads/master:refs/heads/master"},
			Force:      force,
		})
		if err != nil && err != git.NoErrAlreadyUpToDate {
			log.Printf("[sync] Push error: %v", err)
			http.Error(w, fmt.Sprintf("Push failed: %v", err), 500)
			return
		}
	} else {
		http.Error(w, "Invalid action. Use 'upload' or 'download'.", 400)
		return
	}

	w.Write([]byte("Sync action completed successfully."))
}"""
    patch_file("backend/handlers.go", old_handlers, new_handlers)

    # 3. Patch backend/frontend/index.html (Update UI buttons)
    old_html = """<button onclick="syncNow()" class="admin-only"><i class="material-icons">sync</i></button>"""
    new_html = """<label class="admin-only" style="display:flex;align-items:center;font-size:12px;margin-right:2px;cursor:pointer;color:#555;" title="Force next sync action (destructive)"><input type="checkbox" id="forceSyncCb" style="margin:0 2px 0 0;">Force</label>
                    <button onclick="syncAction('download')" class="admin-only" title="Download (Pull)"><i class="material-icons">cloud_download</i></button>
                    <button onclick="syncAction('upload')" class="admin-only" title="Upload (Push)"><i class="material-icons">cloud_upload</i></button>"""
    patch_file("backend/frontend/index.html", old_html, new_html)

    # 4. Patch backend/frontend/html/js/omn-go-core.js (Update JS logic)
    old_js = """async function syncNow() {
            const res = await fetch('/api/sync', { method: 'POST' });
            if (res.ok) {
                alert('Sync complete!');
                window.location.reload();
            } else {
                let msg = await res.text();
                console.error('OMN-Go sync failed:', msg);
                alert('Sync failed: ' + msg + '\\n\\nOpen console (F12) to copy error details.');
            }
        }"""
        
    new_js = """async function syncAction(action) {
            let force = document.getElementById('forceSyncCb') && document.getElementById('forceSyncCb').checked;
            if (force) {
                if (!confirm("WARNING: Force " + action + " is a destructive operation that may overwrite remote or local changes. Are you sure?")) {
                    return;
                }
            }
            const fd = new URLSearchParams();
            fd.append('action', action);
            if (force) fd.append('force', 'true');
            
            const res = await fetch('/api/sync', { method: 'POST', body: fd });
            
            if (force && document.getElementById('forceSyncCb')) {
                document.getElementById('forceSyncCb').checked = false;
            }
            
            if (res.ok) {
                alert(action.charAt(0).toUpperCase() + action.slice(1) + ' complete!');
                window.location.reload();
            } else {
                let msg = await res.text();
                console.error('OMN-Go sync failed:', msg);
                alert(action.charAt(0).toUpperCase() + action.slice(1) + ' failed: ' + msg + '\\n\\nOpen console (F12) to copy error details.');
            }
        }"""
    patch_file("backend/frontend/html/js/omn-go-core.js", old_js, new_js)

    # 5. Print standardised Git commit message
    commit_msg = (
        "feat(core): fix sync author signing and split into upload/download\n\n"
        "- Resolve 'function not implemented' error by explicitly passing Committer sig\n"
        "- Generate email fallback properly using config author name\n"
        "- Split unified sync into distinct download (pull) and upload (push) buttons\n"
        "- Add UI checkbox for one-time forced execution with secondary confirmation alert\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()