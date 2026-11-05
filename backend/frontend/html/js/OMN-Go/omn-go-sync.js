// --- The sync controls ---
//
// Everything behind the download and upload buttons. That is the one call
// to /api/sync, the conflict modal, the force push, and the commit-message
// modal that an upload opens first.
//
// THIS FILE ARRIVES ON DEMAND. omn-go-api.js gives each action at the end
// of this file a stub, and the first press of a sync control loads this
// file. See omnLazyActions in that file, and
// doc/decisions/0015-load-the-click-driven-scripts-on-demand.md.
//
// The actions must match the omnLazyActions list there. An action that this
// file does not give writes a console fault at the first press.
//
// A page from disk never loads this file. The else branch of omn-go-api.js
// gives each action a stub there.
if (window.location.protocol !== 'file:') {

    // The title of the progress overlay, for each action that runSync
    // takes.
    //
    // IT MUST BE IN THIS FILE. The body of omn-go-api.js sits inside an if
    // block, thus a const of that block reaches no other file. A map
    // there would make each press of "Commit & Push" throw "SYNC_TITLES is
    // not defined".
    //
    // A name that this file reads belongs in this file. lazy.test.js runs
    // each exported function of each lazy file and fails on a free
    // variable.
    const SYNC_TITLES = {
        pull: 'Download', pull_ff: 'Download', download: 'Download',
        pull_force: 'Force download', pull_mark: 'Mark conflicts',
        pull_abort: 'Abort pull',
        push: 'Upload', upload: 'Upload', push_force: 'Force upload'
    };

    // The whole body of this file sits inside the protocol guard. A const
    // here thus reaches no other file. No function of this file is on
    // window. Each function is a const, and NOT a function declaration: a
    // declaration in a block becomes a global name in sloppy mode.

    // The commit message of a push that the remote refused. The force push
    // of the push-conflict modal sends it again.
    let retryPushMessage = null;

    // runSync is the one place that talks to /api/sync. It always POSTs
    // action, force and message together, and it always expects a JSON
    // {status, message} response.
    //
    // Both syncAction and the conflict modal handler, which is performSync
    // below, go through this one function. The two thus cannot drift apart.
    const runSync = async function (action, opts) {
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
        const unsubscribe = window.omnGoOnServerLog(window.applySyncLogLine);
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

        const modal = document.getElementById('omn-conflict-modal');
        switch (data.status) {
            case 'success':
                if (modal) modal.classList.add('omn-hidden');
                hidePushConflictModal();
                return data;
            case 'conflict':
                if (modal) {
                    populateConflictFiles(data.files);
                    modal.classList.remove('omn-hidden');
                } else {
                    const list = (data.files && data.files.length)
                        ? '\n\nFiles in contention:\n' + data.files.join('\n') : '';
                    const choice = confirm('Conflict!' + list + '\n\nOK to Force Pull (Keep Untracked), Cancel to Mark Files.');
                    if (choice) runSync('pull_force');
                    else runSync('pull_mark');
                }
                return data;
            case 'push_conflict': {
                // The remote rejected the push. It is not a
                // fast-forward update. Keep the failed push's commit
                // message so a "Force Push" retry can reuse it. Offer
                // the choice in a modal, like the pull conflict one. A
                // rejected push leaves local state untouched (see
                // syncPush). Abort is a pure UI cancel.
                retryPushMessage = opts.message || null;
                const pModal = document.getElementById('omn-push-conflict-modal');
                if (pModal) {
                    pModal.classList.remove('omn-hidden');
                } else {
                    const choice = confirm('Push rejected: the remote has new commits.\n\nOK to Force Push (destructive), Cancel to Abort.');
                    if (choice) performPushForce();
                }
                return data;
            }
            case 'needs_commit_message':
                alert('Please provide a commit message.');
                return data;
            case 'host_key_changed':
                return trustHostKey(data, action, opts);
            default:
                alert('Sync failed: ' + (data.message || 'unknown error'));
                return data;
        }
    };

    // trustHostKey shows the two fingerprints of a changed server key. OK
    // stores the new key and runs the same action again. The server stores
    // the key only when the fingerprint is the one that the person saw. See
    // doc/decisions/0019-trust-the-host-key-on-first-use.md.
    async function trustHostKey(data, action, opts) {
        const ok = confirm('The key of the git server changed.\n\n' +
            'Server: ' + data.host + '\n' +
            'Known key: ' + data.known + '\n' +
            'New key: ' + data.fingerprint + '\n\n' +
            'A changed key can mean that another machine answers for the server. ' +
            'Compare the new key with the key that the owner of the server gives you.\n\n' +
            'OK trusts the new key and tries again. Cancel stops the sync.');
        if (!ok) return data;
        const fd = new URLSearchParams({ host: data.host, fingerprint: data.fingerprint });
        const res = await fetch('/api/sync/trust-host-key', { method: 'POST', body: fd });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            alert('The server did not store the new key: ' + (err.message || res.status));
            return data;
        }
        return runSync(action, opts);
    }

    // populateConflictFiles fills the file list of the conflict modal. The
    // files are the ones that the backend reported as in contention, which
    // are the ones that "Mark Conflicts" would fill with markers.
    //
    // An empty list means that the histories diverged with no per-file
    // overlap, which is a clean local tree with its own commits. Force Pull
    // is then the meaningful choice, thus the modal says so and does not
    // show an empty box.
    //
    // Built with textContent, and never with innerHTML, thus a note filename
    // cannot inject markup.
    const populateConflictFiles = function (files) {
        const box = document.getElementById('omn-conflict-files');
        const list = document.getElementById('omn-conflict-file-list');
        if (!box || !list) return;
        list.textContent = '';
        const arr = Array.isArray(files) ? files : [];
        if (arr.length === 0) {
            const li = document.createElement('li');
            li.className = 'omn-conflict-files-none';
            li.textContent = 'No individual file conflicts — histories diverged; Force Pull is recommended.';
            list.appendChild(li);
        } else {
            arr.forEach(function(name) {
                const li = document.createElement('li');
                li.textContent = name;
                list.appendChild(li);
            });
        }
        box.classList.remove('omn-hidden');
    };

    // performSync handles the three buttons on the conflict modal in
    // index.html. It moved here from an inline <script> in that file, thus
    // all sync UI logic lives together. It uses runSync
    // above, thus the modal and the header sync buttons cannot disagree
    // about the wire format or the response handling.
    const performSync = async function (action) {
        const modal = document.getElementById('omn-conflict-modal');
        if (action === 'abort') {
            // A plain "pull" never changes local state before it reports a
            // conflict. To abort here is thus a UI cancel alone, and there
            // is nothing on the server to undo.
            if (modal) modal.classList.add('omn-hidden');
            return;
        }
        if (modal) modal.classList.add('omn-hidden');

        const data = await runSync(action);
        if (data && data.status === 'success') {
            // pull_force and pull_mark both change what is on disk under
            // this page, thus reload to show it.
            location.reload();
        }
    };

    // hidePushConflictModal dismisses the push-rejection modal.
    // A rejected push never touches local state. The backend returns
    // push_conflict before any mutation. The Abort button only hides
    // this modal, like the pull modal's Abort button.
    const hidePushConflictModal = function () {
        const modal = document.getElementById('omn-push-conflict-modal');
        if (modal) modal.classList.add('omn-hidden');
    };

    // performPushForce handles "Force Push" on the push-rejection modal.
    // It retries the failed push as push_force. It reuses the original
    // commit message when the rejected push had one. A force push with
    // no message asks for one first. The backend requires a commit
    // message for a force push, even when there is nothing new to
    // commit. The message is a checkpoint before a destructive push
    // (see syncPush).
    const performPushForce = async function () {
        hidePushConflictModal();

        let message = retryPushMessage || '';
        if (!message) {
            message = window.prompt
                ? (window.prompt('Force push requires a commit message.\n\nDescribe what this push changes on the remote:') || '').trim()
                : '';
            if (!message) {
                alert('Force push canceled — no commit message.');
                return;
            }
        }

        const data = await runSync('push_force', { message });
        if (data && data.status === 'success') {
            if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                window.location.reload();
            }
        }
    };

    const syncAction = async function (action) {
        if (action === 'upload') {
            // Uploads always go through the commit-message modal, which
            // also shows the file list and handles "nothing to commit".
            previewAndCommit();
            return;
        }

        const data = await runSync(action);
        if (data && data.status === 'success') {
            if (confirm('Sync complete.\n\nWould you like to reload the page now to see updated content?')) {
                window.location.reload();
            }
        }
    };

    const previewAndCommit = async function () {
        // To build the preview walks the whole worktree diff. That is the
        // slow half of an upload on a large note collection. Show progress
        // here too, and not during the commit and push alone.
        let res, preview, err = null;
        window.OMNProgress.show('Upload');
        window.OMNProgress.stage('Collecting pending changes…');
        const unsubscribe = window.omnGoOnServerLog(window.applySyncLogLine);
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
                // push failed leaves commits that the active remote has
                // never seen, and so does a git profile switched after a
                // successful push. This branch used to end the upload right
                // here. There was then no way to retry them, short of a new
                // change.
                //
                // There is nothing to commit, so no commit message is asked
                // for: the upload goes straight to the push.
                if (preview.unpushed) {
                    const data = await runSync('upload');
                    if (data && data.status === 'success') {
                        if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                            window.location.reload();
                        }
                    }
                    return;
                }
                // Only now is "nothing to do" an honest thing to say. It also
                // says which remote it is true OF. With several profiles
                // configured, that is the part that matters.
                var where = preview.remote ? ' on ' + preview.remote : '';
                if (preview.remote_error) {
                    alert('Nothing to commit.\n\nCould not reach the remote' + where +
                          ' to check for unpushed commits:\n' + preview.remote_error);
                } else {
                    alert('Nothing to commit, and nothing to push' + where + '.');
                }
                return;
            }

            var listEl = document.getElementById('omn-commit-file-list');
            if (listEl) listEl.textContent = files.join('\n');
            document.getElementById('omn-commit-modal').style.display = 'flex';
        } catch(e) {
            alert('Error: ' + e);
        }
    };

    const commitAndUpload = async function () {
        const message = document.getElementById('omn-commit-message').value.trim();
        if (!message) {
            alert('Please enter a commit message.');
            return;
        }
        hideCommitModal();

        const data = await runSync('upload', { message });
        if (data && data.status === 'success') {
            if (confirm('Upload complete.\n\nWould you like to reload the page now to see updated content?')) {
                window.location.reload();
            }
        }
    };

    const hideCommitModal = function () {
        document.getElementById('omn-commit-modal').style.display = 'none';
        document.getElementById('omn-commit-message').value = '';
    };

    // The actions of the sync controls. See OMN.action in omn-go-core.js.
    // data-arg of sync is "download" or "upload". data-arg of sync-resolve
    // is "pull_force", "pull_mark" or "abort".
    window.OMN.action('sync', function (el) { return syncAction(el.getAttribute('data-arg')); });
    window.OMN.action('sync-resolve', function (el) { return performSync(el.getAttribute('data-arg')); });
    window.OMN.action('push-force', function () { return performPushForce(); });
    window.OMN.action('push-cancel', function () { hidePushConflictModal(); });
    window.OMN.action('commit-upload', function () { return commitAndUpload(); });
    window.OMN.action('commit-cancel', function () { hideCommitModal(); });
}
