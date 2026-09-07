// --- The sync controls ---
//
// Everything behind the download and upload buttons. That is the one call
// to /api/sync, the conflict modal, the force push, and the commit-message
// modal that an upload opens first.
//
// THIS FILE ARRIVES ON DEMAND. It was part of omn-go-sse.js until
// 26.09.24, thus every note page parsed it to run nothing. omn-go-sse.js
// writes a stub for each name below, and the first press of a sync button
// loads this file. See omnLazy in that file.
//
// The names must match the omnLazy list there. A name that this file does
// not define writes a console fault at the first press.
if (window.location.protocol !== 'file:') {

    // The title of the progress overlay, for each action that runSync
    // takes.
    //
    // IT LIVED IN omn-go-sse.js UNTIL 26.09.41, AND THAT BROKE THE UPLOAD.
    // F3 moved the sync code into this file in 26.09.24 and left the map
    // behind. The body of omn-go-sse.js sits inside an if block, thus a
    // const of that block reaches no other file. Each press of
    // "Commit & Push" then threw "SYNC_TITLES is not defined" and the
    // button did nothing.
    //
    // A name that this file reads belongs in this file. See
    // TestLazyFilesDefineWhatTheyRead, which runs each exported function
    // of each lazy file and fails on a free variable.
    const SYNC_TITLES = {
        pull: 'Download', pull_ff: 'Download', download: 'Download',
        pull_force: 'Force download', pull_mark: 'Mark conflicts',
        pull_abort: 'Abort pull',
        push: 'Upload', upload: 'Upload', push_force: 'Force upload'
    };

    // populateConflictFiles and the functions below were inside an
    // IIFE named Logger until 26.09.42. The name was wrong: the block
    // held the sync controls and no logger. Its return value went into
    // a const that nothing read.
    //
    // The whole body of this file already sits inside the protocol
    // guard, thus a name here reaches no other file unless it goes on
    // window. The wrapper added a scope and hid nothing.

    // runSync is the one place that talks to /api/sync. It always POSTs
    // action, force and message together, and it always expects a JSON
    // {status, message} response.
    //
    // The backend previously read "action" from the URL query string
    // alone, and this file posted it in the body. The action was thus
    // silently ignored, and every request fell back to a plain "pull".
    //
    // Both syncAction and the conflict modal handler, which is performSync
    // below, go through this one function. The two thus cannot drift apart.
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
    // index.html. It moved here from an inline <script> in that file, thus
    // all sync UI logic lives together. It goes through window.runSync
    // above, thus the modal and the header sync buttons cannot disagree
    // about the wire format or the response handling.
    window.performSync = async function(action) {
        const modal = document.getElementById('conflict-modal');
        if (action === 'abort') {
            // A plain "pull" never changes local state before it reports a
            // conflict. To abort here is thus a UI cancel alone, and there
            // is nothing on the server to undo.
            if (modal) modal.classList.add('hidden');
            return;
        }
        if (modal) modal.classList.add('hidden');

        const data = await window.runSync(action);
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

    window.previewAndCommit = async function() {
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
                    const data = await window.runSync('upload');
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

    // NOTE: In-page editing was removed. To edit a note now opens the
    // dedicated editor page. That is any URL with ?edit=true, served by the
    // Go backend and driven by omn-go-editor.js. It fetches the source from
    // /api/note itself. The view page therefore embeds no #editor textarea
    // any more. The old toggleMode, loadNoteIntoEditor, setupEditorDragDrop
    // and saveNote helpers that manipulated it are gone.

} else {
    window.runSync = function() { printDebug('runSync'); };
    window.syncAction = function() { printDebug('syncAction'); };
    window.performSync = function() { printDebug('performSync'); };
    window.performPushForce = function() { printDebug('performPushForce'); };
    window.hidePushConflictModal = function() { printDebug('hidePushConflictModal'); };
    window.previewAndCommit = function() { printDebug('previewAndCommit'); };
    window.commitAndUpload = function() { printDebug('commitAndUpload'); };
    window.hideCommitModal = function() { printDebug('hideCommitModal'); };
}
