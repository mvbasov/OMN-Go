// --- OMN-Go Server Extensions ---
// These modules interact with the Go backend API. They will cleanly bypass themselves
// if the user is merely viewing an exported HTML file locally without the server.

if (window.location.protocol !== 'file:') {
    const Logger = (function() {
        // runSync is the single place that talks to /api/sync. It always
        // POSTs action/force/message together and always expects a JSON
        // {status, message} response — the backend previously only read
        // "action" from the URL query string while this file posted it in
        // the body, so the action was silently ignored and every request
        // fell back to a plain "pull". Both this file and the inline
        // conflict-modal script in index.html now go through this one
        // function so the two can't drift out of sync with each other.
        window.runSync = async function(action, opts) {
            opts = opts || {};
            const fd = new URLSearchParams();
            fd.append('action', action);
            if (opts.force) fd.append('force', 'true');
            if (opts.message) fd.append('message', opts.message);

            let data;
            try {
                const res = await fetch('/api/sync', { method: 'POST', body: fd });
                data = await res.json();
            } catch (e) {
                alert('Sync error: ' + e);
                return null;
            }

            const modal = document.getElementById('conflict-modal');
            switch (data.status) {
                case 'success':
                    if (modal) modal.classList.add('hidden');
                    return data;
                case 'conflict':
                    if (modal) {
                        modal.classList.remove('hidden');
                    } else {
                        const choice = confirm('Conflict! OK to Force Pull (Keep Untracked), Cancel to Mark Files.');
                        if (choice) window.runSync('pull_force');
                        else window.runSync('pull_mark');
                    }
                    return data;
                case 'push_conflict':
                    alert('Push rejected: the remote has new commits. Please pull first, then try pushing again.');
                    return data;
                case 'needs_commit_message':
                    alert('Please provide a commit message.');
                    return data;
                default:
                    alert('Sync failed: ' + (data.message || 'unknown error'));
                    return data;
            }
        };

        window.syncAction = async function (action) {
            let forceCb = document.getElementById('forceSyncCb');
            let force = forceCb && forceCb.checked;
            if (force) {
                if (!confirm("WARNING: Force " + action + " is a destructive operation that may overwrite remote or local changes. Are you sure?")) {
                    return;
                }
            }
            if (forceCb) forceCb.checked = false;

            if (action === 'upload') {
                // Uploads always go through the commit-message modal, which
                // also shows the file list and handles "nothing to commit".
                previewAndCommit(force);
                return;
            }

            const data = await window.runSync(action, { force });
            if (data && data.status === 'success') {
                if (confirm('Sync complete.\n\nWould you like to reload the page now to see updated content?')) {
                    window.location.reload();
                }
            }
        }
        // Export to global scope to preserve HTML onclick attributes

    window.saveConfig = async function() {
        const form = document.getElementById('configForm');
        if (!form) { alert('Config form not found'); return; }
        const fd = new FormData(form);
        try {
            const res = await fetch('/api/config', { method: 'POST', body: fd });
            if (res.ok) {
                alert('Configuration saved. Reloading...');
                window.location.reload();
            } else {
                let msg = await res.text();
                alert('Failed to save configuration: ' + msg);
            }
        } catch (e) {
            alert('Network error: ' + e);
        }
    };
        return { syncAction };
    })();

    window.previewAndCommit = async function(force) {
        try {
            const res = await fetch('/api/sync/preview?action=upload');
            if (!res.ok) {
                alert('Failed to get pending changes');
                return;
            }
            const files = await res.json();
            if (!files || files.length === 0) {
                alert('Nothing to commit');
                return;
            }
            var listEl = document.getElementById('commitFileList');
            if (listEl) listEl.textContent = files.join('\n');
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
        hideCommitModal();

        const data = await window.runSync('upload', { force, message });
        if (data && data.status === 'success') {
            if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                window.location.reload();
            }
        }
    };

    window.hideCommitModal = function() {
        document.getElementById('commitModal').style.display = 'none';
        document.getElementById('commitMessage').value = '';
    };

    window.loadNoteIntoEditor = async function() {
        const res = await fetch('/api/getnote?name=' + encodeURIComponent(currentNote));
        if (res.ok) {
            document.getElementById('editor').value = await res.text();
        }
    };

    window.toggleMode = async function() {
        if (currentMode === 'view') {
            if (typeof USE_INTERNAL_ED !== 'undefined' && !USE_INTERNAL_ED) {
                var ext = (typeof PAGE_EXT !== 'undefined' && PAGE_EXT) ? PAGE_EXT : '.md';
                window.location.replace('/api/edit-external?name=' + encodeURIComponent(currentNote + ext));
                return;
            }

            await loadNoteIntoEditor();

            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');

            editor.style.display = 'block';
            preview.style.display = 'none';
            btn.innerHTML = '<i class="material-icons" title="Switch to View Mode">visibility</i>';
            document.getElementById('saveBtn').style.display = 'block';
            document.getElementById('metaToggleBtn').style.display = 'none';
            document.getElementById('metadataPanel').classList.add('hidden');
            currentMode = 'edit';
        } else {
            const editor = document.getElementById('editor');
            const preview = document.getElementById('preview');
            const btn = document.getElementById('toggleBtn');

            editor.style.display = 'none';
            preview.style.display = 'block';
            btn.innerHTML = '<i class="material-icons" title="Switch to Edit Mode">edit</i>';
            document.getElementById('saveBtn').style.display = 'none';
            document.getElementById('metaToggleBtn').style.display = 'block';
            currentMode = 'view';
        }
    };

    window.setupEditorDragDrop = function() {
        const editor = document.getElementById('editor');
        if (!editor) return;
        editor.addEventListener('dragover', e => e.preventDefault());
        editor.addEventListener('drop', async e => {
            e.preventDefault();
            if(e.dataTransfer.files.length > 0) {
                const fd = new FormData();
                fd.append('image', e.dataTransfer.files[0]);
                const res = await fetch('/api/upload', { method: 'POST', body: fd });
                if(res.ok) {
                    const text = await res.text();
                    const cursor = editor.selectionStart;
                    editor.value = editor.value.substring(0, cursor) + text + editor.value.substring(cursor);
                    editor.dispatchEvent(new Event('input'));
                }
            }
        });
    };
    document.addEventListener('DOMContentLoaded', window.setupEditorDragDrop);

    window.login = async function() {
        const pwd = document.getElementById('pwdInput').value;
        const res = await fetch('/login', {
            method: 'POST',
            headers: {'Content-Type': 'application/x-www-form-urlencoded'},
            body: 'password=' + encodeURIComponent(pwd)
        });
        if(res.ok) {
            document.getElementById('loginOverlay').style.display = 'none';
            document.getElementById('mainUI').style.display = 'flex';
            checkRole();
        } else {
            alert('Invalid Password');
        }
    };

    window.createNewPage = async function() {
        let title = prompt("Enter New Page Title:");
        if (!title) return;
        let camel = toCamelCase(title);
        let safeName = camel.replace(/[^a-zA-Z0-9-]/g, '-');
        let fileName = prompt("Confirm File Name:", safeName);
        if (!fileName) return;

        let src = typeof currentNote !== 'undefined' ? currentNote : 'Welcome';
        const fd = new URLSearchParams();
        fd.append('source', src);
        fd.append('target', fileName);
        fd.append('title', title);

        const res = await fetch('/api/newpage', { method: 'POST', body: fd });
        if (res.ok) {
            // The server resolves fileName relative to the current page's
            // directory (a bare name becomes a sibling of src, not a
            // root-level page), so the actual created page may live at
            // e.g. "local/test" even though fileName was just "test".
            // Redirect using what the server tells us it actually created.
            const resolvedTarget = await res.text();
            window.location.href = '/' + resolvedTarget + '.html?edit=true';
        } else {
            alert("Failed to create new page!");
        }
    };

    window.saveNote = async function() {
        let content = document.getElementById('editor').value;
        const fd = new URLSearchParams();
        fd.append('name', currentNote);
        fd.append('content', content);
        const res = await fetch('/api/save', { method: 'POST', body: fd });
        if(res.ok) {
            alert('Note saved!');
            window.location.reload();
        } else {
            alert('Failed to save!');
        }
    };

    window.submitQuickNote = async function() {
        const fd = new URLSearchParams();
        fd.append('note', document.getElementById('quickText').value);
        const res = await fetch('/api/quick', { method: 'POST', body: fd });
        if(res.ok) {
            document.getElementById('quickText').value = '';
            document.getElementById('quickPanel').classList.add('hidden');
            alert('Saved!');
            window.location.reload();
        }
    };

    window.submitBookmark = async function() {
        const fd = new URLSearchParams();
        fd.append('url', document.getElementById('bmUrl').value);
        fd.append('title', document.getElementById('bmTitle').value);
        fd.append('tags', document.getElementById('bmTags').value);
        fd.append('notes', document.getElementById('bmNotes').value);
        const res = await fetch('/api/bookmark', { method: 'POST', body: fd });
        if(res.ok) {
            document.getElementById('bmPanel').classList.add('hidden');
            document.querySelectorAll('#bmPanel input, #bmPanel textarea').forEach(el => el.value = '');
            alert('Saved!');
            window.location.reload();
        }
    };

    window.checkSession = async function() {
        // Unhide UI if role cookies exist
        if (document.cookie.includes('session_role=')) {
            document.getElementById('loginOverlay').style.display = 'none';
            document.getElementById('mainUI').style.display = 'flex';
            checkRole();
        } else {
            // Check if server is configured with public role or check backend
            const test = await fetch('/api/config');
            if (test.status === 401) {
                document.getElementById('loginOverlay').style.display = 'flex';
                document.getElementById('mainUI').style.display = 'none';
            } else {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
            }
        }
    };

    // GoOMN Log Interceptor - Bridges Go background logs to JS UI
    document.addEventListener('DOMContentLoaded', () => {
        try {
            const logSource = new EventSource('/api/logs');
            logSource.onmessage = function(event) {
                let msg = event.data.trim();
                if(msg) {
                    console.log("[GO] " + msg);
                }
            };
        } catch(e) {
            console.error("Log source error:", e);
        }
    });

} else {
    console.warn("OMN-Go: Page opened locally. Server Extensions (Sync/SSE) safely disabled.");
    window.printDebug = function(funcName) { console.debug('\'' + funcName + '\' Not usable on standalone page'); }
    
    window.loadNoteIntoEditor = function() { printDebug('loadNoteIntoEditor'); };
    window.toggleMode = function() { printDebug('toggleMode'); };
    window.setupEditorDragDrop = function() { printDebug('setupEditorDragDrop'); };
    window.login = function() { printDebug('login'); };
    window.createNewPage = function() { printDebug('createNewPage'); };
    window.saveNote = function() { printDebug('saveNote'); };
    window.submitQuickNote = function() { printDebug('submitQuickNote'); };
    window.submitBookmark = function() { printDebug('submitBookmark'); };
    window.checkSession = function() { printDebug('checkSession'); };
}
