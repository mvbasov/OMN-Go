// --- OMN-Go Server Extensions ---
//
// This file talks to the Go backend. It bypasses itself when a person
// opens an exported page from disk, where no server answers. See the
// else branch at the end.
//
// WHAT LOADS WHEN, SINCE 26.09.24
//
// index.html carries three script elements, and this is the third. The
// file held 2045 lines until 26.09.23, and every note page parsed all of
// them to run a fraction. Three parts are now files of their own, and
// each one arrives at the first press of the control that needs it:
//
//	omn-go-sync.js      the sync buttons and the commit modal
//	omn-go-bookmark.js  the bookmark panel and the tag autocomplete
//	omn-go-search.js    the search overlay
//
// omnLoadModule below fetches one of them, and omnLazy writes a stub for
// each global that it holds. The stub loads the file, then calls the real
// function with the same arguments.
//
// WHAT CANNOT BE LAZY, AND WHY.
//
//  1. THE SQL API. ScriptRules.md documents a plain <script> in a note
//     that calls omnGoOpenDatabase while the page parses. A script that
//     this file injects runs after the parser, thus the note would find
//     no function. omnGoOpenDatabase stays in this file.
//  2. omnGoInsertCapture. MainActivity compares the answer of this
//     function against true, with three equal signs. A stub answers with
//     a Promise, which is not true, thus Android would show its own
//     dialog in place of the panel that opened.
//  3. THE DRAG AND DROP LISTENER. It must already listen when a person
//     drops a link. It writes the two boxes itself and calls the lazy
//     showBookmarkPanel last.
//  4. THE SEARCH KEYBOARD SHORTCUT. Ctrl-K and the slash key must work
//     before the overlay exists. The listener is here, and it calls the
//     lazy omnSearchOpen.
//  5. THE LOG STREAM AND THE SESSION CHECK. Each page starts both.

// printDebug is defined OUTSIDE the guard below, and it was inside the
// else branch until 26.09.24. Each stub of that branch calls it, and the
// split files each carry a branch of their own now. One definition, at
// the top, answers for all of them.
window.printDebug = function (funcName) {
    console.debug("'" + funcName + "' Not usable on standalone page");
};

if (window.location.protocol !== 'file:') {

    // Subscribers to the /api/logs SSE stream (registered below, fed by the
    // EventSource at the bottom of this file). Returns an unsubscribe
    // function - callers MUST call it, or a finished operation keeps
    // reacting to unrelated server log lines.
    const logSubscribers = [];
    window.omnGoOnServerLog = function(fn) {
        logSubscribers.push(fn);
        return function() {
            const i = logSubscribers.indexOf(fn);
            if (i !== -1) logSubscribers.splice(i, 1);
        };
    };

    // Decides whether one server log line reaches the browser console.
    //
    // The server sends EVERY line over /api/logs, whatever the Config page
    // says (see logger.go). Two things need that. The sync progress overlay
    // below is fed by "[sync]" lines, most of which are (debug). And a
    // change on the Config page then applies to the next line, with no
    // server restart and no page reload of the writer's side.
    //
    // So the filter lives here. OMN_LOG_DEBUG, OMN_LOG_INFO and
    // OMN_LOG_TAGS arrive with the runtime variables the server injects
    // into every page (see injectRuntimeVars in templates.go).
    //
    // A line reads "<stamp> [tag] (level) message". A line with no level -
    // the three log.Printf call sites that cannot reach an application -
    // always prints, because each one is a fault.
    const LOG_LINE_RE = /\[([a-z0-9-]+)\]\s+\(([a-z]+)\)\s/;
    function logLinePrints(msg) {
        const m = LOG_LINE_RE.exec(msg);
        if (!m) return true;
        const tag = m[1], level = m[2];
        if (level === 'error') return true;
        if (level === 'debug' && !window.OMN_LOG_DEBUG) return false;
        if (level === 'info' && !window.OMN_LOG_INFO) return false;
        const tags = (window.OMN_LOG_TAGS || '').split(',');
        return tags.indexOf(tag) !== -1;
    }
    // Exposed so a test can run it, and harmless in a browser. The same
    // shape as OMN_expandEmmet in omn-go-editor.js.
    //
    // logLinePrints and logLineEnabled in backend/logger.go are the two
    // implementations of ONE decision, which rule 7 of CLAUDE.md section
    // 1 allows only with a test that compares them.
    // TestLogFilterPortAgreesWithTheRealJavaScript is that test, and it
    // needs a name to call.
    window.logLinePrints = logLinePrints;

    // Maps a backend "[sync] ..." log line to a human-readable stage. First
    // match wins, so more specific prefixes come first. Anything unmatched
    // leaves the current stage alone and only updates the detail line - that
    // way a log message added to git_sync.go later degrades to "still
    // working" rather than blanking the stage.
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
        ['No SSH key',              'No SSH key configured'],
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
        // go-git sideband text relayed from the remote (see
        // syncProgressWriter in git_sync.go).
        ['remote:',                 'Transferring…'],
        ['Counting objects',        'Transferring…'],
        ['Compressing objects',     'Transferring…'],
        ['Receiving objects',       'Transferring…'],
        ['Resolving deltas',        'Transferring…'],
        ['Writing objects',         'Transferring…']
    ];

    // Feeds one server log line into the progress overlay. Only "[sync]"
    // lines are relevant; everything else on the stream is ignored so an
    // unrelated background log cannot hijack the display.
    //
    // A line reads "<stamp> [sync] (debug) Staging file: x". The level word
    // sits between the tag and the message. It is stripped here before any
    // SYNC_STAGES prefix is tried, or every prefix below stops matching. Sync progress is mostly (debug), and this overlay keeps
    // working with (debug) switched off because the SSE stream always
    // carries every line (see logger.go).
    function applySyncLogLine(msg) {
        const at = msg.indexOf('[sync]');
        if (at === -1) return;
        const line = msg.slice(at + '[sync]'.length).trim().replace(/^\([a-z]+\)\s*/, '');
        if (!line) return;
        for (const [prefix, label] of SYNC_STAGES) {
            if (line.indexOf(prefix) === 0) {
                window.OMNProgress.stage(label);
                break;
            }
        }
        window.OMNProgress.detail(line);
    }

    // applySyncLogLine reaches omn-go-sync.js through window, and not
    // through the scope.
    //
    // The body of this file sits inside an if block. Annex B of the
    // standard hoists a function of such a block to the global scope.
    // omn-go-sync.js found this name that way for 17 versions, by
    // accident. A const of the same block does NOT hoist, and
    // SYNC_TITLES broke the upload of 26.09.24 for that reason. See the
    // banner of omn-go-sync.js.
    //
    // One export, written out, cannot break that way.
    window.applySyncLogLine = applySyncLogLine;

    // ------------------------------------------------------------------
    // The lazy loader
    // ------------------------------------------------------------------
    //
    // omnLoadModule fetches one script one time and answers with the same
    // Promise for each later call. A failed load drops the record, thus a
    // second press tries again.
    //
    // The path is absolute. A compiled note can sit at any depth under
    // html/, and the asset prefix of the shell is relative for such a
    // page. An absolute path answers from each of them.
    const omnModuleLoads = {};
    function omnLoadModule(file) {
        if (omnModuleLoads[file]) return omnModuleLoads[file];
        omnModuleLoads[file] = new Promise(function (resolve, reject) {
            const el = document.createElement('script');
            el.src = '/js/OMN-Go/' + file;
            // async = false keeps the execution order of two injected
            // scripts. Nothing here loads two at one time today, and the
            // line costs nothing and removes a surprise.
            el.async = false;
            el.onload = function () { resolve(); };
            el.onerror = function () {
                delete omnModuleLoads[file];
                reject(new Error('OMN-Go: failed to load ' + file));
            };
            document.head.appendChild(el);
        });
        return omnModuleLoads[file];
    }
    window.omnLoadModule = omnLoadModule;

    // omnLazy writes a stub for each name. The stub loads the file and
    // then calls the real function, which the file wrote over the stub.
    //
    // The stub answers with a Promise. Each name below is a control that a
    // person presses or that Android calls without reading the answer, thus
    // no caller loses anything. A function whose answer a caller reads must
    // NOT be lazy. See omnGoInsertCapture and point 2 of the banner.
    function omnLazy(file, names) {
        names.forEach(function (name) {
            const stub = function () {
                const args = arguments, self = this;
                return omnLoadModule(file).then(function () {
                    if (window[name] === stub) {
                        // The file loaded and defined nothing. A silent
                        // return would look like a dead button.
                        console.error('OMN-Go: ' + file + ' defines no ' + name);
                        return;
                    }
                    return window[name].apply(self, args);
                });
            };
            window[name] = stub;
        });
    }

    omnLazy('omn-go-sync.js', [
        'runSync', 'syncAction', 'performSync', 'performPushForce',
        'hidePushConflictModal', 'previewAndCommit', 'commitAndUpload',
        'hideCommitModal',
    ]);
    omnLazy('omn-go-bookmark.js', [
        'handleShare', 'showBookmarkPanel', 'toggleBookmarkPanel',
    ]);
    omnLazy('omn-go-search.js', ['omnSearchOpen']);

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


    // Called from Android (MainActivity.insertCapturedText) to pre-fill the
    // Quick Note panel with a captured result - a scanned barcode, or a Termux
    // command's output - for the user to review and save. Unlike handleShare,
    // this ALWAYS targets the Quick Note panel (never the bookmark panel), so a
    // scanned URL still lands in Quick Notes as the user asked, rather than
    // being re-routed. Returns true only if the panel actually exists on the
    // current page; the native side uses that to fall back to its own dialog
    // when the WebView is on a page without the panel (e.g. mid-edit on
    // editor.html, which doesn't load this file at all).
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

    // roleHint reads the role out of the session_role_hint cookie.
    //
    // THIS VALUE DECIDES NOTHING ON THE SERVER. The server reads the
    // signed session_role cookie, which is HttpOnly and thus invisible
    // here. See the banner of backend/session.go. A reader who changes
    // this cookie changes what this page shows and gets no permission.
    //
    // The old code asked document.cookie.includes('session_role=guest').
    // The signed cookie is HttpOnly since 26.09.6, thus that test found
    // nothing and each guest saw the controls of an admin.
    function roleHint() {
        var parts = document.cookie.split(';');
        for (var i = 0; i < parts.length; i++) {
            var pair = parts[i].trim();
            if (pair.indexOf('session_role_hint=') === 0) {
                return pair.slice('session_role_hint='.length);
            }
        }
        return '';
    }

    function checkRole() {
        if (roleHint() === 'guest') {
            document.querySelectorAll('.admin-only').forEach(el => {
                if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';
            });
        }
    }

    window.checkSession = async function() {
        // #loginOverlay is a server-injected modal. See injectRuntimeVars.
        // An exported page and an offline page have no login gate. When
        // the element is absent, leave the visible content alone rather
        // than read a property of null. #mainUI stays in the page, and
        // the guard covers it for the same reason.
        const overlay = document.getElementById('loginOverlay');
        const main = document.getElementById('mainUI');
        if (!overlay || !main) return;
        // A hint cookie means that this browser logged in. The server
        // still tests the signed cookie on each request.
        if (roleHint() !== '') {
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
    //
    // Every log line the backend writes reaches this stream (see logger.go),
    // which is why the sync progress overlay needs no transport of its own:
    // git_sync.go's "[sync] ..." lines are the progress feed. Subscribers
    // registered through window.omnGoOnServerLog get each line in addition
    // to the console mirroring that has always happened here.
    //
    // The stream carries every level. The console mirror does not: it asks
    // logLinePrints above, which reads the switches of the Config page. A
    // subscriber is never filtered, because the overlay is built on the
    // (debug) lines a reader normally does not want to see.
    //
    // Caveat worth knowing: JSLogger drops a message rather than blocking
    // when a client's 10-slot channel is full, so this stream is a live
    // sample, not a guaranteed-complete transcript. That is fine for a
    // progress display (it only ever shows the newest line) but means it
    // must never be used to drive state that has to see every event.
    document.addEventListener('DOMContentLoaded', () => {
        try {
            const logSource = new EventSource('/api/logs');
	    // stream is released before the document is cached
	    window.addEventListener('pagehide', () => logSource.close());
            logSource.onmessage = function(event) {
                let msg = event.data.trim();
                if(msg) {
                    // The console mirror is filtered. Every subscriber
                    // below still gets the line, whatever its level: the
                    // sync overlay needs the (debug) lines it is built on.
                    if (logLinePrints(msg)) console.log("[GO] " + msg);
                    for (const fn of logSubscribers.slice()) {
                        try { fn(msg); } catch (e) { /* a bad subscriber must not kill the stream */ }
                    }
                }
            };
        } catch(e) {
            console.error("Log source error:", e);
        }
    });


    // ------------------------------------------------------------------
    // The search shortcuts and the highlight alias
    // ------------------------------------------------------------------
    //
    // Both stay in this file, and omn-go-search.js holds the overlay. A
    // shortcut has to work before the overlay exists, thus the listener
    // cannot travel with the code that it opens.
    //
    // The highlighting itself lives in omn-go-core.js. It runs on each
    // page, an exported one included, thus the alias needs no server half
    // and no lazy load.
    window.omnSearchClearHighlights = window.omnClearHighlights;

    // Ctrl-K anywhere, and the slash key when the reader is not typing.
    // Those are the two conventions that people arrive with. Both are
    // desktop conventions in practice, and the header button is the route
    // on Android.
    //
    // Escape asks the overlay whether it is open. omn-go-search.js exports
    // omnSearchIsOpen and omnSearchClose for this listener. Neither one
    // exists before the first open, and Escape then does nothing, which is
    // the correct answer when no overlay is on screen.
    document.addEventListener('keydown', function (e) {
        if (e.defaultPrevented) return;

        if ((e.key === 'k' || e.key === 'K') && (e.ctrlKey || e.metaKey)) {
            e.preventDefault();
            window.omnSearchOpen();
            return;
        }
        if (e.key === 'Escape') {
            if (window.omnSearchIsOpen && window.omnSearchIsOpen()) {
                e.preventDefault();
                window.omnSearchClose();
            }
            return;
        }
        if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey) return;

        const t = e.target;
        const tag = (t && t.tagName) ? t.tagName.toLowerCase() : '';
        if (tag === 'input' || tag === 'textarea' || (t && t.isContentEditable)) return;

        e.preventDefault();
        window.omnSearchOpen();
    });


    // ----------------------------------------------------------------
    // The receive box on the Incoming notes page (note exchange)
    // ----------------------------------------------------------------
    //
    // The markup is in modals.html and the behaviour is here, for the same
    // reason every other panel is split that way: the incoming index is the
    // user's note and holds nothing but the list of what arrived. A control
    // that OMN-Go owns has no business being stored inside it, where a user
    // could delete half of it and be left with a box that does nothing.
    //
    // WHICH PAGE. OMN_INCOMING_PAGE is the note name, injected per request
    // (see injectRuntimeVars) so the name lives in Go beside the code that
    // writes the page and not as a second copy in this file.
    //
    // NOT ON ANDROID. The share sheet receives a note there; this box would
    // be a control that duplicates it and cannot be reached from the
    // application that holds the file.
    //
    // The whole block sits inside the "not file:" gate above, so an exported
    // page never runs it - and never carries the markup either, because the
    // modals slot is filled at serve time.
    (function () {
        function onIncomingPage() {
            return typeof OMN_INCOMING_PAGE !== 'undefined' &&
                typeof PageName !== 'undefined' &&
                PageName === OMN_INCOMING_PAGE;
        }

        function setUp() {
            const box = document.getElementById('omnIncoming');
            if (!box) return;
            if (!onIncomingPage() || (typeof IS_ANDROID !== 'undefined' && IS_ANDROID)) {
                box.remove();
                return;
            }
            const preview = document.getElementById('preview');
            const input = document.getElementById('omnIncomingFiles');
            const button = document.getElementById('omnIncomingImport');
            const statusEl = document.getElementById('omnIncomingStatus');
            if (!preview || !input || !button || !statusEl) return;

            // Above the list, which is what the page is for. The box is
            // closed until it is wanted, so it costs one line.
            preview.insertBefore(box, preview.firstChild);
            box.classList.remove('hidden');

            const say = function (msg, bad) {
                statusEl.textContent = msg || '';
                statusEl.classList.toggle('is-error', !!bad);
            };

            // One note per request. The rules live in the backend
            // (note_exchange.go), which is the same code the Android share
            // path reaches - so a note lands in the same place whichever
            // way it came.
            const importOne = async function (file) {
                const form = new FormData();
                form.append('file', file, file.name);
                const res = await fetch('/api/import/note', { method: 'POST', body: form });
                let data = {};
                try { data = await res.json(); } catch (e) { /* not JSON */ }
                if (res.status === 401) {
                    throw new Error('log in as admin to import a note');
                }
                if (!res.ok || data.status !== 'success') {
                    throw new Error(data.message || ('HTTP ' + res.status));
                }
                return data;
            };

            const run = async function (files) {
                box.open = true;
                if (!files || !files.length) {
                    say('Choose a file first.', true);
                    return;
                }
                button.disabled = true;
                let done = 0;
                const failed = [];
                for (let i = 0; i < files.length; i++) {
                    say('Importing ' + (i + 1) + ' of ' + files.length + '…');
                    try {
                        await importOne(files[i]);
                        done++;
                    } catch (e) {
                        failed.push(files[i].name + ': ' + e.message);
                    }
                }
                button.disabled = false;

                if (!failed.length) {
                    // The list is written by the server, so the page has to
                    // come again to show what just arrived.
                    say('Imported ' + done + '. Refreshing…');
                    window.location.reload();
                    return;
                }
                say(done + ' imported, ' + failed.length + ' failed — ' + failed.join('; '), true);
            };

            button.addEventListener('click', function () { run(input.files); });

            // Dropping a file on the box is the other way a desktop does
            // this. The target is the whole element, so a file can be
            // dropped on the closed summary as well as on the open box.
            ['dragenter', 'dragover'].forEach(function (name) {
                box.addEventListener(name, function (e) {
                    e.preventDefault();
                    box.classList.add('is-over');
                });
            });
            ['dragleave', 'dragend'].forEach(function (name) {
                box.addEventListener(name, function () { box.classList.remove('is-over'); });
            });
            box.addEventListener('drop', function (e) {
                e.preventDefault();
                box.classList.remove('is-over');
                run(e.dataTransfer && e.dataTransfer.files);
            });
        }

        if (document.readyState === 'loading') {
            document.addEventListener('DOMContentLoaded', setUp);
        } else {
            setUp();
        }
    })();

} else {
    console.warn("OMN-Go: Page opened locally. Server Extensions (Sync/SSE) safely disabled.");

    window.login = function() { printDebug('login'); };
    window.createNewPage = function() { printDebug('createNewPage'); };
    window.submitQuickNote = function() { printDebug('submitQuickNote'); };
    window.submitBookmark = function() { printDebug('submitBookmark'); };
    window.checkSession = function() { printDebug('checkSession'); };
    window.omnGoOpenDatabase = function() { printDebug('omnGoOpenDatabase'); };
    window.openDatabase = function() { printDebug('openDatabase'); };
    // Search needs the server: the panel queries /api/search, which does not
    // exist on a page opened from disk. The header button is .server-only and
    // therefore already hidden here (applyOfflineUI) - these stubs cover a
    // note script or a stale keyboard shortcut calling in anyway.
    // omnSearchClearHighlights is NOT stubbed here: the highlighting lives in
    // omn-go-core.js and works offline, so the real one is already defined.
}
