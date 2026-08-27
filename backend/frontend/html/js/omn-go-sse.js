// --- OMN-Go Server Extensions ---
// These modules interact with the Go backend API. They will cleanly bypass themselves
// if the user is merely viewing an exported HTML file locally without the server.

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

    // Maps a backend "[sync] ..." log line to a human-readable stage. First
    // match wins, so more specific prefixes come first. Anything unmatched
    // leaves the current stage alone and only updates the detail line - that
    // way a log message added to git_helper.go later degrades to "still
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
        // syncProgressWriter in git_helper.go).
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
    // sits between the tag and the message, so it is stripped here before
    // any SYNC_STAGES prefix is tried - otherwise every prefix below stops
    // matching. Sync progress is mostly (debug), and this overlay keeps
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

    const SYNC_TITLES = {
        pull: 'Download', pull_ff: 'Download', download: 'Download',
        pull_force: 'Force download', pull_mark: 'Mark conflicts',
        pull_abort: 'Abort pull',
        push: 'Upload', upload: 'Upload', push_force: 'Force upload'
    };

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

            // The overlay is fed by the server's own "[sync]" log lines over
            // the /api/logs stream, so it reports real backend stages rather
            // than a guess. It is torn down before any alert()/modal below,
            // otherwise a blocking dialog would sit on top of a still-
            // spinning bar.
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
                    window.hidePushConflictModal();
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
                case 'push_conflict': {
                    // The remote rejected the push. It is not a
                    // fast-forward update. Keep the failed push's commit
                    // message so a "Force Push" retry can reuse it. Offer
                    // the choice in a modal, like the pull conflict one. A
                    // rejected push leaves local state untouched (see
                    // syncPush). Abort is a pure UI cancel.
                    window._retryPushMessage = opts.message || null;
                    const pModal = document.getElementById('push-conflict-modal');
                    if (pModal) {
                        pModal.classList.remove('hidden');
                    } else {
                        const choice = confirm('Push rejected: the remote has new commits.\n\nOK to Force Push (destructive), Cancel to Abort.');
                        if (choice) window.performPushForce();
                    }
                    return data;
                }
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

        // hidePushConflictModal dismisses the push-rejection modal.
        // A rejected push never touches local state. The backend returns
        // push_conflict before any mutation. The Abort button only hides
        // this modal, like the pull modal's Abort button.
        window.hidePushConflictModal = function() {
            const modal = document.getElementById('push-conflict-modal');
            if (modal) modal.classList.add('hidden');
        };

        // performPushForce handles "Force Push" on the push-rejection modal.
        // It retries the failed push as push_force. It reuses the original
        // commit message when the rejected push had one. A force push with
        // no message asks for one first. The backend requires a commit
        // message for a force push, even when there is nothing new to
        // commit. The message is a checkpoint before a destructive push
        // (see syncPush).
        window.performPushForce = async function() {
            window.hidePushConflictModal();

            let message = window._retryPushMessage || '';
            if (!message) {
                message = window.prompt
                    ? (window.prompt('Force push requires a commit message.\n\nDescribe what this push changes on the remote:') || '').trim()
                    : '';
                if (!message) {
                    alert('Force push cancelled — no commit message.');
                    return;
                }
            }

            const data = await window.runSync('push_force', { message });
            if (data && data.status === 'success') {
                if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                    window.location.reload();
                }
            }
        };

        window.syncAction = async function (action) {
            if (action === 'upload') {
                // Uploads always go through the commit-message modal, which
                // also shows the file list and handles "nothing to commit".
                previewAndCommit();
                return;
            }

            const data = await window.runSync(action);
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
                // Config is now persisted server-side; clear the dirty flag
                // before either reload path below so the save doesn't
                // immediately re-trigger its own "leave site?" prompt.
                if (window.configMarkClean) window.configMarkClean();
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

    window.previewAndCommit = async function() {
        // Building the preview walks the whole worktree diff, which is the
        // slow half of an upload on a large note collection - show progress
        // here too, not just during the commit/push that follows.
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
                // A clean worktree is NOT nothing to upload. A commit whose
                // push failed, or a git profile switched after a successful
                // push, leaves commits the active remote has never seen -
                // and this branch used to end the upload right here, with no
                // way to retry them short of making a new change.
                //
                // There is nothing to commit, so no commit message is asked
                // for: the upload goes straight to the push.
                if (preview.unpushed) {
                    const data = await window.runSync('upload');
                    if (data && data.status === 'success') {
                        if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                            window.location.reload();
                        }
                    }
                    return;
                }
                // Only now is "nothing to do" an honest thing to say - and it
                // says which remote it is true OF, because with several
                // profiles configured that is the part that matters.
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
        hideCommitModal();

        const data = await window.runSync('upload', { message });
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
    //
    // Every log.Printf in the backend already reaches this stream (see
    // logger.go), which is why the sync progress overlay needs no transport
    // of its own: git_helper.go's "[sync] ..." lines are the progress feed.
    // Subscribers registered through window.omnGoOnServerLog get each line in
    // addition to the console mirroring that has always happened here.
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
    //
    // The everyday entry point to search: a spotlight-style panel over the
    // current page. It has two scopes and one control to pick between them:
    //
    //   page - the open note only. No index, no configuration, always
    //          available, so this panel works on any device and in any state
    //          of the app: there is nothing to switch on first.
    //   all  - every indexed note. Offered only when the server says it can
    //          answer (OMN_SEARCH_GLOBAL), so the control never leads nowhere.
    //
    // It lives in this file rather than a new asset for two reasons: this file
    // is already inside the `protocol !== 'file:'` guard, so an exported page
    // gets the stub version for free; and it is already in
    // versionDependentAssets + gitignorePatterns, so no new plumbing is needed
    // to ship it.
    //
    // Everything the server returns is written with textContent (or into a
    // <mark> element's textContent). Nothing from a response is ever assigned
    // to innerHTML - the same discipline OMNProgress.build documents in
    // omn-go-core.js, and it matters more here because the text being rendered
    // is the user's own notes.
    (function () {
        // THE SEARCH IS ASKED FOR, NOT GUESSED AT.
        //
        // Typing does not search. The magnifier button does, and so does
        // Enter. A query of all notes reads every note the index holds, and a
        // search per keystroke did that work five or six times for one word
        // and kept only the last answer. A timer instead of a button only
        // moves the guess: too short and it fires mid-word, too long and the
        // panel looks broken.
        //
        // The button is the same control the results page carries, so "type,
        // then press the magnifier" is one habit for both.
        var MIN_QUERY = 2;       // 1 rune matches nearly everything
        var MAX_SNIPPETS = 10;   // the API's own cap

        var overlay = null, input = null, list = null, statusEl = null, scopeEl = null;
        var progressEl = null, goEl = null;
        var seeAllEl = null;   // lives in the scope row - see renderScope
        var rows = [];
        var active = -1;
        // The query the rows on screen belong to. Enter opens a row while the
        // field still says that; once the field says something else, Enter is
        // a request to search for the new thing instead - see onInputKey.
        var lastQuery = null;
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
                    // The one control that searches. It stands where the
                    // submit button of the results page stands - after the
                    // field, same glyph - so the two behave alike. The
                    // decorative magnifier that used to lead this row is
                    // gone: two of them, one inert, said the wrong thing
                    // about which one to press.
                    '<button type="button" class="omn-search-go" ' +
                            'title="Search (↵)" aria-label="Search">' +
                      '<i class="material-icons icon-sm">search</i>' +
                    '</button>' +
                    '<button type="button" class="omn-search-close" aria-label="Close">' +
                      '<i class="material-icons icon-sm">close</i>' +
                    '</button>' +
                  '</div>' +
                  // The wait, on the line under the field that causes it. It
                  // reuses the shared .omn-progress-track / -fill look from
                  // omn-go-core.css, so a wait is the same object here as in
                  // the sync overlay; only the placement is local.
                  '<div class="omn-search-progress omn-progress-track" ' +
                       'role="progressbar" aria-label="Search progress" hidden>' +
                    '<div class="omn-progress-fill"></div>' +
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
            progressEl = overlay.querySelector('.omn-search-progress');
            goEl = overlay.querySelector('.omn-search-go');

            goEl.addEventListener('click', function () {
                // Back to the field afterwards: on a phone the tap on this
                // button closes the keyboard, and the next thing a reader
                // does is usually edit the query.
                run();
                focusInput();
            });
            overlay.querySelector('.omn-search-close').addEventListener('click', close);
            // A click on the backdrop closes; a click inside the card must not.
            overlay.addEventListener('click', function (e) {
                if (e.target === overlay) close();
            });
            input.addEventListener('input', onInput);
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
            // The results page searches every note and renders the answer
            // server-side, so this wait is in the NAVIGATION, after this
            // document is gone. The slow-navigation guard in omn-go-core.js
            // does not cover it: that one watches <a> clicks and this is an
            // assignment to location.
            //
            // Armed, not shown - the same 300 ms the guard uses, and for the
            // same reason: a fast answer must not flash an overlay on the way
            // past. The timer dies with the document if the page arrives
            // first, so there is nothing to take down.
            if (window.OMNProgress) {
                setTimeout(function () {
                    window.OMNProgress.show('Searching');
                    window.OMNProgress.stage('Reading all notes…');
                    window.OMNProgress.detail(q);
                }, 300);
            }
            window.location.href = '/OMNGoSearch.html?q=' + encodeURIComponent(q);
        }

        // Switching to "All notes" is the slowest thing the dialog does and
        // the one that most needs to say so: run() raises the bar before the
        // request leaves.
        function setScope(next) {
            if (scope === next && scopeShown === next) return;
            scope = next;
            scopeShown = next;
            renderScope();
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
            showProgress(false);
            if (inflight) {
                inflight.abort();
                inflight = null;
            }
            if (overlay) overlay.hidden = true;
        }

        function isOpen() {
            return !!overlay && !overlay.hidden;
        }

        // showProgress raises and lowers the bar under the input.
        //
        // One wait, and its length is NOT known: it depends on how many notes
        // the index holds, which is why "All notes" needed this most. So the
        // bar is the indeterminate sweep - it says "working" and promises no
        // time. It is never left on screen empty, which would read as a wait
        // that is making no progress.
        function showProgress(on) {
            if (!progressEl) return;
            progressEl.hidden = !on;
            progressEl.classList.toggle('indeterminate', !!on);
        }

        // Typing changes no results. It only keeps the two things that
        // describe the field honest: whether "See all results" applies, and
        // whether the rows below still answer what the field says.
        function onInput() {
            updateSeeAll();
            var q = input.value.trim();
            if (q.length < MIN_QUERY) {
                setStatus(q ? 'Type at least ' + MIN_QUERY + ' characters' : '');
                return;
            }
            if (q !== lastQuery) {
                // Without this the panel answers a new query with the old
                // one's results and says nothing about it. It also states
                // what to press, which is the whole of the interaction.
                setStatus('Press ↵ or the magnifier to search');
            }
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
                showProgress(false);
                clearRows();
                lastQuery = null;
                setStatus(q ? 'Type at least ' + MIN_QUERY + ' characters' : '');
                return;
            }

            // The rows about to arrive answer THIS text. onInputKey compares
            // against it to decide what Enter means, so it is set here, where
            // the request is made, and not where the answer lands - a reply
            // that never comes must not leave Enter opening rows that belong
            // to a query the field no longer shows.
            lastQuery = q;

            // From here a request is going out, and how long it takes is the
            // server's business - an index of every note answers slower than
            // one page. This is the wait that "All notes" makes visible.
            showProgress(true);
            setStatus('Searching…');

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
                    // After the superseded check, never before it: a newer
                    // request is still running and its bar has to stay up.
                    showProgress(false);
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
                    // An abort is this dialog's own doing (a newer query, or
                    // close) and the newer owner is showing its own bar.
                    if (err && err.name === 'AbortError') return;
                    if (ctrl && inflight !== ctrl) return;
                    inflight = null;
                    showProgress(false);
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

                // Each row is one LINE, so each row opens the document AT that
                // line. Passing only r sent every row of a result to the same
                // place - the first match in the note - whichever line the
                // reader chose. The heading row above keeps that behaviour,
                // because it names the document and no line in it.
                (r.matches || []).forEach(function (m) {
                    list.appendChild(buildSnippetRow(m, function () { openResult(r, m); }));
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
        // m is the line the reader chose, and is absent when the document
        // itself was chosen.
        function openResult(r, m) {
            close();
            if (r && r.url) window.location.href = withHighlight(r.url, m);
        }

        // withHighlight hangs the query terms off a URL as ?hl=, so the page
        // being opened marks them on arrival. With a line it also hangs that
        // line's text off as ?hlt=, which is what the page goes TO: a result
        // lists each matching line, and every one of them opening the first
        // match in the note is only right for the first. Same two parameters
        // the results page puts on its links (highlightURL and snippetURL in
        // search.go), so a result behaves identically whichever list it came
        // from. The receiving page strips them from the address bar once
        // applied - see omn-go-core.js.
        function withHighlight(url, m) {
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
            // url already ends in the BEST hit's section; this line may be in
            // another one, and the fragment is what the page falls back to
            // when it cannot find the text.
            if (m && m.section && m.section.id) frag = '#' + m.section.id;

            var sep = url.indexOf('?') === -1 ? '?' : '&';
            for (var i = 0; i < lastTerms.length; i++) {
                url += sep + 'hl=' + encodeURIComponent(lastTerms[i]);
                sep = '&';
            }
            // A hit inside a <script> block gets no ?hlt=: the text is indexed
            // but never rendered, so there is no word on the page to go to and
            // a search for it would only find a coincidence. Without terms
            // there is nothing marked, so there is nothing to go to either.
            if (lastTerms.length && m && m.text && m.context !== 'script') {
                url += sep + 'hlt=' + encodeURIComponent(m.text);
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
                // Enter is the keyboard's magnifier. It searches whenever the
                // field says something the rows on screen do not answer -
                // which, now that typing searches nothing, is every moment
                // between typing and asking. Only when the two agree does
                // Enter mean what it used to: open the row I am on.
                if (input.value.trim() !== lastQuery) {
                    run();
                    break;
                }
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
