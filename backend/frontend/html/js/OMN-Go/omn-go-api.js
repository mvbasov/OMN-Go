// --- OMN-Go Server Extensions ---
//
// This file talks to the Go backend. It bypasses itself when a person
// opens an exported page from disk, where no server answers. See the
// else branch at the end.
//
// WHAT LOADS WHEN
//
// index.html carries three script elements, and this is the third. Every
// note page parses this file. Three parts are files of their own, and
// each one arrives at the first press of the control that needs it. See
// doc/decisions/0015-load-the-click-driven-scripts-on-demand.md:
//
//	omn-go-sync.js      the sync buttons and the commit modal
//	omn-go-bookmark.js  the bookmark panel and the tag autocomplete
//	omn-go-search.js    the search overlay
//
// omnLoadModule below fetches one of them. omnLazy writes a stub for each
// global that the file holds, and omnLazyActions writes a stub for each
// action that it gives. The stub loads the file, then calls the real
// function with the same arguments.
//
// WHAT CANNOT BE LAZY, AND WHY.
//
//  1. THE SQL API. ScriptRules.md documents a plain <script> in a note
//     that calls omnGoOpenDatabase while the page parses. A script that
//     this file injects runs after the parser, thus the note would find
//     no function. omnGoOpenDatabase stays in this file.
//  2. omnGoInsertCapture. IntentBridge compares the answer of this
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

// This file defines printDebug OUTSIDE the guard below. Each stub of the else
// branch calls it, and the split files each carry a branch of their own.
// One definition, at the top, answers for all of them.
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
    // says (see backend/internal/logx/hub.go). Two things need that. The
    // sync progress overlay below reads the "[sync]" lines, and most of them
    // are (debug). Also, a change on the Config page then applies to the
    // next line, with no server restart and no page reload of the writer's
    // side.
    //
    // So the filter lives here. OMN_LOG_DEBUG, OMN_LOG_INFO and OMN_LOG_TAGS
    // arrive with the runtime variables the server injects into every page (see
    // render.Renderer.InjectRuntimeVars in internal/render/pages.go).
    //
    // A line reads "<stamp> [tag] (level) message". A line with no level -
    // the three log.Printf call sites that cannot reach an application -
    // always prints, because each one is a fault.
    const LOG_LINE_RE = /\[([a-z0-9-]+)\]\s+\(([a-z]+)\)\s/;

    // omnParseLogLine splits one server log line into its tag and its
    // level. It answers null for a line that carries neither.
    //
    // THE SHAPE OF A LINE LIVES HERE ALONE. logLinePrints below reads it,
    // and so does omn-go-logs.js for the filter of the Log page. Rule 7
    // of CLAUDE.md section 1 asks for one authority, and a second regular
    // expression in the other file would be a second one.
    //
    // A separate file reads this through window, and never as a bare
    // name. The body of this file sits inside an if block, thus a const
    // of that block reaches no other file. See section 4 of CLAUDE.md.
    window.omnParseLogLine = function(msg) {
        const m = LOG_LINE_RE.exec(msg);
        if (!m) return null;
        return { tag: m[1], level: m[2] };
    };

    function logLinePrints(msg) {
        const parts = window.omnParseLogLine(msg);
        if (!parts) return true;
        const tag = parts.tag, level = parts.level;
        if (level === 'error') return true;
        if (level === 'debug' && !window.OMN_LOG_DEBUG) return false;
        if (level === 'info' && !window.OMN_LOG_INFO) return false;
        const tags = (window.OMN_LOG_TAGS || '').split(',');
        return tags.indexOf(tag) !== -1;
    }
    // Exposed so a test can run it, and harmless in a browser. The same
    // shape as OMN_expandEmmet in omn-go-editor.js.
    //
    // logLinePrints and Logger.Enabled in backend/internal/logx/hub.go are
    // the two implementations of ONE decision, which rule 7 of CLAUDE.md section
    // 1 allows only with a test that compares them.
    // TestLogFilterPortAgreesWithTheRealJavaScript is that test, and it
    // needs a name to call.
    window.logLinePrints = logLinePrints;

    // Maps a backend "[sync] ..." log line to a human-readable stage. The
    // first match wins, thus a more specific prefix comes first. Anything
    // unmatched leaves the current stage alone and updates the detail line
    // alone. A new log message of the sync code thus shows as "still
    // working", and it does not clear the stage.
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
        // syncProgressWriter in internal/gitsync/sync.go).
        ['remote:',                 'Transferring…'],
        ['Counting objects',        'Transferring…'],
        ['Compressing objects',     'Transferring…'],
        ['Receiving objects',       'Transferring…'],
        ['Resolving deltas',        'Transferring…'],
        ['Writing objects',         'Transferring…']
    ];

    // Feeds one server log line into the progress overlay. Only a "[sync]"
    // line is relevant. Everything else on the stream is ignored, thus an
    // unrelated background log cannot hijack the display.
    //
    // A line reads "<stamp> [sync] (debug) Staging file: x". The level word
    // sits between the tag and the message. It is stripped here before any
    // SYNC_STAGES prefix is tried, or every prefix below stops matching. Sync progress is mostly (debug), and this overlay keeps
    // working with (debug) switched off because the SSE stream always
    // carries every line (see backend/internal/logx/hub.go).
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
    // standard hoists a function of such a block to the global scope. A
    // const of the same block does NOT hoist. Do not depend on that
    // difference. See doc/decisions/0015-load-the-click-driven-scripts-on-demand.md.
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

    // omnModuleStyles names the stylesheet of a lazy file. The file shows
    // nothing before its stylesheet is there, thus the load waits for both.
    const omnModuleStyles = { 'omn-go-search.js': 'omn-go-search.css' };

    // omnLoadStyle puts one stylesheet before omn-go-custom.css, thus a rule
    // of the user still wins. A page that names the stylesheet already
    // answers at once. A failed stylesheet does not stop the script.
    function omnLoadStyle(file) {
        const href = '/css/OMN-Go/' + file;
        if (document.querySelector('link[href$="/' + file + '"]')) return Promise.resolve();
        return new Promise(function (resolve) {
            const link = document.createElement('link');
            link.rel = 'stylesheet';
            link.href = href;
            link.onload = function () { resolve(); };
            link.onerror = function () { resolve(); };
            const custom = document.querySelector('link[href*="omn-go-custom.css"]');
            if (custom && custom.parentNode === document.head) {
                document.head.insertBefore(link, custom);
            } else {
                document.head.appendChild(link);
            }
        });
    }

    function omnLoadModule(file) {
        if (omnModuleLoads[file]) return omnModuleLoads[file];
        const script = new Promise(function (resolve, reject) {
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
        const style = omnModuleStyles[file] ? omnLoadStyle(omnModuleStyles[file]) : Promise.resolve();
        omnModuleLoads[file] = Promise.all([script, style]).then(function () {});
        return omnModuleLoads[file];
    }
    window.omnLoadModule = omnLoadModule;

    // omnLazy writes a stub for each name. The stub loads the file and
    // then calls the real function, which the file wrote over the stub.
    //
    // The stub answers with a Promise. Each name below is a control that a
    // person presses or that Android calls without reading the answer, thus
    // no caller loses anything. A function whose answer a caller reads must
    // NOT be lazy. See omnGoInsertCapture and point 2 of the banner. The one
    // exception is a function that answers a Promise itself, for example
    // omnConfirm. The caller waits for the Promise of the stub, and the
    // stub gives the answer of the real Promise.
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

    // omnLazyActions gives each action a stub. The stub loads the file, and
    // the file calls OMN.action for the same name. The stub then calls the
    // function that the file gave. See OMN.action in omn-go-core.js.
    //
    // A lazy file that only a control starts needs no name on window.
    function omnLazyActions(file, names) {
        names.forEach(function (name) {
            const stub = function (el, event) {
                return omnLoadModule(file).then(function () {
                    const fn = window.OMN.action(name);
                    if (fn === stub) {
                        console.error('OMN-Go: ' + file + ' gives no action ' + name);
                        return;
                    }
                    return fn(el, event);
                });
            };
            window.OMN.action(name, stub);
        });
    }

    omnLazyActions('omn-go-sync.js', ['sync']);
    omnLazy('omn-go-bookmark.js', ['handleShare', 'showBookmarkPanel']);
    omnLazyActions('omn-go-bookmark.js', ['bookmark-panel']);
    omnLazy('omn-go-search.js', ['omnSearchOpen']);
    // The dialogs of the application. Each name answers a Promise. See
    // omn-go-dialog.js.
    omnLazy('omn-go-dialog.js', ['omnDialog', 'omnAlert', 'omnConfirm', 'omnPrompt']);
    // omnSearchOpen keeps its name, because the User Manual promises it.
    window.OMN.action('search', function () { window.omnSearchOpen(); });

    // askLogin shows the login dialog until the server accepts the
    // password. The dialog has no Cancel, and Escape and the Back key do
    // nothing: the page under it needs a login. A wrong password shows
    // "Invalid password." in the dialog, and the dialog stays open.
    function askLogin() {
        return window.omnDialog({
            kind: 'login',
            title: 'OMN-Go Login',
            closable: false,
            fields: [{ type: 'password', placeholder: 'Admin Password', autocomplete: 'current-password' }],
            check: async function (answer) {
                const res = await fetch('/login', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/x-www-form-urlencoded'},
                    body: 'password=' + encodeURIComponent(answer.input)
                });
                return res.ok ? '' : 'Invalid password.';
            },
            buttons: [{ label: 'Enter', value: true, default: true }],
        });
    }

    // The actions of this block need the server. The else branch at the end
    // of this file gives each one a stub for a page from disk.

    function toCamelCase(str) {
        let words = str.split(/[-_\s]+/);
        return words.map(w => w ? w.charAt(0).toUpperCase() + w.slice(1) : '').join('');
    }

    // fileNameOf makes a file name from a title: "my new note" gives
    // "MyNewNote". Each other character becomes "-".
    function fileNameOf(title) {
        return toCamelCase(title).replace(/[^a-zA-Z0-9-]/g, '-');
    }

    // One dialog asks for the title and the file name. The file name
    // follows the title while the person types. A file name that the
    // person typed stops that. An empty file name starts it again.
    window.OMN.action('new-page', async function () {
        let nameTyped = false;
        const answer = await window.omnDialog({
            kind: 'form',
            title: 'New page',
            text: 'The file name comes from the title. You can change it.',
            fields: [{ label: 'Title' }, { label: 'File name' }],
            onInput: function (inputs, index) {
                if (index === 1) {
                    nameTyped = inputs[1].value !== '';
                } else if (!nameTyped) {
                    inputs[1].value = fileNameOf(inputs[0].value);
                }
            },
            buttons: [
                { label: 'Create', value: true, default: true },
                { label: 'Cancel', value: false, cancel: true },
            ],
        });
        if (answer.value !== true) return;
        const title = answer.inputs[0].trim();
        if (!title) return;
        const fileName = answer.inputs[1].trim() || fileNameOf(title);

        let src = typeof currentNote !== 'undefined' ? currentNote : 'Welcome';
        const fd = new URLSearchParams();
        fd.append('source', src);
        fd.append('target', fileName);
        fd.append('title', title);

        const res = await fetch('/api/newpage', { method: 'POST', body: fd });
        if (res.ok) {
            // The server resolves fileName relative to the directory of the
            // current page. A bare name becomes a sibling of src, and not a
            // root-level page. The created page may thus live at
            // "local/test" although fileName was only "test". Redirect with
            // what the server says it created.
            const resolvedTarget = await res.text();
            window.location.href = '/' + resolvedTarget + '.html?edit=true';
        } else {
            await window.omnAlert('The new page was not created.');
        }
    });


    window.OMN.action('quick-note-save', async function () {
        const fd = new URLSearchParams();
        fd.append('note', document.getElementById('omn-quick-text').value);
        const res = await fetch('/api/quick', { method: 'POST', body: fd });
        if(res.ok) {
            document.getElementById('omn-quick-text').value = '';
            document.getElementById('omn-quick-panel').classList.add('omn-hidden');
            await window.omnAlert('Saved.');
            window.location.reload();
        }
    });

    window.OMN.action('bookmark-save', async function () {
        const fd = new URLSearchParams();
        fd.append('url', document.getElementById('omn-bm-url').value);
        fd.append('title', document.getElementById('omn-bm-title').value);
        fd.append('tags', document.getElementById('omn-bm-tags').value);
        fd.append('notes', document.getElementById('omn-bm-notes').value);
        const res = await fetch('/api/bookmark', { method: 'POST', body: fd });
        if(res.ok) {
            document.getElementById('omn-bm-panel').classList.add('omn-hidden');
            document.querySelectorAll('#omn-bm-panel input, #omn-bm-panel textarea').forEach(el => el.value = '');
            await window.omnAlert('Saved.');
            window.location.reload();
        }
    });


    // Called from Android, in IntentBridge.insertCapturedText, to pre-fill
    // the Quick Note panel with a captured result. That result is a scanned
    // barcode, or the output of a Termux command. The user reviews it and
    // saves it.
    //
    // Unlike handleShare, this ALWAYS targets the Quick Note panel, and never
    // the bookmark panel. A scanned URL thus still lands in Quick Notes as
    // the user asked, and it is not re-routed.
    //
    // It answers true only when the panel exists on the current page. The
    // native side uses that answer to fall back to its own dialog, when the
    // WebView is on a page without the panel. A page mid-edit on editor.html
    // is such a page, because it loads this file not at all.
    window.omnGoInsertCapture = function(text, label) {
        var q = document.getElementById('omn-quick-text');
        var p = document.getElementById('omn-quick-panel');
        if (!q || !p) { return false; }
        var content = '';
        if (label) { content += label + "\n\n"; }
        if (text) { content += text; }
        q.value = content.trim();
        p.classList.remove('omn-hidden');
        var bm = document.getElementById('omn-bm-panel');
        if (bm) { bm.classList.add('omn-hidden'); }
        return true;
    };

    // Global Drag & Drop for URLs (Bookmarks). It is registered on
    // DOMContentLoaded. This file now runs in <head>, where document.body is
    // still null. To touch it directly here would throw, and it would kill
    // the rest of this script.
    document.addEventListener('DOMContentLoaded', () => {
        document.body.addEventListener('dragover', e => {
            if (!e.target.closest('#omn-editor-text')) e.preventDefault();
        });
        document.body.addEventListener('drop', e => {
            if (e.target.closest('#omn-editor-text')) return;
            const url = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
            if (url && (url.startsWith('http://') || url.startsWith('https://'))) {
                e.preventDefault();
                document.getElementById('omn-bm-url').value = url;
                document.getElementById('omn-bm-title').value = '';
                const html = e.dataTransfer.getData('text/html');
                if (html) {
                    const match = html.match(/<a[^>]*>(.*?)<\/a>/i);
                    if (match && match[1]) {
                        document.getElementById('omn-bm-title').value = match[1].replace(/<[^>]+>/g, '').trim();
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
    //
    //   - const db = omnGoOpenDatabase('mydata');
    //   - await db.exec('CREATE TABLE IF NOT EXISTS t(a,b)');
    //   - const r = await db.exec('SELECT * FROM t WHERE a > ?', [5]);
    //   - r.rows._array.forEach(row => console.log(row.a, row.b));
    //   - await db.batch([['INSERT INTO t VALUES(?,?)', [1,2]],
    //                     ['INSERT INTO t VALUES(?,?)', [3,4]]]); // atomic
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
            // Every statement queued synchronously inside the callback runs
            // as ONE atomic transaction on the server. A statement queued
            // from inside a success callback runs as a FOLLOW-UP atomic
            // batch, which is a separate transaction. That is the one
            // semantic difference from real WebSQL, where the whole cascade
            // shared a transaction.
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
                        // okCb calls may have queued more statements. Loop.
                    }
                    if (doneCb) doneCb();
                })();
            }
        };
        db.readTransaction = db.transaction;
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
    // here. See the banner of backend/internal/app/session.go. A reader who
    // changes this cookie changes what this page shows and gets no
    // permission.
    //
    // A test of document.cookie for 'session_role' finds nothing, because
    // the signed cookie is HttpOnly.
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

    window.checkSession = async function() {
        // An exported page and an offline page have no login gate. When the
        // server sends a page, InjectRuntimeVars puts the modals into
        // #omn-go-modals-slot. A page that still holds the empty slot is
        // thus a copy from disk.
        const main = document.getElementById('omn-main-ui');
        if (!main || document.getElementById('omn-go-modals-slot')) return;
        // An admin hint cookie means that this browser logged in. The
        // server still tests the signed cookie on each request. An old
        // "guest" hint gives no role. See doc/decisions/0018-keep-one-role.md.
        if (roleHint() !== 'admin') {
            // Only the answer 401 needs a login. The device itself and a
            // server with no admin password get each other answer.
            const test = await fetch('/api/config');
            if (test.status === 401) {
                main.style.display = 'none';
                await askLogin();
            }
        }
        main.style.display = 'flex';
    };

    // GoOMN Log Interceptor - Bridges Go background logs to JS UI
    //
    // Every log line that the backend writes reaches this stream, see
    // backend/internal/logx/hub.go. That is why the sync progress overlay
    // needs no transport of its own. The "[sync] ..." lines of the sync code are the progress feed.
    // A subscriber registered through window.omnGoOnServerLog gets each
    // line, beside the console mirroring that has always happened here.
    //
    // The stream carries every level. The console mirror does not: it asks
    // logLinePrints above, which reads the switches of the Config page. A
    // subscriber is never filtered, because the overlay is built on the
    // (debug) lines a reader normally does not want to see.
    //
    // Hub.Broadcast drops a message and does not wait when the 10-slot
    // channel of a client is full. This stream is thus a
    // live sample, and not a guaranteed-complete transcript.
    //
    // That is fine for a progress display, which only ever shows the newest
    // line. It does mean that the stream must never drive state that has to
    // see every event.
    document.addEventListener('DOMContentLoaded', () => {
        // A caller on another machine with no admin cookie gets 401 from
        // the stream. The EventSource does not try again after an HTTP
        // status.
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
    // The highlighting itself lives in omn-go-highlight.js. It runs on each
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
    // The markup is in modals.html and the behavior is here. Every other
    // panel is split that way for the same reason. The incoming index is the
    // note of the user, and it holds nothing but the list of what arrived. A
    // control that OMN-Go owns has no business inside it. A user could
    // delete half of it there, and be left with a box that does nothing.
    //
    // WHICH PAGE. OMN_INCOMING_PAGE is the note name, injected for each
    // request, see injectRuntimeVars. The name thus lives in Go beside the
    // code that writes the page, and not as a second copy in this file.
    //
    // NOT ON ANDROID. The share sheet receives a note there. This box would
    // be a control that duplicates it, and that cannot be reached from the
    // application that holds the file.
    //
    // The whole block sits inside the "not file:" gate above. An exported
    // page thus never runs it. Such a page never carries the markup either,
    // because the modals slot is filled at serve time.
    (function () {
        function onIncomingPage() {
            return typeof OMN_INCOMING_PAGE !== 'undefined' &&
                typeof PageName !== 'undefined' &&
                PageName === OMN_INCOMING_PAGE;
        }

        function setUp() {
            const box = document.getElementById('omn-incoming');
            if (!box) return;
            if (!onIncomingPage() || (typeof IS_ANDROID !== 'undefined' && IS_ANDROID)) {
                box.remove();
                return;
            }
            const preview = document.getElementById('omn-preview');
            const input = document.getElementById('omn-incoming-files');
            const button = document.getElementById('omn-incoming-import');
            const statusEl = document.getElementById('omn-incoming-status');
            if (!preview || !input || !button || !statusEl) return;

            // Above the list, which is what the page is for. The box is
            // closed until it is wanted, so it costs one line.
            preview.insertBefore(box, preview.firstChild);
            box.classList.remove('omn-hidden');

            const say = function (msg, bad) {
                statusEl.textContent = msg || '';
                statusEl.classList.toggle('omn-is-error', !!bad);
            };

            // OMN_USER_FILE_UPLOADS maps the extension of a JSON file, of a
            // contact and of a calendar to the upload route of its tree. The
            // server injects it, the same as OMN_INCOMING_PAGE.
            const uploads = (typeof OMN_USER_FILE_UPLOADS !== 'undefined') ? OMN_USER_FILE_UPLOADS : {};
            const uploadExts = Object.keys(uploads);
            if (uploadExts.length) input.accept += ',' + uploadExts.join(',');

            // uploadRoute answers the upload route of a file that is not a
            // note, or undefined.
            const uploadRoute = function (file) {
                const name = String(file.name || '').toLowerCase();
                const dot = name.lastIndexOf('.');
                return dot >= 0 ? uploads[name.substring(dot)] : undefined;
            };

            // A file that is not a note goes to its own tree. incoming=1
            // makes the server put a line for it in the list below.
            const uploadOne = async function (file, route) {
                const form = new FormData();
                form.append('file', file, file.name);
                const res = await fetch(route + '?incoming=1', { method: 'POST', body: form });
                if (res.status === 401) {
                    throw new Error('log in as admin to import a file');
                }
                if (!res.ok) {
                    let why = '';
                    try { why = (await res.text()).trim(); } catch (e) { /* no text */ }
                    throw new Error(why || ('HTTP ' + res.status));
                }
            };

            // One note for each request. The rules live in the backend, in
            // internal/exchange/exchange.go. The Android share path reaches
            // that same code. A note thus lands in the same place, whichever
            // way it came.
            const importOne = async function (file) {
                const route = uploadRoute(file);
                if (route) return uploadOne(file, route);
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
                    // The server writes the list, thus the page has to come
                    // again to show what arrived.
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
                    box.classList.add('omn-is-over');
                });
            });
            ['dragleave', 'dragend'].forEach(function (name) {
                box.addEventListener(name, function () { box.classList.remove('omn-is-over'); });
            });
            box.addEventListener('drop', function (e) {
                e.preventDefault();
                box.classList.remove('omn-is-over');
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

    // The actions of the guarded block above. A control that needs the
    // server is hidden on such a page, thus a stub is enough. The last two
    // names are the actions of the lazy files. A page from disk never loads
    // such a file.
    [
        'new-page', 'quick-note-save', 'bookmark-save', 'search',
        'bookmark-panel', 'sync',
    ].forEach(function (name) {
        window.OMN.action(name, function () { printDebug(name); });
    });
    window.checkSession = function() { printDebug('checkSession'); };
    window.omnGoOpenDatabase = function() { printDebug('omnGoOpenDatabase'); };
    window.openDatabase = function() { printDebug('openDatabase'); };
    // The dialogs of omn-go-dialog.js. A page from disk cannot load that
    // file. Each stub gives the answer of a closed dialog: no value for
    // omnAlert, false for omnConfirm, null for omnPrompt.
    window.omnDialog = function() { printDebug('omnDialog'); return Promise.resolve({ value: null, input: null }); };
    window.omnAlert = function() { printDebug('omnAlert'); return Promise.resolve(); };
    window.omnConfirm = function() { printDebug('omnConfirm'); return Promise.resolve(false); };
    window.omnPrompt = function() { printDebug('omnPrompt'); return Promise.resolve(null); };
    // Search needs the server: the panel queries /api/search, which does not
    // exist on a page opened from disk. The header button is .omn-server-only and
    // therefore already hidden here (applyOfflineUI) - these stubs cover a
    // note script or a stale keyboard shortcut calling in anyway.
    // omnSearchClearHighlights is NOT stubbed here: the highlighting lives in
    // omn-go-highlight.js and works offline, so the real one is already defined.
}
