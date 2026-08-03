// --- OMN-Go Server Extensions ---
// These modules call the backend API and disable themselves on file: pages.

if (window.location.protocol !== 'file:') {

    // /api/logs SSE subscribers. The caller must call the returned
    // unsubscribe function, or a finished operation keeps reacting.
    const logSubscribers = [];
    window.omnGoOnServerLog = function(fn) {
        logSubscribers.push(fn);
        return function() {
            const i = logSubscribers.indexOf(fn);
            if (i !== -1) logSubscribers.splice(i, 1);
        };
    };

    // Maps a backend "[sync] ..." log line to a stage label. First match
    // wins. An unmatched line keeps the stage and updates only the detail.
    const SYNC_STAGES = [
        ['Opening repo',            'Opening repository…'],
        ['Repo not found',          'Initializing repository…'],
        ['Repo initialized',        'Repository ready'],
        ['Repo opened',             'Repository ready'],
        ['Created .gitignore',      'Preparing repository…'],
        ['Updated .gitignore',      'Preparing repository…'],
        ['cannot update .gitignore','Preparing repository…'],
        ['Remote',                  'Configuring remote…'],
        ['Removing remote',         'Configuring remote…'],
        ['Adding remote',           'Configuring remote…'],
        ['Active server slot',      'Configuring remote…'],
        ['SSH',                     'Authenticating…'],
        ['Error: No SSH key',       'No SSH key configured'],
        ['Checking worktree',       'Checking local changes…'],
        ['Nothing to commit',       'Nothing to commit'],
        ['No real changes',         'Nothing to commit'],
        ['Staging',                 'Staging changes…'],
        ['Staged',                  'Staging changes…'],
        ['Ignoring',                'Staging changes…'],
        ['Committing',              'Committing…'],
        ['Committed',               'Committing…'],
        ['Commit aborted',          'Nothing to commit'],
        ['Pull: fetching',          'Fetching from remote…'],
        ['Force pull: fetching',    'Fetching from remote…'],
        ['Pull: already up to date','Already up to date'],
        ['Pull: local tracked',     'Local changes block fast-forward'],
        ['Pull: fast-forward not',  'Histories have diverged'],
        ['Pull: fast-forward comp', 'Pull complete'],
        ['Pull: 3-way conflict',    'Conflicts need resolving'],
        ['Force pull complete',     'Pull complete'],
        ['pull:',                   'Applying changes…'],
        ['force pull:',             'Applying changes…'],
        ['pull_abort',              'Restoring local state…'],
        ['Pushing to',              'Uploading to remote…'],
        ['push:',                   'Finishing upload…'],
        // go-git sideband text relayed from the remote.
        ['remote:',                 'Transferring…'],
        ['Counting objects',        'Transferring…'],
        ['Compressing objects',     'Transferring…'],
        ['Receiving objects',       'Transferring…'],
        ['Resolving deltas',        'Transferring…'],
        ['Writing objects',         'Transferring…']
    ];

    // Feeds one backend log line into the progress overlay. A non-"[sync]"
    // line is ignored so an unrelated log cannot hijack the display.
    function applySyncLogLine(msg) {
        const at = msg.indexOf('[sync]');
        if (at === -1) return;
        const line = msg.slice(at + '[sync]'.length).trim();
        if (!line) return;
        for (const [prefix, label] of SYNC_STAGES) {
            if (line.indexOf(prefix) === 0) {
                window.OMNProgress.stage(label);
                break;
            }
        }
        window.OMNProgress.detail(line);
    }

    const SYNC_TITLES = {
        pull: 'Download', pull_ff: 'Download', download: 'Download',
        pull_force: 'Force download', pull_mark: 'Mark conflicts',
        pull_abort: 'Abort pull',
        push: 'Upload', upload: 'Upload', push_force: 'Force upload'
    };

    const Logger = (function() {
        // runSync is the only caller of the /api/sync endpoint. It POSTs
        // action, force and message in the request body, where the backend
        // reads them.
        window.runSync = async function(action, opts) {
            opts = opts || {};
            const fd = new URLSearchParams();
            fd.append('action', action);
            if (opts.force) fd.append('force', 'true');
            if (opts.message) fd.append('message', opts.message);

            // The overlay is fed by the backend "[sync]" log lines. Hide it
            // before any alert() or modal, which would block on top of it.
            let data, netErr = null;
            window.OMNProgress.show(SYNC_TITLES[action] || 'Sync');
            window.OMNProgress.stage('Contacting server…');
            const unsubscribe = window.omnGoOnServerLog(applySyncLogLine);
            try {
                const res = await fetch('/api/sync', { method: 'POST', body: fd });
                data = await res.json();
            } catch (e) {
                netErr = e;
            } finally {
                unsubscribe();
                window.OMNProgress.hide();
            }
            if (netErr) {
                alert('Sync error: ' + netErr);
                return null;
            }

            const modal = document.getElementById('conflict-modal');
            switch (data.status) {
                case 'success':
                    if (modal) modal.classList.add('hidden');
                    return data;
                case 'conflict':
                    if (modal) {
                        populateConflictFiles(data.files);
                        modal.classList.remove('hidden');
                    } else {
                        const list = (data.files && data.files.length)
                            ? '\n\nFiles in contention:\n' + data.files.join('\n') : '';
                        const choice = confirm('Conflict!' + list + '\n\nOK to Force Pull (Keep Untracked), Cancel to Mark Files.');
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

        // populateConflictFiles lists the files the backend reports in
        // conflict. An empty list means the histories diverged with no
        // per-file overlap. textContent stops a filename injecting markup.
        function populateConflictFiles(files) {
            const box = document.getElementById('conflict-files');
            const list = document.getElementById('conflict-file-list');
            if (!box || !list) return;
            list.textContent = '';
            const arr = Array.isArray(files) ? files : [];
            if (arr.length === 0) {
                const li = document.createElement('li');
                li.className = 'conflict-files-none';
                li.textContent = 'No individual file conflicts — histories diverged; Force Pull is recommended.';
                list.appendChild(li);
            } else {
                arr.forEach(function(name) {
                    const li = document.createElement('li');
                    li.textContent = name;
                    list.appendChild(li);
                });
            }
            box.classList.remove('hidden');
        }

        // performSync handles the three buttons on the conflict modal.
        window.performSync = async function(action) {
            const modal = document.getElementById('conflict-modal');
            if (action === 'abort') {
                // A plain pull does not change local state before it reports
                // a conflict, so abort is a UI cancel with nothing to undo.
                if (modal) modal.classList.add('hidden');
                return;
            }
            if (modal) modal.classList.add('hidden');

            const data = await window.runSync(action);
            if (data && data.status === 'success') {
                // pull_force and pull_mark change the files under this page.
                location.reload();
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

    window.saveConfig = async function() {
        const form = document.getElementById('configForm');
        if (!form) { alert('Config form not found'); return; }
        const fd = new FormData(form);
        try {
            const res = await fetch('/api/config', { method: 'POST', body: fd });
            if (res.ok) {
                // Clear the dirty flag before either reload path, or the
                // save re-triggers its own "leave site?" prompt.
                if (window.configMarkClean) window.configMarkClean();
                const body = await res.text();
                if (body === 'RestartRequired') {
                    // LAN sharing changed. The listen socket is bound once
                    // at startup, so the backend must restart to rebind.
                    alert('LAN sharing changed - the application will now restart to apply it.\n\nDesktop: this page reloads automatically in a few seconds.\nAndroid: the app will close; reopen it manually.');
                    try { await fetch('/api/restart', { method: 'POST' }); } catch (e) { /* the connection drops as the backend exits */ }
                    // Desktop: the replacement process is up in 1-3s, so
                    // reload to reconnect. On Android the process exits
                    // during the restart and a response may never arrive.
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
        // The preview walks the whole worktree diff, the slow half of an
        // upload.
        let res, preview, err = null;
        window.OMNProgress.show('Upload');
        window.OMNProgress.stage('Collecting pending changes…');
        const unsubscribe = window.omnGoOnServerLog(applySyncLogLine);
        try {
            res = await fetch('/api/sync/preview?action=upload');
            if (res.ok) preview = await res.json();
        } catch (e) {
            err = e;
        } finally {
            unsubscribe();
            window.OMNProgress.hide();
        }
        try {
            if (err) {
                alert('Error: ' + err);
                return;
            }
            if (!res.ok) {
                alert('Failed to get pending changes');
                return;
            }
            const files = (preview && preview.files) || [];

            if (files.length === 0) {
                // A clean worktree can still hold commits the remote has
                // never seen. With nothing to commit, no commit message is
                // asked for and the upload goes straight to the push.
                if (preview.unpushed) {
                    const data = await window.runSync('upload', { force });
                    if (data && data.status === 'success') {
                        if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                            window.location.reload();
                        }
                    }
                    return;
                }
                // Name the remote: several git server slots may be set up.
                var where = preview.remote ? ' on ' + preview.remote : '';
                if (preview.remote_error) {
                    alert('Nothing to commit.\n\nCould not reach the remote' + where +
                          ' to check for unpushed commits:\n' + preview.remote_error);
                } else {
                    alert('Nothing to commit, and nothing to push' + where + '.');
                }
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

    // Editing a note opens the editor page (?edit=true, omn-go-editor.js).

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

    function toCamelCase(str) {
        let words = str.split(/[-_\s]+/);
        return words.map(w => w ? w.charAt(0).toUpperCase() + w.slice(1) : '').join('');
    }

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
            // The backend resolves fileName relative to the current page's
            // directory, so a bare name becomes a sibling of src.
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

    // --- Bookmark capture UI ---
    window.handleShare = function(text, subject) {
        text = text || '';
        subject = subject || '';

        const urlMatch = text.match(/(https?:\/\/[^\s]+)/) || subject.match(/(https?:\/\/[^\s]+)/);

        if (urlMatch) {
            const url = urlMatch[0];
            document.getElementById('bmUrl').value = url;

            let title = subject;
            if (!title || title.includes(url)) {
                title = text.replace(url, '').trim();
            }
            if (!title) title = "Shared Link";

            document.getElementById('bmTitle').value = title;
            window.showBookmarkPanel();
            document.getElementById('quickPanel').classList.add('hidden');
        } else {
            let content = '';
            if (subject) content += subject + "\n\n";
            if (text) content += text;

            document.getElementById('quickText').value = content.trim();
            document.getElementById('quickPanel').classList.remove('hidden');
            document.getElementById('bmPanel').classList.add('hidden');
        }
    };

    // Called from Android (MainActivity.insertCapturedText). Always targets
    // the Quick Note panel, never the bookmark panel, even for a URL.
    // Returns false when the page has no panel.
    window.omnGoInsertCapture = function(text, label) {
        var q = document.getElementById('quickText');
        var p = document.getElementById('quickPanel');
        if (!q || !p) { return false; }
        var content = '';
        if (label) { content += label + "\n\n"; }
        if (text) { content += text; }
        q.value = content.trim();
        p.classList.remove('hidden');
        var bm = document.getElementById('bmPanel');
        if (bm) { bm.classList.add('hidden'); }
        return true;
    };

    // Registered on DOMContentLoaded: this file runs in <head>, where
    // document.body is still null and touching it would throw.
    document.addEventListener('DOMContentLoaded', () => {
        document.body.addEventListener('dragover', e => {
            if (!e.target.closest('#editor')) e.preventDefault();
        });
        document.body.addEventListener('drop', e => {
            if (e.target.closest('#editor')) return;
            const url = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
            if (url && (url.startsWith('http://') || url.startsWith('https://'))) {
                e.preventDefault();
                document.getElementById('bmUrl').value = url;
                document.getElementById('bmTitle').value = '';
                const html = e.dataTransfer.getData('text/html');
                if (html) {
                    const match = html.match(/<a[^>]*>(.*?)<\/a>/i);
                    if (match && match[1]) {
                        document.getElementById('bmTitle').value = match[1].replace(/<[^>]+>/g, '').trim();
                    }
                }
                window.showBookmarkPanel();
            }
        });
    });

    // --- Bookmark "Tags" autocomplete ---
    // Only the fragment after the last comma is matched, once it reaches
    // the #bmTags minChars attribute (default 2). A failed fetch means "no
    // suggestions", so the field stays a plain comma-separated input.
    (function () {
        var tagsCache = null;    // array once loaded, even if empty
        var tagsPromise = null;  // in-flight fetch, if any
        var wired = false;       // listeners attach only once

        // Fetches the tag list at most once per page. Safe to call on
        // every modal open.
        function ensureTagsLoaded() {
            if (tagsCache) return Promise.resolve(tagsCache);
            if (tagsPromise) return tagsPromise;
            tagsPromise = fetch('/json/bookmarker-tags.json', { cache: 'no-store' })
                .then(function (res) { return res.ok ? res.json() : []; })
                .then(function (data) { return (tagsCache = Array.isArray(data) ? data : []); })
                .catch(function () { return (tagsCache = []); });
            return tagsPromise;
        }

        // "foo, bar, ba" -> { done: ["foo", "bar"], fragment: "ba" }
        function splitTags(value) {
            var parts = value.split(',');
            var fragment = parts.pop();
            var done = parts.map(function (s) { return s.trim(); }).filter(Boolean);
            return { done: done, fragment: fragment.replace(/^\s+/, '') };
        }

        // The wired guard makes repeat calls no-ops, so reopening the
        // panel never double-attaches listeners.
        function wireBookmarkTagAutocomplete() {
            if (wired) return;
            var input = document.getElementById('bmTags');
            var list = document.getElementById('bmTagsSuggestions');
            if (!input || !list) return;
            wired = true;

            var minChars = parseInt(input.getAttribute('minChars'), 10);
            if (!minChars || minChars < 1) minChars = 2;

            var activeIndex = -1;

            function hide() {
                list.innerHTML = '';
                list.classList.add('hidden');
                activeIndex = -1;
            }

            function setActive(idx) {
                var items = list.querySelectorAll('.tag-suggestion-item');
                items.forEach(function (it, i) { it.classList.toggle('active', i === idx); });
                activeIndex = idx;
            }

            function pick(tag) {
                var split = splitTags(input.value);
                var used = split.done.concat([tag]);
                // The trailing ", " primes the field for the next tag.
                input.value = used.join(', ') + ', ';
                hide();
                input.focus();
                var end = input.value.length;
                input.setSelectionRange(end, end);
            }

            function render(matches) {
                list.innerHTML = '';
                if (!matches.length) { hide(); return; }
                matches.forEach(function (tag) {
                    var li = document.createElement('li');
                    li.textContent = tag;
                    li.className = 'tag-suggestion-item';
                    // mousedown fires before #bmTags blurs, so the pick
                    // survives the input losing focus.
                    li.addEventListener('mousedown', function (e) {
                        e.preventDefault();
                        pick(tag);
                    });
                    list.appendChild(li);
                });
                activeIndex = -1;
                list.classList.remove('hidden');
            }

            function update() {
                var split = splitTags(input.value);
                var fragment = split.fragment;
                if (fragment.length < minChars) { hide(); return; }
                ensureTagsLoaded().then(function (tags) {
                    // Drop a stale response: the field may have moved on.
                    if (splitTags(input.value).fragment !== fragment) return;
                    var lower = fragment.toLowerCase();
                    var used = split.done.map(function (t) { return t.toLowerCase(); });
                    var matches = tags.filter(function (t) {
                        return typeof t === 'string' &&
                            t.toLowerCase().indexOf(lower) === 0 &&
                            used.indexOf(t.toLowerCase()) === -1;
                    }).slice(0, 20);
                    render(matches);
                });
            }

            input.addEventListener('input', update);
            input.addEventListener('focus', update);

            input.addEventListener('keydown', function (e) {
                var items = list.querySelectorAll('.tag-suggestion-item');
                if (list.classList.contains('hidden') || !items.length) return;
                if (e.key === 'ArrowDown') {
                    e.preventDefault();
                    setActive((activeIndex + 1) % items.length);
                } else if (e.key === 'ArrowUp') {
                    e.preventDefault();
                    setActive((activeIndex - 1 + items.length) % items.length);
                } else if (e.key === 'Enter' && activeIndex >= 0) {
                    e.preventDefault();
                    pick(items[activeIndex].textContent);
                } else if (e.key === 'Escape') {
                    hide();
                }
            });

            document.addEventListener('click', function (e) {
                if (e.target !== input && !list.contains(e.target)) hide();
            });
        }

        // Always shows #bmPanel. Never toggles it.
        window.showBookmarkPanel = function () {
            var panel = document.getElementById('bmPanel');
            if (!panel) return;
            panel.classList.remove('hidden');
            wireBookmarkTagAutocomplete();
            ensureTagsLoaded();
        };

        // Toggles #bmPanel. Prepares the autocomplete only when opening.
        window.toggleBookmarkPanel = function () {
            var panel = document.getElementById('bmPanel');
            if (!panel) return;
            var opening = panel.classList.contains('hidden');
            panel.classList.toggle('hidden');
            if (opening) {
                wireBookmarkTagAutocomplete();
                ensureTagsLoaded();
            }
        };
    })();

    // --- Server-backed SQLite ---
    // Data lives in <storage>/db/<name>.sqlite, so every device sees the
    // same data. Requires the admin role, which a local connection has.
    // db.batch([...]) is atomic.
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

        // Backend result -> WebSQL-shaped result set.
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
            // Statements queued synchronously in the callback run as one
            // atomic transaction. Statements queued from success callbacks
            // run as a separate batch, which real WebSQL kept in the same
            // transaction.
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
                        // okCb calls may have queued more statements.
                    }
                    if (doneCb) doneCb();
                })();
            }
        };
        db.readTransaction = db.transaction;

        // Backups are whole-database and managed from the /db_backups page.
        return db;
    };

    // Stand-in for the WebSQL entry point. version, displayName and size
    // are accepted and ignored.
    window.openDatabase = function(name, version, displayName, size, creationCallback) {
        const db = window.omnGoOpenDatabase(name);
        if (typeof creationCallback === 'function') {
            try { creationCallback(db); } catch (e) { console.error(e); }
        }
        return db;
    };

    function checkRole() {
        if(document.cookie.includes('session_role=guest')) {
            document.querySelectorAll('.admin-only').forEach(el => {
                if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';
            });
        }
    }

    window.checkSession = async function() {
        // The backend injects #loginOverlay. An exported page has no login
        // gate, so leave the visible content alone.
        const overlay = document.getElementById('loginOverlay');
        const main = document.getElementById('mainUI');
        if (!overlay || !main) return;
        if (document.cookie.includes('session_role=')) {
            overlay.style.display = 'none';
            main.style.display = 'flex';
            checkRole();
        } else {
            const test = await fetch('/api/config');
            if (test.status === 401) {
                overlay.style.display = 'flex';
                main.style.display = 'none';
            } else {
                overlay.style.display = 'none';
                main.style.display = 'flex';
            }
        }
    };

    // Bridges backend logs to the frontend. Every log.Printf reaches this
    // stream, which is what feeds the sync progress overlay.
    // Each subscriber has a 10-slot buffered channel that DROPS messages
    // rather than blocking, so never drive state that must see every event.
    document.addEventListener('DOMContentLoaded', () => {
        try {
            const logSource = new EventSource('/api/logs');
	    // stream is released before the document is cached
	    window.addEventListener('pagehide', () => logSource.close());
            logSource.onmessage = function(event) {
                let msg = event.data.trim();
                if(msg) {
                    console.log("[GO] " + msg);
                    for (const fn of logSubscribers.slice()) {
                        try { fn(msg); } catch (e) { /* a bad subscriber must not kill the stream */ }
                    }
                }
            };
        } catch(e) {
            console.error("Log source error:", e);
        }
    });

    // --- Search overlay ---
    // Two scopes: page (the open note, always available) and all (indexed
    // notes, offered only when OMN_SEARCH_GLOBAL says the backend can answer).
    // Response text goes in with textContent, never innerHTML.
    (function () {
        var DEBOUNCE_MS = 150;   // skips intermediate keystrokes
        var MIN_QUERY = 2;       // 1 rune matches nearly everything
        var MAX_SNIPPETS = 10;   // the API's own cap

        var overlay = null, input = null, list = null, statusEl = null, scopeEl = null;
        var seeAllEl = null;   // lives in the scope row - see renderScope
        var rows = [];
        var active = -1;
        var timer = null;
        var inflight = null;
        var lastTerms = [];
        // "" until the user picks: the FIRST query deliberately sends no scope
        // and adopts whatever the server used (it knows the configured
        // default, and whether global search can answer at all). After that
        // the choice is explicit and sticky for the session.
        var scope = "";
        var scopeShown = "";

        // The page to search. index.html defines currentNote for every
        // rendered note; without it there is nothing to scope to.
        function pageName() {
            return (typeof currentNote !== 'undefined' && currentNote) ? currentNote : '';
        }

        function build() {
            if (overlay || !document.body) return overlay;

            overlay = document.createElement('div');
            overlay.className = 'omn-search-overlay';
            overlay.hidden = true;
            // Static markup only - see the note at the top of this module.
            overlay.innerHTML =
                '<div class="omn-search-card" role="dialog" aria-label="Search">' +
                  '<div class="omn-search-head">' +
                    '<i class="material-icons omn-search-icon">search</i>' +
                    // Deliberately BARE: no spellcheck, no autocorrect, no
                    // autocapitalize, no autocomplete, no inputmode. Every one
                    // of those is a hint the Android keyboard reads when it
                    // attaches, and several of them (spellcheck="false" and
                    // autocorrect="off" certainly, autocomplete="off" in some
                    // WebView builds) fold into the NO_SUGGESTIONS flag, which
                    // switches off the COMPOSING region - the mechanism every
                    // non-Latin layout uses to enter text at all.
                    //
                    // None of them buys anything here. The field is not in a
                    // <form> and has no name, so autofill never engages;
                    // matching is case-folded, so auto-capitalisation is
                    // harmless; and a red squiggle under a query is cosmetic.
                    // A search box has no reason to describe itself as
                    // anything other than a plain text field.
                    '<input type="text" class="omn-search-input" ' +
                          'placeholder="Search this page">' +
                    '<button type="button" class="omn-search-close" aria-label="Close">' +
                      '<i class="material-icons icon-sm">close</i>' +
                    '</button>' +
                  '</div>' +
                  '<div class="omn-search-scope"></div>' +
                  '<ul class="omn-search-results"></ul>' +
                  '<div class="omn-search-status"></div>' +
                '</div>';
            document.body.appendChild(overlay);

            input = overlay.querySelector('.omn-search-input');
            list = overlay.querySelector('.omn-search-results');
            statusEl = overlay.querySelector('.omn-search-status');
            scopeEl = overlay.querySelector('.omn-search-scope');

            overlay.querySelector('.omn-search-close').addEventListener('click', close);
            // A click on the backdrop closes; a click inside the card must not.
            overlay.addEventListener('click', function (e) {
                if (e.target === overlay) close();
            });
            input.addEventListener('input', schedule);
            input.addEventListener('keydown', onInputKey);

            renderScope();
            return overlay;
        }

        // Whether the server can answer a scope=all query: the setting is on
        // AND there is an index to ask. Injected per request (see
        // injectRuntimeVars), so a page cached before the setting changed
        // still gets the current answer.
        function globalAvailable() {
            return typeof OMN_SEARCH_GLOBAL !== 'undefined' && OMN_SEARCH_GLOBAL;
        }

        // The scope row: one control, and only when there is a choice to make.
        // With global search off it stays what it was before - a statement of
        // where you are searching, not a switch that leads nowhere.
        function renderScope() {
            if (!scopeEl) return;
            scopeEl.textContent = '';
            var showing = scopeShown || (globalAvailable() ? 'all' : 'page');

            function chip(value, label) {
                var el = document.createElement('span');
                el.className = 'omn-search-chip' + (showing === value ? ' is-active' : '');
                el.textContent = label;
                if (globalAvailable()) {
                    el.setAttribute('role', 'button');
                    el.tabIndex = 0;
                    el.addEventListener('click', function () { setScope(value); });
                }
                scopeEl.appendChild(el);
            }

            if (globalAvailable()) chip('all', 'All notes');
            chip('page', 'This page');
            if (input) {
                input.placeholder = showing === 'all' ? 'Search all notes' : 'Search this page';
            }

            // Which page "this page" means. Shown only in page scope, where it
            // is the thing being searched.
            var name = pageName();
            if (showing === 'page' && name) {
                var where = document.createElement('span');
                where.className = 'omn-search-where';
                where.textContent = name;
                scopeEl.appendChild(where);
            }

            // "See all results" belongs up here with the scope, not at the foot
            // of the list: it is a statement about WHERE to search rather than
            // one of the answers, and at the bottom it moved with every query
            // and was only reachable after scrolling past everything above it.
            //
            // A real <button> rather than a chip, so Tab reaches it and Enter
            // and Space work without a keydown handler of its own - which
            // matters more now that it is no longer in the arrow-key list.
            seeAllEl = null;
            if (globalAvailable()) {
                seeAllEl = document.createElement('button');
                seeAllEl.type = 'button';
                seeAllEl.className = 'omn-search-seeall';
                seeAllEl.textContent = 'See all results \u2192';
                seeAllEl.title = 'Open the full results page';
                seeAllEl.addEventListener('click', openResultsPage);
                scopeEl.appendChild(seeAllEl);
            }
            updateSeeAll();
        }

        // The results page is global-only - serveSearchPage answers 404 with
        // global search off - and an empty query would land on a bare form. So
        // the control is present only when following it would show something.
        function updateSeeAll() {
            if (!seeAllEl) return;
            var showing = scopeShown || (globalAvailable() ? 'all' : 'page');
            seeAllEl.hidden = !(showing === 'all' && input &&
                                input.value.trim().length >= MIN_QUERY);
        }

        function openResultsPage() {
            var q = input.value.trim();
            if (!q) return;
            close();
            window.location.href = '/OMNGoSearch.html?q=' + encodeURIComponent(q);
        }

        function setScope(next) {
            if (scope === next && scopeShown === next) return;
            scope = next;
            scopeShown = next;
            renderScope();
            clearTimeout(timer);
            run();
        }

        function open() {
            if (!document.body) return;
            build();
            overlay.hidden = false;
            focusInput();
            if (input.value.trim().length >= MIN_QUERY) {
                run();
            } else {
                setStatus('Type at least ' + MIN_QUERY + ' characters');
            }
        }

        // focusInput hands the field to the keyboard twice, and the second time
        // is the one that matters.
        //
        // The soft keyboard attaches to whatever element has focus and reads
        // its configuration at that instant. This overlay goes from
        // display:none to display:flex and takes focus in the SAME tick, so on
        // Android the IME can attach to an element the browser has not laid out
        // yet. When that happens it comes up without a COMPOSING region - which
        // is how every non-Latin layout enters text - and the field then accepts
        // Latin typing while silently refusing Cyrillic. That is why it reads as
        // "the search box is broken" rather than "the keyboard attached wrong",
        // and why it is intermittent: it depends on what the browser had already
        // laid out.
        //
        // blur() before the second focus() is what makes it a re-attach rather
        // than a no-op - focus() on the already-focused element does nothing,
        // and doing nothing is exactly the state that needs clearing. It is the
        // same reset a user stumbles on by opening the quick note panel and
        // closing it again.
        //
        // The first, synchronous focus stays so that a character typed straight
        // after Ctrl-K on a desktop is not dropped in the frame between.
        function focusInput() {
            input.focus();
            if (input.value) input.select();

            var reattach = function () {
                // Only if nothing else has taken over in the meantime: the user
                // may have closed the panel or tapped elsewhere already.
                if (!isOpen() || document.activeElement !== input) return;
                input.blur();
                input.focus();
                if (input.value) input.select();
            };

            if (window.requestAnimationFrame) {
                // Two frames: one to lay the overlay out, one to be sure it has
                // been through a paint before the keyboard looks at it.
                window.requestAnimationFrame(function () {
                    window.requestAnimationFrame(reattach);
                });
            } else {
                setTimeout(reattach, 32);
            }
        }

        function close() {
            clearTimeout(timer);
            if (inflight) {
                inflight.abort();
                inflight = null;
            }
            if (overlay) overlay.hidden = true;
        }

        function isOpen() {
            return !!overlay && !overlay.hidden;
        }

        function schedule() {
            // Updated here rather than in run(), so the control appears as soon
            // as the query is long enough instead of one debounce later.
            updateSeeAll();
            clearTimeout(timer);
            timer = setTimeout(run, DEBOUNCE_MS);
        }

        function setStatus(text) {
            if (statusEl) statusEl.textContent = text || '';
        }

        function clearRows() {
            rows = [];
            active = -1;
            if (list) list.textContent = '';
        }

        function run() {
            var q = input.value.trim();
            if (q.length < MIN_QUERY) {
                clearRows();
                setStatus(q ? 'Type at least ' + MIN_QUERY + ' characters' : '');
                return;
            }

            // Cancel the previous request rather than racing it: on a fast
            // typist the older answer can otherwise arrive last and overwrite
            // the newer one.
            if (inflight) inflight.abort();
            var ctrl = (typeof AbortController !== 'undefined') ? new AbortController() : null;
            inflight = ctrl;

            var url = '/api/search?snippets=' + MAX_SNIPPETS +
                '&q=' + encodeURIComponent(q) +
                '&on=' + encodeURIComponent(pageName());
            if (scope) url += '&scope=' + encodeURIComponent(scope);

            var opts = { cache: 'no-store' };
            if (ctrl) opts.signal = ctrl.signal;

            fetch(url, opts)
                .then(function (r) { return r.json(); })
                .then(function (data) {
                    if (ctrl && inflight !== ctrl) return; // superseded
                    inflight = null;
                    if (data && data.status && data.status !== 'ok' && data.error) {
                        // The server refused this scope (global search off, or
                        // its index not ready). Say why and drop back to the
                        // scope that always works, rather than showing an
                        // empty list that would read as "nothing matched".
                        clearRows();
                        setStatus(data.error);
                        if (scope !== 'page') {
                            scope = 'page';
                            scopeShown = 'page';
                            renderScope();
                        }
                        return;
                    }
                    render(q, data);
                })
                .catch(function (err) {
                    if (err && err.name === 'AbortError') return;
                    if (ctrl && inflight !== ctrl) return;
                    inflight = null;
                    clearRows();
                    setStatus('Search failed');
                    console.error('search: ' + err);
                });
        }

        function render(query, data) {
            clearRows();
            // The server's own list, not a naive split: it has already dropped
            // the field prefixes ("tag:hydro" is a search for "hydro", and
            // marking the literal "tag:hydro" would find nothing) and applied
            // the same minimum length the highlighter uses. Falling back to a
            // split keeps this working against an older server.
            lastTerms = (data && data.highlight && data.highlight.length)
                ? data.highlight
                : query.split(/\s+/).filter(function (t) { return t.length > 0; });

            // The server reports which scope it actually used. Adopting it
            // means the first query needs no guess about the configured
            // default, and the chips can never disagree with the results
            // underneath them.
            if (data && data.scope && data.scope !== scopeShown) {
                scopeShown = data.scope;
                renderScope();
            }

            var results = (data && data.results) ? data.results : [];
            if (!results.length) {
                setStatus(scopeShown === 'all' ? 'No matches in your notes' : 'No matches on this page');
                return;
            }

            if (scopeShown === 'all') {
                renderGlobal(data, results);
            } else {
                renderPage(results[0]);
            }
        }

        // Global scope: several documents, each with its own snippets. A row
        // opens the document; the heading above it says which one.
        function renderGlobal(data, results) {
            results.forEach(function (r) {
                var head = document.createElement('li');
                head.className = 'omn-search-doc';

                var title = document.createElement('span');
                title.className = 'omn-search-doc-title';
                title.textContent = r.title || r.name;
                head.appendChild(title);

                var path = document.createElement('span');
                path.className = 'omn-search-doc-path';
                path.textContent = r.name;
                head.appendChild(path);

                head.addEventListener('click', function () { openResult(r); });
                list.appendChild(head);

                (r.matches || []).forEach(function (m) {
                    list.appendChild(buildSnippetRow(m, function () { openResult(r); }));
                });
                if (r.truncated) {
                    var note = document.createElement('li');
                    note.className = 'omn-search-note';
                    note.textContent = 'only the first 500 KiB of this file was searched';
                    list.appendChild(note);
                }
            });

            var n = data.total || results.length;
            var note = n === 1 ? '1 result' : n + ' results';
            if (data.truncated && n > results.length) note += ' (showing ' + results.length + ')';
            setStatus(note + ' \u00b7 \u2191\u2193 to move \u00b7 \u21b5 to open');
            setActive(0);
        }

        // A result in global scope is a different document, so following it is
        // a navigation - unlike page scope, where the answer is already on
        // screen and the useful move is to highlight it in place.
        function openResult(r) {
            close();
            if (r && r.url) window.location.href = withHighlight(r.url);
        }

        // withHighlight hangs the query terms off a URL as ?hl=, so the page
        // being opened marks them and scrolls to the first on arrival. Same
        // parameter the results page puts on its links (highlightURL in
        // search.go), so a result behaves identically whichever list it came
        // from. The receiving page strips them from the address bar once
        // applied - see omn-go-core.js.
        function withHighlight(url) {
            if (!lastTerms.length) return url;
            // The fragment stays last. A sectioned result arrives here as
            // "/Bookmarks.html#2026-06-15-200000", and appending blindly gives
            // "#2026-06-15-200000?hl=cats" - one fragment that names no
            // element, and no query string at all, so the page neither scrolls
            // nor highlights. This mirrors highlightURL in search.go; the two
            // build the same URL from opposite ends of the app and have to
            // agree.
            var frag = '';
            var hash = url.indexOf('#');
            if (hash >= 0) {
                frag = url.slice(hash);
                url = url.slice(0, hash);
            }
            var sep = url.indexOf('?') === -1 ? '?' : '&';
            for (var i = 0; i < lastTerms.length; i++) {
                url += sep + 'hl=' + encodeURIComponent(lastTerms[i]);
                sep = '&';
            }
            return url + frag;
        }

        function renderPage(result) {
            if (!result || !result.matches || !result.matches.length) {
                // A note can match on its title or a tag and have no matching
                // LINE - say so, rather than showing an empty list that reads
                // as "nothing found".
                setStatus('Matches this page’s title or tags, but no line in the text');
                return;
            }

            result.matches.forEach(function (m, i) {
                var idx = i;
                list.appendChild(buildSnippetRow(m, function () { choose(idx, m); }));
            });

            var n = result.matches.length;
            var note = n === 1 ? '1 matching line' : n + ' matching lines';
            if (result.truncated) note += ' · only the first 500 KiB was searched';
            setStatus(note + ' · ↑↓ to move · ↵ to highlight in the page');
            setActive(0);
        }

        // buildSnippetRow renders one match. Shared by both scopes so a line
        // looks the same wherever it was found; only what following it DOES
        // differs, which is the caller's business.
        function buildSnippetRow(m, onChoose) {
            var li = document.createElement('li');
            li.className = 'omn-search-row';
            li.setAttribute('role', 'option');

            // A section label replaces the bare line number when there is one.
            // "27 Jul, 07:23" or "OMN-Go on GitHub" locates a hit inside a
            // 3 000-line QuickNotes in a way "line 1842" does not; the line
            // number is still what the API reports, and still what the editor
            // needs, but it is not what a reader is looking for.
            var num = document.createElement('span');
            num.className = 'omn-search-line';
            if (m.section && m.section.label) {
                num.classList.add('omn-search-section');
                num.textContent = '\u203a ' + m.section.label;
                num.title = 'line ' + m.line;
            } else {
                num.textContent = m.line;
            }
            li.appendChild(num);

            if (m.context) {
                // A hit inside a script or a fenced block is a different kind
                // of answer from one in prose. Marked, never ranked down: code
                // in notes is a normal thing to search for.
                var ctx = document.createElement('span');
                ctx.className = 'omn-search-ctx';
                ctx.textContent = '‹/›';
                ctx.title = m.context === 'script' ? 'inside a <script> block' : 'inside a code block';
                li.appendChild(ctx);
            }

            var text = document.createElement('span');
            text.className = 'omn-search-text';
            renderHighlighted(text, m.text || '', m.spans || []);
            li.appendChild(text);

            var idx = rows.length;
            li.addEventListener('click', onChoose);
            li.addEventListener('mousemove', function () { setActive(idx); });
            rows.push(li);
            li._omnChoose = onChoose;
            return li;
        }

        // renderHighlighted writes text into node, wrapping each span in a
        // <mark>. Spans are RUNE offsets (the Go side works in runes so that
        // Cyrillic is not cut in half), so the text is split with Array.from,
        // which iterates code points - text.substring would use UTF-16 units
        // and drift on anything outside the BMP.
        function renderHighlighted(node, text, spans) {
            var runes = Array.from(text);
            var at = 0;
            spans.forEach(function (s) {
                var start = s[0], len = s[1];
                if (typeof start !== 'number' || typeof len !== 'number') return;
                if (start < at || start + len > runes.length) return;
                if (start > at) {
                    node.appendChild(document.createTextNode(runes.slice(at, start).join('')));
                }
                var mark = document.createElement('mark');
                mark.className = 'omn-search-hit';
                mark.textContent = runes.slice(start, start + len).join('');
                node.appendChild(mark);
                at = start + len;
            });
            if (at < runes.length) {
                node.appendChild(document.createTextNode(runes.slice(at).join('')));
            }
        }

        function setActive(i) {
            if (!rows.length) return;
            if (i < 0) i = 0;
            if (i >= rows.length) i = rows.length - 1;
            if (active >= 0 && rows[active]) rows[active].classList.remove('is-active');
            active = i;
            rows[active].classList.add('is-active');
            if (rows[active].scrollIntoView) {
                rows[active].scrollIntoView({ block: 'nearest' });
            }
        }

        // choose closes the panel and marks the query in the page itself.
        //
        // Per-LINE jumping is deliberately not attempted here: the result's
        // line number indexes the markdown SOURCE, and the page shows compiled
        // HTML, so there is no reliable mapping between the two without the
        // machinery a later phase adds. Highlighting every occurrence and
        // scrolling to the first is honest about what it knows, and is the
        // answer to "where does this note talk about X" either way.
        function choose(i, m) {
            setActive(i);
            close();
            var first = window.omnHighlightTerms(lastTerms);

            // Go to the occurrence THIS row is about, not the first one on the
            // page. A row inside a <script> block is skipped deliberately: the
            // text is indexed but never rendered, so there is nothing on the
            // page to scroll to and looking would only find a coincidence.
            var target = null;
            if (m && m.context !== 'script' && window.omnMarkNear) {
                target = window.omnMarkNear(m.text || '');
            }

            var el = target || first;
            if (el && el.scrollIntoView) {
                el.scrollIntoView({ block: 'center' });
                if (el.classList) el.classList.add('omn-search-hit-current');
            }
        }

        function onInputKey(e) {
            switch (e.key) {
            case 'Escape':
                e.preventDefault();
                close();
                break;
            case 'ArrowDown':
                e.preventDefault();
                setActive(active + 1);
                break;
            case 'ArrowUp':
                e.preventDefault();
                setActive(active - 1);
                break;
            case 'Enter':
                e.preventDefault();
                clearTimeout(timer);
                if (rows.length) {
                    var row = rows[active < 0 ? 0 : active];
                    if (row && row._omnChoose) row._omnChoose();
                } else {
                    run();
                }
                break;
            }
        }

        // --- highlighting inside the rendered page ---
        //
        // The implementation lives in omn-go-core.js, not here: arriving at a
        // page with ?hl= needs it on every page, including one opened from
        // disk where this file's server half never runs. This module is just
        // one of its callers.

        // --- entry points ---

        window.omnSearchOpen = function () {
            if (isOpen()) {
                input.focus();
                input.select();
                return;
            }
            open();
        };

        window.omnSearchClearHighlights = window.omnClearHighlights;

        // Keyboard: Ctrl/Cmd-K anywhere, and "/" when not already typing -
        // the two conventions people arrive with. Both are desktop-only in
        // practice; the header button is the route on Android.
        document.addEventListener('keydown', function (e) {
            if (e.defaultPrevented) return;

            if ((e.key === 'k' || e.key === 'K') && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                window.omnSearchOpen();
                return;
            }
            if (e.key === 'Escape' && isOpen()) {
                e.preventDefault();
                close();
                return;
            }
            if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return;

            var t = e.target;
            var tag = (t && t.tagName) ? t.tagName.toLowerCase() : '';
            if (tag === 'input' || tag === 'textarea' || (t && t.isContentEditable)) return;

            e.preventDefault();
            window.omnSearchOpen();
        });
    })();

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
    window.handleShare = function() { printDebug('handleShare'); };
    window.showBookmarkPanel = function() { printDebug('showBookmarkPanel'); };
    window.toggleBookmarkPanel = function() { printDebug('toggleBookmarkPanel'); };
    // Search needs the server: the panel queries /api/search, which does not
    // exist on a page opened from disk. The header button is .server-only and
    // therefore already hidden here (applyOfflineUI) - these stubs cover a
    // note script or a stale keyboard shortcut calling in anyway.
    window.omnSearchOpen = function() { printDebug('omnSearchOpen'); };
    // omnSearchClearHighlights is NOT stubbed here: the highlighting lives in
    // omn-go-core.js and works offline, so the real one is already defined.
}
