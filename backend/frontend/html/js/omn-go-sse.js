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
                const body = await res.text();
                if (body === 'RestartRequired') {
                    // ShareLAN changed: the listen socket is bound once at
                    // startup, so the server must fully restart to rebind.
                    alert('LAN sharing changed - the application will now restart to apply it.\n\nDesktop: this page reloads automatically in a few seconds.\nAndroid: the app will close; reopen it manually.');
                    try { await fetch('/api/restart', { method: 'POST' }); } catch (e) { /* connection drops as the server exits - expected */ }
                    // Desktop: the replacement process is up within ~1-3s
                    // (bind retry included); reload to reconnect. On
                    // Android the whole app process exits before this
                    // timer matters.
                    setTimeout(function(){ window.location.reload(); }, 3000);
                    return;
                }
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

    // NOTE: In-page editing was removed. Editing a note now opens the
    // dedicated editor page (any URL with ?edit=true, served by the Go
    // backend and driven by omn-go-editor.js), which fetches the source
    // from /api/note itself. The view page therefore no longer embeds an
    // #editor textarea, and the old toggleMode / loadNoteIntoEditor /
    // setupEditorDragDrop / saveNote helpers that manipulated it are gone.

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

    // --- Server-backed SQLite (replacement for the removed WebSQL API) ---
    // Data lives server-side in <storage>/db/<name>.sqlite, so unlike the
    // old per-browser window.openDatabase, every device sees the same
    // data. Requires admin role (local connections qualify automatically).
    //
    // Modern API (preferred for new note scripts):
    //   const db = omnGoOpenDatabase('mydata');
    //   await db.exec('CREATE TABLE IF NOT EXISTS t(a,b)');
    //   const r = await db.exec('SELECT * FROM t WHERE a > ?', [5]);
    //   r.rows._array.forEach(row => console.log(row.a, row.b));
    //   await db.batch([['INSERT INTO t VALUES(?,?)', [1,2]],
    //                   ['INSERT INTO t VALUES(?,?)', [3,4]]]); // atomic
    window.omnGoOpenDatabase = function(name) {
        async function post(statements) {
            const res = await fetch('/api/sql', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ db: name, statements: statements })
            });
            const data = await res.json();
            if (data.status !== 'success') {
                const idx = (data.failed_statement !== undefined && data.failed_statement !== null)
                    ? ' (statement #' + data.failed_statement + ')' : '';
                throw new Error((data.message || 'SQL error') + idx);
            }
            return data.results;
        }

        // Server result -> WebSQL-shaped result set.
        function wrap(r) {
            const cols = r.columns || [];
            const arr = (r.rows || []).map(row => {
                const o = {};
                cols.forEach((c, i) => { o[c] = row[i]; });
                return o;
            });
            return {
                insertId: r.last_insert_id,
                rowsAffected: r.rows_affected,
                rows: { length: arr.length, item: i => arr[i], _array: arr }
            };
        }

        const db = {
            exec: async function(sql, args) {
                return wrap((await post([{ sql: sql, args: args || [] }]))[0]);
            },
            batch: async function(stmts) {
                const norm = stmts.map(s => Array.isArray(s)
                    ? { sql: s[0], args: s[1] || [] }
                    : { sql: s.sql, args: s.args || [] });
                return (await post(norm)).map(wrap);
            },
            // WebSQL-compatible: db.transaction(tx => tx.executeSql(...)).
            // All statements queued synchronously inside the callback run
            // as ONE atomic server-side transaction. Statements queued
            // from inside success callbacks run as a FOLLOW-UP atomic
            // batch (a separate transaction) - the one semantic
            // difference from real WebSQL, where the whole cascade shared
            // a transaction.
            transaction: function(cb, errCb, doneCb) {
                const queue = [];
                const tx = {
                    executeSql: function(sql, args, okCb, failCb) {
                        queue.push({ sql: sql, args: args || [], okCb: okCb, failCb: failCb });
                    }
                };
                try { cb(tx); } catch (e) { if (errCb) errCb(e); return; }
                (async () => {
                    while (queue.length) {
                        const batch = queue.splice(0, queue.length);
                        let results;
                        try {
                            results = await post(batch.map(q => ({ sql: q.sql, args: q.args })));
                        } catch (e) {
                            batch.forEach(q => { if (q.failCb) try { q.failCb(tx, e); } catch (_) {} });
                            if (errCb) errCb(e);
                            return;
                        }
                        batch.forEach((q, i) => {
                            if (q.okCb) try { q.okCb(tx, wrap(results[i])); } catch (_) {}
                        });
                        // okCb calls may have queued more statements; loop.
                    }
                    if (doneCb) doneCb();
                })();
            }
        };
        db.readTransaction = db.transaction;

        // Manual force-triggers for the git-tracked JSON backup (see
        // Database.md). Normal operation needs neither: export happens
        // automatically right before every git push, and restore happens
        // lazily the next time this database is opened after a pull
        // brings in newer JSON. These exist for wiring a manual "Backup
        // now" / "Restore now" button onto a specific page, or for
        // debugging from the console.
        //
        // table (optional) restricts the call to one table/view/trigger
        // by name; omitted, the whole database is exported/restored.
        db.exportBackup = async function(table) {
            let url = '/api/db/export?db=' + encodeURIComponent(name);
            if (table) url += '&table=' + encodeURIComponent(table);
            const res = await fetch(url, { method: 'POST' });
            const data = await res.json();
            if (data.status !== 'success') throw new Error(data.message || 'export failed');
            return data.changed_files || [];
        };
        db.restoreBackup = async function(table) {
            let url = '/api/db/restore?db=' + encodeURIComponent(name);
            if (table) url += '&table=' + encodeURIComponent(table);
            const res = await fetch(url, { method: 'POST' });
            const data = await res.json();
            if (data.status !== 'success') throw new Error(data.message || 'restore failed');
            return true;
        };
        return db;
    };

    // Drop-in stand-in for the deprecated WebSQL entry point, so old note
    // scripts keep working with the original call shape. version /
    // displayName / size are accepted and ignored.
    window.openDatabase = function(name, version, displayName, size, creationCallback) {
        const db = window.omnGoOpenDatabase(name);
        if (typeof creationCallback === 'function') {
            try { creationCallback(db); } catch (e) { console.error(e); }
        }
        return db;
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

    window.login = function() { printDebug('login'); };
    window.createNewPage = function() { printDebug('createNewPage'); };
    window.submitQuickNote = function() { printDebug('submitQuickNote'); };
    window.submitBookmark = function() { printDebug('submitBookmark'); };
    window.checkSession = function() { printDebug('checkSession'); };
    window.omnGoOpenDatabase = function() { printDebug('omnGoOpenDatabase'); };
    window.openDatabase = function() { printDebug('openDatabase'); };
}
