// --- The sync controls ---
//
// Everything behind the download and upload buttons. That is the one call
// to /api/sync, the conflict dialog, the force push, and the commit dialog
// that an upload opens first. Each dialog comes from omnDialog of
// omn-go-dialog.js.
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

    // runSync is the one place that talks to /api/sync. It always POSTs
    // action, force and message together, and it always expects a JSON
    // {status, message} response.
    //
    // The sync buttons and each choice of a dialog below go through this
    // one function. They thus cannot drift apart.
    const runSync = async function (action, opts) {
        opts = opts || {};
        const fd = new URLSearchParams();
        fd.append('action', action);
        if (opts.force) fd.append('force', 'true');
        if (opts.message) fd.append('message', opts.message);

        // The overlay is fed by the server's own "[sync]" log lines over
        // the /api/logs stream, so it reports real backend stages rather
        // than a guess. The code removes it before each dialog below. A
        // dialog on top of a bar that still turns gives a wrong message.
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
            await window.omnAlert('Sync error: ' + netErr);
            return null;
        }

        switch (data.status) {
            case 'success':
                return data;
            case 'conflict':
                await resolveConflict(await askConflict(data.files));
                return data;
            case 'push_conflict':
                // The remote rejected the push. It is not a fast-forward
                // update. A rejected push leaves the local state as it was
                // (see syncPush), thus Abort only closes the dialog. The
                // force push sends the commit message of the rejected push
                // again.
                if (await askPushConflict()) await performPushForce(opts.message || '');
                return data;
            case 'needs_commit_message':
                await window.omnAlert('Give a commit message.');
                return data;
            case 'host_key_changed':
                return trustHostKey(data, action, opts);
            default:
                await window.omnAlert('Sync failed: ' + (data.message || 'unknown error'));
                return data;
        }
    };

    // trustHostKey shows the two fingerprints of a changed server key. OK
    // stores the new key and runs the same action again. The server stores
    // the key only when the fingerprint is the one that the person saw. See
    // doc/decisions/0019-trust-the-host-key-on-first-use.md.
    async function trustHostKey(data, action, opts) {
        const ok = await window.omnConfirm('The key of the git server changed.\n\n' +
            'Server: ' + data.host + '\n' +
            'Known key: ' + data.known + '\n' +
            'New key: ' + data.fingerprint + '\n\n' +
            'A changed key can mean that another machine answers for the server. ' +
            'Compare the new key with the key that the owner of the server gives you.\n\n' +
            'Trust the new key to try again. Cancel stops the sync.', 'Trust the new key');
        if (!ok) return data;
        const fd = new URLSearchParams({ host: data.host, fingerprint: data.fingerprint });
        const res = await fetch('/api/sync/trust-host-key', { method: 'POST', body: fd });
        if (!res.ok) {
            const err = await res.json().catch(() => ({}));
            await window.omnAlert('The server did not store the new key: ' + (err.message || res.status));
            return data;
        }
        return runSync(action, opts);
    }

    // askConflict shows the files that the server reported in contention.
    // These are the files that "Mark Conflicts" fills with markers. It
    // answers "pull_force", "pull_mark" or "abort".
    //
    // An empty list means that the histories went apart with no conflict
    // in a file: a clean local tree with its own commits. Force Pull is
    // then the useful choice, and the dialog says so.
    //
    // Abort is also the default button. Thus Enter never starts a
    // destructive action.
    const askConflict = async function (files) {
        const list = Array.isArray(files) ? files : [];
        const content = list.length
            ? [{ p: 'Files in contention:' }, { list: list }]
            : [{ p: 'No file has a conflict of its own. The histories went apart. Force Pull is the recommended choice.' }];
        const answer = await window.omnDialog({
            kind: 'conflict',
            title: 'Merge Conflict',
            text: 'Local changes conflict with the remote repository. Choose how to resolve this:',
            content: content,
            buttons: [
                {
                    label: 'Force Pull (Reset to Remote)', value: 'pull_force', danger: true,
                    hint: 'Overwrites the tracked files. Also deletes the local files that are not tracked and not in .gitignore.',
                },
                {
                    label: 'Mark Conflicts in Files', value: 'pull_mark',
                    hint: 'Puts <<< ==== >>> conflict markers into the files for a manual fix.',
                },
                { label: 'Abort', value: 'abort', cancel: true, default: true },
            ],
        });
        return answer.value;
    };

    // resolveConflict runs the choice of the conflict dialog. A plain pull
    // never changes the local state before it reports a conflict. Abort
    // thus sends nothing, and there is nothing on the server to undo.
    const resolveConflict = async function (choice) {
        if (choice !== 'pull_force' && choice !== 'pull_mark') return;
        const data = await runSync(choice);
        if (data && data.status === 'success') {
            // Both choices change the files under this page, thus the page
            // loads again to show them.
            location.reload();
        }
    };

    // askPushConflict asks what to do after the remote refused a push. It
    // answers true for Force Push. Abort is the default button.
    const askPushConflict = async function () {
        const answer = await window.omnDialog({
            kind: 'push-conflict',
            title: 'Push Rejected',
            text: 'The remote has newer commits and rejected the push. Download the remote changes first, or overwrite the remote:',
            buttons: [
                {
                    label: 'Force Push', value: 'force', danger: true,
                    hint: 'Overwrites the history on the remote with the history of this device. The commits that only the remote has are lost.',
                },
                { label: 'Abort', value: 'abort', cancel: true, default: true },
            ],
        });
        return answer.value === 'force';
    };

    // askCommitMessage asks for a commit message in a textarea. It answers
    // the message without the outer spaces, or null for Cancel. The dialog
    // does not close with an empty message. opts holds kind, title, text,
    // content and okLabel.
    const askCommitMessage = async function (opts) {
        const answer = await window.omnDialog({
            kind: opts.kind,
            title: opts.title,
            text: opts.text,
            content: opts.content,
            width: '500px',
            fields: [{ type: 'textarea', rows: 4, placeholder: 'Enter a commit message' }],
            check: function (a) { return a.input.trim() ? '' : 'Enter a commit message.'; },
            buttons: [
                { label: opts.okLabel, value: true, default: true },
                { label: 'Cancel', value: false, cancel: true },
            ],
        });
        return answer.value === true ? answer.input.trim() : null;
    };

    // askReload asks to load the page again after a sync that changed the
    // files. text names the sync, for example "Upload complete.".
    const askReload = async function (text) {
        if (await window.omnConfirm(text + '\n\nLoad the page again to see the new content?', 'Load again', 'Keep')) {
            window.location.reload();
        }
    };

    // performPushForce sends the rejected push again as push_force. The
    // backend needs a commit message for a force push, also when there is
    // nothing new to commit. The message marks the point before a
    // destructive push (see syncPush). message is the message of the
    // rejected push. Without it, the dialog asks for one.
    const performPushForce = async function (message) {
        if (!message) {
            message = await askCommitMessage({
                kind: 'force-push-message',
                title: 'Force Push',
                text: 'A force push needs a commit message. Describe what this push changes on the remote:',
                okLabel: 'Force Push',
            });
            if (!message) return;
        }
        const data = await runSync('push_force', { message: message });
        if (data && data.status === 'success') await askReload('Upload complete.');
    };

    const syncAction = async function (action) {
        if (action === 'upload') {
            // An upload always starts with the preview. It shows the file
            // list in the commit dialog, and it handles "nothing to commit".
            await previewAndCommit();
            return;
        }

        const data = await runSync(action);
        if (data && data.status === 'success') await askReload('Sync complete.');
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
                await window.omnAlert('Error: ' + err);
                return;
            }
            if (!res.ok) {
                await window.omnAlert('Could not get the pending changes.');
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
                    if (data && data.status === 'success') await askReload('Upload complete.');
                    return;
                }
                // Only now is "nothing to do" an honest thing to say. It also
                // says which remote it is true OF. With several profiles
                // configured, that is the part that matters.
                var where = preview.remote ? ' on ' + preview.remote : '';
                if (preview.remote_error) {
                    await window.omnAlert('Nothing to commit.\n\nCould not reach the remote' + where +
                          ' to check for unpushed commits:\n' + preview.remote_error);
                } else {
                    await window.omnAlert('Nothing to commit, and nothing to push' + where + '.');
                }
                return;
            }

            const message = await askCommitMessage({
                kind: 'commit',
                title: 'Commit Changes',
                content: [{ p: 'Files to commit:' }, { list: files }],
                okLabel: 'Commit & Push',
            });
            if (!message) return;
            const data = await runSync('upload', { message: message });
            if (data && data.status === 'success') await askReload('Upload complete.');
        } catch(e) {
            await window.omnAlert('Error: ' + e);
        }
    };

    // The action of the sync controls. See OMN.action in omn-go-core.js.
    // data-arg is "download" or "upload". The dialogs of this file need no
    // action: omnDialog answers the button.
    window.OMN.action('sync', function (el) { return syncAction(el.getAttribute('data-arg')); });
}
