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
        // fell back to a plain "pull". Both syncAction and the conflict
        // modal handler (performSync below) go through this one function
        // so the two can't drift out of sync with each other.
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

        // populateConflictFiles fills the conflict modal's file list with the
        // files the backend reported as being in contention (the ones "Mark
        // Conflicts" would inject markers into). An empty list means the
        // histories diverged with no per-file overlap (a clean local tree with
        // its own commits) - Force Pull is then the meaningful choice - so the
        // modal says so rather than showing an empty box. Built with
        // textContent, never innerHTML, so a note filename can't inject markup.
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

        // performSync handles the three buttons on the conflict modal in
        // index.html (moved here from an inline <script> in that file so
        // all sync UI logic lives together). It goes through window.runSync
        // above, so the modal and the header sync buttons can't disagree
        // about the wire format or response handling.
        window.performSync = async function(action) {
            const modal = document.getElementById('conflict-modal');
            if (action === 'abort') {
                // A plain "pull" never mutates local state before reporting a
                // conflict, so aborting here is purely a UI cancel — there is
                // nothing on the server to undo.
                if (modal) modal.classList.add('hidden');
                return;
            }
            if (modal) modal.classList.add('hidden');

            const data = await window.runSync(action);
            if (data && data.status === 'success') {
                // pull_force / pull_mark both change what's on disk under this
                // page, so reload to show it.
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

    // --- Bookmark capture UI (moved here from omn-go-core.js in Phase 5a) ---
    // handleShare (Android share-to), the URL drag-and-drop, and the tag
    // autocomplete all belong to the server-backed bookmark/quick-note
    // capture flow whose submit handlers already live in this file.
    window.handleShare = function(text, subject) {
        text = text || '';
        subject = subject || '';

        // Regex to find the first valid URL
        const urlMatch = text.match(/(https?:\/\/[^\s]+)/) || subject.match(/(https?:\/\/[^\s]+)/);

        if (urlMatch) {
            // URL Found -> Route to Bookmark Panel
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
            // No URL -> Route to Quick Note Panel
            let content = '';
            if (subject) content += subject + "\n\n";
            if (text) content += text;

            document.getElementById('quickText').value = content.trim();
            document.getElementById('quickPanel').classList.remove('hidden');
            document.getElementById('bmPanel').classList.add('hidden');
        }
    };

    // Global Drag & Drop for URLs (Bookmarks). Registered on
    // DOMContentLoaded: this file now runs in <head>, where
    // document.body is still null - touching it directly here would
    // throw and kill the rest of this script.
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
    // Suggests existing tags while typing into the Ingest Bookmark modal's
    // #bmTags field. Tags are typed comma-separated ("work, recipe, ita|" -
    // the "|" marks the caret); suggestions are computed against only the
    // fragment after the last comma, and are only shown once that fragment
    // reaches #bmTags's minChars attribute (default 2, set in index.html).
    // Picking a suggestion completes the fragment and appends ", " so the
    // next tag can be typed right away.
    //
    // This is plain same-origin UI sugar, not a "server extension" - unlike
    // the sync/login/etc. calls in omn-go-sse.js it doesn't need a protocol
    // guard: a failed fetch (e.g. the page opened offline) is treated as "no
    // suggestions" rather than an error, so the field still works as a plain
    // comma-separated text input either way.
    //
    // Both the DOM wiring and the tag-list fetch are deliberately lazy: they
    // only run the first time the Ingest Bookmark modal is actually opened,
    // not on every page load (most page views never touch this panel).
    // window.showBookmarkPanel()/toggleBookmarkPanel() below are the only
    // places that reveal #bmPanel - the header's "add bookmark" button, the
    // URL drag-and-drop handler, and window.handleShare all go through one of
    // them now instead of poking #bmPanel's classList directly - so "the
    // modal is opening" is caught in exactly one place.
    (function () {
        var tagsCache = null;    // null until prepared; array once loaded (even if empty)
        var tagsPromise = null;  // in-flight fetch, if any
        var wired = false;       // #bmTags/#bmTagsSuggestions listeners attached only once

        // Fetches /json/bookmarker-tags.json at most once per page. Safe to
        // call every time the modal opens: if the list is already prepared
        // (tagsCache set) or already loading (tagsPromise set) this reuses
        // that instead of firing a second request.
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

        // Attaches the input/keydown/click listeners to #bmTags exactly once.
        // Called from showBookmarkPanel()/toggleBookmarkPanel() every time the
        // modal opens; the `wired` guard makes repeat calls no-ops so reopening
        // the panel never double-attaches listeners.
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
                // Rebuilding from scratch (rather than splicing) keeps this
                // correct even if the fragment was picked mid-string; the
                // trailing ", " primes the field for the next tag.
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
                    // mousedown (not click) fires before #bmTags's blur, so
                    // the pick survives the input losing focus.
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
                    // The field may have moved on while this fetch/cache
                    // lookup was pending - drop a stale response.
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

        // Unconditionally shows #bmPanel (used by drag-and-drop and
        // handleShare, which only ever want it open, never toggled).
        window.showBookmarkPanel = function () {
            var panel = document.getElementById('bmPanel');
            if (!panel) return;
            panel.classList.remove('hidden');
            wireBookmarkTagAutocomplete();
            ensureTagsLoaded();
        };

        // Toggles #bmPanel (used by the header's "add bookmark" button, which
        // both opens and closes it). Only prepares the autocomplete on the
        // transition into "visible" - closing the panel does nothing extra.
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

        // db.exportBackup / db.restoreBackup were removed together with
        // the per-table db_json backup mechanism: backups are now
        // whole-database snapshots managed from the /db_backups page
        // (see db_backup.go).
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

    function checkRole() {
        if(document.cookie.includes('session_role=guest')) {
            document.querySelectorAll('.admin-only').forEach(el => {
                if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';
            });
        }
    }

    window.checkSession = async function() {
        // #loginOverlay is a server-injected modal (see injectRuntimeVars); an
        // exported/offline page has no login gate, so if it isn't present just
        // leave the already-visible content alone rather than dereferencing
        // null. #mainUI stays in the page, but guard it too for safety.
        const overlay = document.getElementById('loginOverlay');
        const main = document.getElementById('mainUI');
        if (!overlay || !main) return;
        // Unhide UI if role cookies exist
        if (document.cookie.includes('session_role=')) {
            overlay.style.display = 'none';
            main.style.display = 'flex';
            checkRole();
        } else {
            // Check if server is configured with public role or check backend
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

    // GoOMN Log Interceptor - Bridges Go background logs to JS UI
    document.addEventListener('DOMContentLoaded', () => {
        try {
            const logSource = new EventSource('/api/logs');
	    // stream is released before the document is cached
	    window.addEventListener('pagehide', () => logSource.close());
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
    window.handleShare = function() { printDebug('handleShare'); };
    window.showBookmarkPanel = function() { printDebug('showBookmarkPanel'); };
    window.toggleBookmarkPanel = function() { printDebug('toggleBookmarkPanel'); };
}
