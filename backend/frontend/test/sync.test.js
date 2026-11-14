// The sync controls, in the REAL JavaScript and on the markup of the
// templates.
//
// Each test loads the scripts of index.html and presses a control of the
// page. It then reads the requests that reached the server, and what the
// page shows. The server is a function of the test.
//
// WHAT THESE TESTS PROTECT. A sync writes to the notes of a person and to a
// remote repository. A control that sends the wrong action can overwrite
// one of the two. Three rules matter most:
//
//   - Abort and Cancel send NOTHING.
//   - Each force push has a commit message.
//   - A new host key is stored only after the person said OK.
//
// A real sync needs a git server, and no test here has one. The Go tests of
// internal/gitsync hold the server half.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');

const DOWNLOAD = '[data-action="sync"][data-arg="download"]';
const UPLOAD = '[data-action="sync"][data-arg="upload"]';

// syncPosts answers the body of each POST to /api/sync, in order.
function syncPosts(h) {
    return h.requests
        .filter(function (r) { return r.url === '/api/sync' && r.method === 'POST'; })
        .map(function (r) { return r.body; });
}

// serverWith answers a server function. answers maps the start of a URL to
// its answer, and a function answer gets the request.
function serverWith(answers) {
    return function (request) {
        for (const prefix of Object.keys(answers)) {
            if (request.url.indexOf(prefix) === 0) {
                const a = answers[prefix];
                return typeof a === 'function' ? a(request) : a;
            }
        }
        return { status: 'success' };
    };
}

test('the first press of a sync control loads omn-go-sync.js one time', async () => {
    const h = notePage();
    assert.deepStrictEqual(h.loads, [], 'a note page loads no lazy file before a press');
    h.press(DOWNLOAD);
    await h.settle();
    h.press(DOWNLOAD);
    await h.settle();
    // The question after the download loads the dialog file, one time too.
    assert.deepStrictEqual(h.loads, ['/js/OMN-Go/omn-go-sync.js', '/js/OMN-Go/omn-go-dialog.js']);
    assert.deepStrictEqual(syncPosts(h), ['action=download', 'action=download']);
});

test('a download that succeeds asks before it loads the page again', async () => {
    const h = notePage();
    h.confirm = function () { return true; };
    h.press(DOWNLOAD);
    await h.settle();
    assert.deepStrictEqual(syncPosts(h), ['action=download']);
    assert.strictEqual(h.dialogs.length, 1);
    assert.strictEqual(h.dialogs[0].kind, 'confirm');
    assert.strictEqual(h.page.location.reloads, 1);

    // The person can keep the page as it is.
    const stay = notePage();
    stay.press(DOWNLOAD);
    await stay.settle();
    assert.strictEqual(stay.page.location.reloads, 0);
});

test('the progress overlay shows the stage of the server and closes at the end', async () => {
    const h = notePage();
    let release;
    h.server = serverWith({
        '/api/sync': function () {
            return new Promise(function (resolve) { release = resolve; });
        },
    });
    h.press(DOWNLOAD);
    await h.settle();
    const overlay = h.$('.omn-progress-overlay');
    assert.strictEqual(overlay.hidden, false, 'the overlay is not open while the server works');
    assert.strictEqual(h.$('.omn-progress-title').textContent, 'Download');

    // A line of the log stream moves the stage. The level word is not a
    // part of the text that the person reads.
    h.serverLog('2026/10/02 10:00:00 [sync] (debug) Committing 3 files');
    assert.strictEqual(h.$('.omn-progress-stage').textContent, 'Committing…');
    assert.strictEqual(h.$('.omn-progress-detail').textContent, 'Committing 3 files');
    // A line of another part of the server changes nothing.
    h.serverLog('2026/10/02 10:00:01 [search] (info) index ready');
    assert.strictEqual(h.$('.omn-progress-detail').textContent, 'Committing 3 files');

    release({ status: 'success' });
    await h.settle();
    assert.strictEqual(overlay.hidden, true, 'the overlay stays open after the answer');

    // A later line must not open or change the overlay: the subscription
    // ended with the request.
    h.serverLog('2026/10/02 10:00:02 [sync] (debug) Staging');
    assert.strictEqual(h.$('.omn-progress-detail').textContent, 'Committing 3 files');
});

test('a conflict opens the modal with the files, and each button does its work', async () => {
    const h = notePage();
    h.server = serverWith({
        '/api/sync': function (r) {
            return r.body === 'action=download'
                ? { status: 'conflict', files: ['md/A.md', 'md/<b>.md'] }
                : { status: 'success' };
        },
    });
    h.press(DOWNLOAD);
    await h.settle();
    assert.strictEqual(h.hidden('#omn-conflict-modal'), false, 'the conflict modal did not open');
    const items = h.document.querySelectorAll('#omn-conflict-file-list li');
    assert.deepStrictEqual(items.map(function (li) { return li.textContent; }), ['md/A.md', 'md/<b>.md']);
    // A file name is text. It must not become markup.
    assert.strictEqual(h.document.querySelectorAll('#omn-conflict-file-list b').length, 0);

    // Abort closes the modal and sends NOTHING.
    h.press('[data-action="sync-resolve"][data-arg="abort"]');
    await h.settle();
    assert.strictEqual(h.hidden('#omn-conflict-modal'), true);
    assert.deepStrictEqual(syncPosts(h), ['action=download']);

    // Mark Conflicts sends pull_mark, and the page loads again.
    h.$('#omn-conflict-modal').classList.remove('omn-hidden');
    h.press('[data-action="sync-resolve"][data-arg="pull_mark"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h), ['action=download', 'action=pull_mark']);
    assert.strictEqual(h.page.location.reloads, 1);

    // Force Pull sends pull_force.
    h.press('[data-action="sync-resolve"][data-arg="pull_force"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h).slice(2), ['action=pull_force']);
});

test('a conflict with no file says that Force Pull is the answer', async () => {
    const h = notePage();
    h.server = serverWith({ '/api/sync': { status: 'conflict', files: [] } });
    h.press(DOWNLOAD);
    await h.settle();
    const items = h.document.querySelectorAll('#omn-conflict-file-list li');
    assert.strictEqual(items.length, 1);
    assert.ok(items[0].textContent.indexOf('Force Pull') >= 0, items[0].textContent);
});

test('an upload shows the files and sends the commit message', async () => {
    const h = notePage();
    h.server = serverWith({
        '/api/sync/preview': { files: ['md/A.md', 'md/B.md'], unpushed: false },
    });
    h.press(UPLOAD);
    await h.settle();
    assert.strictEqual(h.$('#omn-commit-modal').style.display, 'flex');
    assert.strictEqual(h.$('#omn-commit-file-list').textContent, 'md/A.md\nmd/B.md');
    assert.deepStrictEqual(syncPosts(h), [], 'the upload went out before the person gave a message');

    // An empty message sends nothing.
    h.$('#omn-commit-message').value = '   ';
    h.press('[data-action="commit-upload"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h), []);
    assert.strictEqual(h.dialogs[h.dialogs.length - 1].text, 'Enter a commit message.');

    h.$('#omn-commit-message').value = '  two notes  ';
    h.press('[data-action="commit-upload"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h), ['action=upload&message=two+notes']);
    assert.strictEqual(h.$('#omn-commit-modal').style.display, 'none');
    assert.strictEqual(h.$('#omn-commit-message').value, '', 'the old message stays in the box');
});

test('Cancel of the commit modal sends nothing', async () => {
    const h = notePage();
    h.server = serverWith({ '/api/sync/preview': { files: ['md/A.md'] } });
    h.press(UPLOAD);
    await h.settle();
    h.$('#omn-commit-message').value = 'not this time';
    h.press('[data-action="commit-cancel"]');
    await h.settle();
    assert.strictEqual(h.$('#omn-commit-modal').style.display, 'none');
    assert.strictEqual(h.$('#omn-commit-message').value, '');
    assert.deepStrictEqual(syncPosts(h), []);
});

test('an upload with a clean tree pushes the commits that the remote does not have', async () => {
    const h = notePage();
    h.server = serverWith({ '/api/sync/preview': { files: [], unpushed: true } });
    h.press(UPLOAD);
    await h.settle();
    // No commit, thus no message and no modal.
    assert.deepStrictEqual(syncPosts(h), ['action=upload']);
    assert.notStrictEqual(h.$('#omn-commit-modal').style.display, 'flex');
});

test('an upload with nothing to do names the remote', async () => {
    const h = notePage();
    h.server = serverWith({ '/api/sync/preview': { files: [], unpushed: false, remote: 'home' } });
    h.press(UPLOAD);
    await h.settle();
    assert.deepStrictEqual(syncPosts(h), []);
    assert.strictEqual(h.dialogs[0].text, 'Nothing to commit, and nothing to push on home.');

    // When the remote did not answer, the page must not say "nothing to
    // push". It does not know that.
    const blind = notePage();
    blind.server = serverWith({
        '/api/sync/preview': { files: [], unpushed: false, remote: 'home', remote_error: 'timeout' },
    });
    blind.press(UPLOAD);
    await blind.settle();
    assert.ok(blind.dialogs[0].text.indexOf('Could not reach the remote on home') >= 0, blind.dialogs[0].text);
    assert.ok(blind.dialogs[0].text.indexOf('timeout') >= 0);
});

test('a preview that fails tells the person and sends no upload', async () => {
    const h = notePage();
    h.server = serverWith({ '/api/sync/preview': { httpStatus: 500, text: 'fault' } });
    h.press(UPLOAD);
    await h.settle();
    assert.strictEqual(h.dialogs[0].text, 'Could not get the pending changes.');
    assert.deepStrictEqual(syncPosts(h), []);
    assert.strictEqual(h.$('.omn-progress-overlay').hidden, true);
});

test('a rejected push keeps the message for the force push', async () => {
    const h = notePage();
    h.server = serverWith({
        '/api/sync/preview': { files: ['md/A.md'] },
        '/api/sync': function (r) {
            return r.body.indexOf('action=upload') === 0 ? { status: 'push_conflict' } : { status: 'success' };
        },
    });
    h.press(UPLOAD);
    await h.settle();
    h.$('#omn-commit-message').value = 'my change';
    h.press('[data-action="commit-upload"]');
    await h.settle();
    assert.strictEqual(h.hidden('#omn-push-conflict-modal'), false, 'the push modal did not open');

    h.press('[data-action="push-force"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h).slice(1), ['action=push_force&message=my+change']);
    assert.strictEqual(h.hidden('#omn-push-conflict-modal'), true);
    // The person did not have to type the message again.
    assert.strictEqual(h.dialogs.filter(function (d) { return d.kind === 'prompt'; }).length, 0);
});

test('a force push with no message asks for one, and no message sends nothing', async () => {
    const h = notePage();
    h.server = serverWith({
        '/api/sync/preview': { files: [], unpushed: true },
        '/api/sync': function (r) {
            return r.body === 'action=upload' ? { status: 'push_conflict' } : { status: 'success' };
        },
    });
    h.press(UPLOAD);
    await h.settle();
    assert.strictEqual(h.hidden('#omn-push-conflict-modal'), false);

    // The person cancels the question.
    h.press('[data-action="push-force"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h), ['action=upload']);
    assert.strictEqual(h.dialogs[h.dialogs.length - 1].text, 'Force push canceled: no commit message.');

    // The person gives a message.
    h.$('#omn-push-conflict-modal').classList.remove('omn-hidden');
    h.prompt = function () { return '  overwrite the remote '; };
    h.press('[data-action="push-force"]');
    await h.settle();
    assert.deepStrictEqual(syncPosts(h).slice(1), ['action=push_force&message=overwrite+the+remote']);
});

test('Abort of the push modal sends nothing', async () => {
    const h = notePage();
    h.press(DOWNLOAD);
    await h.settle();
    h.requests.length = 0;
    h.$('#omn-push-conflict-modal').classList.remove('omn-hidden');
    h.press('[data-action="push-cancel"]');
    await h.settle();
    assert.strictEqual(h.hidden('#omn-push-conflict-modal'), true);
    assert.deepStrictEqual(h.requests, []);
});

test('a changed host key is stored only after OK, and the sync runs again', async () => {
    let calls = 0;
    const answers = {
        '/api/sync/trust-host-key': { status: 'success' },
        '/api/sync': function () {
            calls++;
            return calls === 1
                ? { status: 'host_key_changed', host: 'git.example:22', known: 'SHA256:old', fingerprint: 'SHA256:new' }
                : { status: 'success' };
        },
    };

    // Cancel: the key is not stored and no second sync runs.
    const no = notePage();
    no.server = serverWith(answers);
    no.press(DOWNLOAD);
    await no.settle();
    assert.ok(no.dialogs[0].text.indexOf('SHA256:old') >= 0 && no.dialogs[0].text.indexOf('SHA256:new') >= 0,
        'the question does not show the two keys');
    assert.deepStrictEqual(no.requests.filter(function (r) { return r.method === 'POST'; }).map(function (r) { return r.url; }),
        ['/api/sync']);

    // OK: the page sends the fingerprint that the person saw, and then the
    // same action again.
    calls = 0;
    const yes = notePage();
    yes.server = serverWith(answers);
    yes.confirm = function (text) { return text.indexOf('The key of the git server changed') === 0; };
    yes.press(DOWNLOAD);
    await yes.settle(8);
    const posts = yes.requests.filter(function (r) { return r.method === 'POST'; });
    assert.deepStrictEqual(posts.map(function (r) { return r.url + ' ' + r.body; }), [
        '/api/sync action=download',
        '/api/sync/trust-host-key host=git.example%3A22&fingerprint=SHA256%3Anew',
        '/api/sync action=download',
    ]);
});

test('a network fault and a server fault each tell the person', async () => {
    const down = notePage();
    down.server = serverWith({ '/api/sync': new Error('connection refused') });
    down.press(DOWNLOAD);
    await down.settle();
    assert.strictEqual(down.dialogs[0].text, 'Sync error: Error: connection refused');
    assert.strictEqual(down.$('.omn-progress-overlay').hidden, true, 'the overlay stays open after a fault');

    const fault = notePage();
    fault.server = serverWith({ '/api/sync': { status: 'error', message: 'no SSH key' } });
    fault.press(DOWNLOAD);
    await fault.settle();
    assert.strictEqual(fault.dialogs[0].text, 'Sync failed: no SSH key');
    assert.strictEqual(fault.page.location.reloads, 0);
});
