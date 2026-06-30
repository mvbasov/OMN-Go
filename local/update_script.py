#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def safe_patch_file(path, old, new):
    """
    Replace *old* with *new* in *path*, but if *old* is already missing
    and *new* is present, the patch is considered already applied → do nothing.
    """
    content = read_file(path)
    if old in content:
        content = content.replace(old, new, 1)
        write_file(path, content)
    elif new not in content:
        raise ValueError(f"❌ Patch target not found in {path} (and replacement also missing):\n{old[:120]}")
    # else: already patched → skip silently

def increment_version(ver_str):
    """'1.3.35' → '1.3.36'"""
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version and bump it
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

    # 2. Apply feature patches

    # --- backend/git_helper.go patches (already partially applied, use safe_patch) ---

    # a) Ensure commitLocalChanges signature includes message parameter
    safe_patch_file("backend/git_helper.go",
        "func commitLocalChanges(repo *git.Repository, wTree *git.Worktree) (bool, error) {",
        "func commitLocalChanges(repo *git.Repository, wTree *git.Worktree, message string) (bool, error) {")

    # b) Ensure commit uses dynamic message
    safe_patch_file("backend/git_helper.go",
        '\tcommitHash, err := wTree.Commit("Local changes before sync", &git.CommitOptions{',
        '\tcommitHash, err := wTree.Commit(message, &git.CommitOptions{')

    # c) Ensure handleSync reads commit message on upload
    old_commit_block = '''\tcommitted, err := commitLocalChanges(repo, wTree)
\tif err != nil {
\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)
\t\treturn
\t}
\tif !committed && action == "upload" {
\t\tw.Write([]byte("Nothing to push"))
\t\treturn
\t}'''
    new_commit_block = '''\tcommitMsg := "Local changes before sync"
\tif action == "upload" {
\t\tcommitMsg = r.FormValue("message")
\t\tif commitMsg == "" {
\t\t\thttp.Error(w, "Commit message required for upload", 400)
\t\t\treturn
\t\t}
\t}
\tcommitted, err := commitLocalChanges(repo, wTree, commitMsg)
\tif err != nil {
\t\thttp.Error(w, fmt.Sprintf("Commit failed: %v", err), 500)
\t\treturn
\t}
\tif !committed && action == "upload" {
\t\tw.Write([]byte("Nothing to push"))
\t\treturn
\t}'''
    safe_patch_file("backend/git_helper.go", old_commit_block, new_commit_block)

    # d) Add handleSyncPreview function (check if already present)
    git_helper = read_file("backend/git_helper.go")
    if "func handleSyncPreview" not in git_helper:
        insert_point = '\n// ---------------------------------------------------------------\n// Utilities\n// ---------------------------------------------------------------'
        new_func = '''
func handleSyncPreview(w http.ResponseWriter, r *http.Request) {
\tif r.Method != "GET" {
\t\thttp.Error(w, "Method Not Allowed", 405)
\t\treturn
\t}
\taction := r.URL.Query().Get("action")
\tif action != "upload" {
\t\thttp.Error(w, "Only upload preview supported", 400)
\t\treturn
\t}

\trepo, err := getOrInitRepo()
\tif err != nil {
\t\thttp.Error(w, fmt.Sprintf("Repo init failed: %v", err), 500)
\t\treturn
\t}
\twTree, err := repo.Worktree()
\tif err != nil {
\t\thttp.Error(w, fmt.Sprintf("Worktree error: %v", err), 500)
\t\treturn
\t}

\tmatcher, err := loadGitignoreMatcher(wTree)
\tif err != nil {
\t\tmatcher = gitignore.NewMatcher(nil)
\t}

\tstatus, err := wTree.Status()
\tif err != nil {
\t\thttp.Error(w, fmt.Sprintf("Status error: %v", err), 500)
\t\treturn
\t}

\tvar files []string
\tfor name, fileStat := range status {
\t\t// Skip ignored and root config.json
\t\tif matcher != nil && matcher.Match(strings.Split(name, string(filepath.Separator)), false) {
\t\t\tcontinue
\t\t}
\t\tif name == "config.json" {
\t\t\tcontinue
\t\t}
\t\tif fileStat.Worktree != git.Unmodified || fileStat.Staging != git.Unmodified {
\t\t\tfiles = append(files, name)
\t\t}
\t}

\tw.Header().Set("Content-Type", "application/json")
\tjson.NewEncoder(w).Encode(files)
}
'''
        # Insert before Utilities
        git_helper = git_helper.replace(insert_point, new_func + insert_point)
        write_file("backend/git_helper.go", git_helper)
        print("Added handleSyncPreview function")
    else:
        print("handleSyncPreview already exists, skipping")

    # --- backend/server.go: register /api/sync/preview route ---
    safe_patch_file("backend/server.go",
        '\t\tmux.HandleFunc("/api/sync", authMiddleware(handleSync, true))',
        '\t\tmux.HandleFunc("/api/sync", authMiddleware(handleSync, true))\n\t\tmux.HandleFunc("/api/sync/preview", authMiddleware(handleSyncPreview, true))')

    # --- frontend/index.html: add commit modal after bmPanel ---
    index_path = "backend/frontend/index.html"
    index_content = read_file(index_path)
    if 'id="commitModal"' in index_content:
        print("Commit modal already in index.html, skipping")
    else:
        bm_end = '    </div>\n\n    <script>\n        /* OMN_GO_PAGE_NAME_JS */'
        commit_modal = '''    </div>

    <!-- Commit Message Modal -->
    <div id="commitModal" class="overlay" style="display:none;">
        <div class="modal" style="width:500px; max-height:80vh; overflow-y:auto;">
            <h2>Commit Changes</h2>
            <p>Files to be committed:</p>
            <pre id="commitFileList" style="background:#f5f5f5; padding:10px; max-height:200px; overflow-y:auto; white-space:pre-wrap;"></pre>
            <textarea id="commitMessage" placeholder="Enter commit message..." style="width:100%; height:80px; margin-top:10px;"></textarea>
            <div class="modal-buttons-row" style="margin-top:10px;">
                <button onclick="commitAndUpload()" class="admin-only">Commit & Push</button>
                <button onclick="hideCommitModal()" class="btn-cancel">Cancel</button>
            </div>
        </div>
    </div>

    <script>
        /* OMN_GO_PAGE_NAME_JS */'''
        if bm_end in index_content:
            index_content = index_content.replace(bm_end, commit_modal, 1)
            write_file(index_path, index_content)
            print("Commit modal inserted into index.html")
        else:
            raise ValueError("Could not find bm_end marker in index.html")

    # --- frontend/html/js/omn-go-sse.js patches ---

    sse_path = "backend/frontend/html/js/omn-go-sse.js"
    sse_content = read_file(sse_path)

    # Check if syncAction already routes upload to previewAndCommit
    if 'previewAndCommit(force);' in sse_content:
        print("syncAction already patched, skipping")
    else:
        # Replace the entire syncAction function body (preserve indentation)
        start_marker = re.search(r'(\s*)async function syncAction\(action\)\s*\{', sse_content)
        if not start_marker:
            raise ValueError("Could not find syncAction in omn-go-sse.js")
        indent = start_marker.group(1)
        # Count braces to find the end of the function
        pos = start_marker.end() - 1
        brace_count = 0
        while pos < len(sse_content):
            if sse_content[pos] == '{':
                brace_count += 1
            elif sse_content[pos] == '}':
                brace_count -= 1
                if brace_count == 0:
                    func_end = pos + 1
                    break
            pos += 1
        if brace_count != 0:
            raise ValueError("Braces unbalanced in syncAction")
        old_body = sse_content[start_marker.start():func_end]

        new_body = '''{}async function syncAction(action) {{
            let force = document.getElementById('forceSyncCb') && document.getElementById('forceSyncCb').checked;
            if (force) {{
                if (!confirm("WARNING: Force " + action + " is a destructive operation that may overwrite remote or local changes. Are you sure?")) {{
                    return;
                }}
            }}
            if (action === 'upload') {{
                // Use new commit message flow
                previewAndCommit(force);
                return;
            }}
            const fd = new URLSearchParams();
            fd.append('action', action);
            if (force) fd.append('force', 'true');
            
            const res = await fetch('/api/sync', {{ method: 'POST', body: fd }});
            
            if (force && document.getElementById('forceSyncCb')) {{
                document.getElementById('forceSyncCb').checked = false;
            }}

            const capAction = action.charAt(0).toUpperCase() + action.slice(1);
            
            if (res.ok) {{
                let msg = await res.text();
                if (msg.includes('Nothing to push')) {{
                    alert(msg);
                }} else if (confirm(msg + '\n\nWould you like to reload the page now to see updated content (console will be reset)?')) {{
                    window.location.reload();
                }}
            }} else {{
                let msg = await res.text();
                console.error('OMN-Go sync failed:', msg);
                alert(capAction + ' failed: ' + msg + '\n\nOpen console to copy error details.');
            }}
        }}'''.format(indent)
        # The new body format uses double braces because of Python's string format; need to adjust
        # Actually, we can use a raw string and not use .format. Simpler: construct with concatenation.
        new_body_str = indent + "async function syncAction(action) {\n" \
            + indent + "    let force = document.getElementById('forceSyncCb') && document.getElementById('forceSyncCb').checked;\n" \
            + indent + "    if (force) {\n" \
            + indent + "        if (!confirm(\"WARNING: Force \" + action + \" is a destructive operation that may overwrite remote or local changes. Are you sure?\")) {\n" \
            + indent + "            return;\n" \
            + indent + "        }\n" \
            + indent + "    }\n" \
            + indent + "    if (action === 'upload') {\n" \
            + indent + "        // Use new commit message flow\n" \
            + indent + "        previewAndCommit(force);\n" \
            + indent + "        return;\n" \
            + indent + "    }\n" \
            + indent + "    const fd = new URLSearchParams();\n" \
            + indent + "    fd.append('action', action);\n" \
            + indent + "    if (force) fd.append('force', 'true');\n" \
            + indent + "    \n" \
            + indent + "    const res = await fetch('/api/sync', { method: 'POST', body: fd });\n" \
            + indent + "    \n" \
            + indent + "    if (force && document.getElementById('forceSyncCb')) {\n" \
            + indent + "        document.getElementById('forceSyncCb').checked = false;\n" \
            + indent + "    }\n" \
            + indent + "\n" \
            + indent + "    const capAction = action.charAt(0).toUpperCase() + action.slice(1);\n" \
            + indent + "    \n" \
            + indent + "    if (res.ok) {\n" \
            + indent + "        let msg = await res.text();\n" \
            + indent + "        if (msg.includes('Nothing to push')) {\n" \
            + indent + "            alert(msg);\n" \
            + indent + "        } else if (confirm(msg + '\\n\\nWould you like to reload the page now to see updated content (console will be reset)?')) {\n" \
            + indent + "            window.location.reload();\n" \
            + indent + "        }\n" \
            + indent + "    } else {\n" \
            + indent + "        let msg = await res.text();\n" \
            + indent + "        console.error('OMN-Go sync failed:', msg);\n" \
            + indent + "        alert(capAction + ' failed: ' + msg + '\\n\\nOpen console to copy error details.');\n" \
            + indent + "    }\n" \
            + indent + "}"
        sse_content = sse_content.replace(old_body, new_body_str, 1)
        write_file(sse_path, sse_content)
        print("syncAction patched to route upload to previewAndCommit")

    # Insert new UI functions after Logger IIFE
    insert_marker = "    })();"
    new_ui_funcs = """    })();

    window.previewAndCommit = async function(force) {
        try {
            const res = await fetch('/api/sync/preview?action=upload');
            if (!res.ok) {
                alert('Failed to get pending changes');
                return;
            }
            const files = await res.json();
            if (files.length === 0) {
                alert('Nothing to commit');
                return;
            }
            document.getElementById('commitFileList').textContent = files.join('\\n');
            document.getElementById('commitModal').style.display = 'flex';
            window._commitForce = force;
        } catch(e) {
            alert('Error: ' + e);
        }
    };

    window.commitAndUpload = async function() {
        const message = document.getElementById('commitMessage').value.trim();
        if (!message) {
            alert('Please enter a commit message.');
            return;
        }
        const force = window._commitForce || false;
        const fd = new URLSearchParams();
        fd.append('action', 'upload');
        fd.append('message', message);
        if (force) fd.append('force', 'true');
        try {
            const res = await fetch('/api/sync', { method: 'POST', body: fd });
            if (force && document.getElementById('forceSyncCb')) {
                document.getElementById('forceSyncCb').checked = false;
            }
            if (res.ok) {
                let msg = await res.text();
                if (confirm(msg + '\\n\\nWould you like to reload the page now to see updated content?')) {
                    window.location.reload();
                }
            } else {
                let msg = await res.text();
                alert('Upload failed: ' + msg);
            }
        } catch(e) {
            alert('Error: ' + e);
        }
        hideCommitModal();
    };

    window.hideCommitModal = function() {
        document.getElementById('commitModal').style.display = 'none';
        document.getElementById('commitMessage').value = '';
    };"""

    if "window.previewAndCommit" in sse_content:
        print("UI functions already present, skipping")
    else:
        if insert_marker not in sse_content:
            raise ValueError("Insert marker not found in omn-go-sse.js")
        sse_content = sse_content.replace(insert_marker, new_ui_funcs, 1)
        write_file(sse_path, sse_content)
        print("UI functions inserted after Logger IIFE")

    # 3. Print commit message
    commit_msg = (
        "feat(sync): require commit message before push\n\n"
        "- Added commit message modal with list of staged files\n"
        "- New /api/sync/preview endpoint returns pending file names\n"
        "- Modified commitLocalChanges to accept a custom message\n"
        "- Upload action now shows commit dialog before push\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()