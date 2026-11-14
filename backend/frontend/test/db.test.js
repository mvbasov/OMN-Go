// The Database Backups page, in the REAL JavaScript and on the markup of
// db_backups.html.
//
// The page lists the databases from /api/db/backups. "Backup now" and
// "Restore" ask in a dialog of omn-go-dialog.js first. Cancel sends nothing.
// Restore replaces a whole database, thus Cancel is its default button.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { systemPage } = require('./dom-page.js');

// list is the answer of /api/db/backups. The database "notes" holds two
// backups, and the prune depth is two. Thus a new backup deletes the oldest.
function list() {
    return {
        status: 'success',
        hostname: 'phone',
        prune_depth: 2,
        databases: [{
            name: 'notes', state: 'insync', sqlite_exists: true, size: 2048, mtime: '2026-11-14T10:00:00Z',
            backups: [
                { file: 'b2.jsonl', valid: true, created: '2026-11-14T09:00:00Z', hostname: 'pc', objects: 3, rows: 40, size: 4096 },
                { file: 'b<1>.jsonl', valid: true, created: '2026-11-13T09:00:00Z', hostname: 'pc', objects: 3, rows: 30, size: 3072 },
            ],
        }],
    };
}

// open answers the page after its first load. more answers the requests
// that are not the list, or null for a success.
async function open(more) {
    const h = systemPage('db_backups.html', 'omn-go-db.js', function (request) {
        if (request.url === '/api/db/backups') return list();
        return (more && more(request)) || { status: 'success' };
    });
    h.keepDialogs = true;
    await h.settle();
    return h;
}

function dialogOf(h, kind) {
    return h.document.querySelector('.omn-dialog[data-omn-dialog="' + kind + '"]');
}

function posts(h) {
    return h.requests.filter(function (r) { return r.method === 'POST'; }).map(function (r) { return r.url; });
}

test('the page lists each database, and a name is text', async () => {
    const h = await open();
    assert.strictEqual(h.$('.omn-dbb-name').textContent, 'notes');
    const files = h.document.querySelectorAll('.omn-dbb-bk code').map(function (c) { return c.textContent; });
    assert.strictEqual(files.join('|'), 'b2.jsonl|b<1>.jsonl');
});

test('Backup now asks first, names the backup that it deletes, and Cancel sends nothing', async () => {
    const h = await open();
    h.press('[data-action="dbb-backup"]');
    await h.settle();
    let dialog = dialogOf(h, 'db-backup');
    assert.ok(dialog, 'no dialog before the backup');
    assert.strictEqual(dialog.querySelector('.omn-dialog-text').textContent, 'Create a new backup of notes?');
    assert.ok(dialog.querySelector('.omn-dialog-warning').textContent.indexOf('b<1>.jsonl') >= 0);
    dialog.querySelector('[data-omn-cancel]').click();
    await h.settle();
    assert.deepStrictEqual(posts(h), []);

    h.press('[data-action="dbb-backup"]');
    await h.settle();
    dialog = dialogOf(h, 'db-backup');
    dialog.querySelector('[data-omn-default]').click();
    await h.settle();
    assert.deepStrictEqual(posts(h), ['/api/db/backup?db=notes']);
    // The page reads the list again after the backup.
    assert.strictEqual(h.requests.filter(function (r) { return r.url === '/api/db/backups'; }).length, 2);
});

test('Restore shows the backup, Cancel is the default, and Replace sends the restore', async () => {
    const h = await open();
    h.press('[data-action="dbb-details"]');
    h.press('[data-action="dbb-restore"]');
    await h.settle();
    const dialog = dialogOf(h, 'db-restore');
    assert.ok(dialog, 'no dialog before the restore');
    const cells = dialog.querySelectorAll('.omn-dialog-table td').map(function (c) { return c.textContent; });
    assert.strictEqual(cells[1], 'b2.jsonl');
    assert.strictEqual(cells[3], '3 objects, 40 rows, 4.0 KB');
    // Enter never replaces a database.
    assert.strictEqual(dialog.querySelector('[data-omn-default]').textContent, 'Cancel');
    const replace = dialog.querySelector('button');
    assert.strictEqual(replace.textContent, 'Replace database');
    assert.ok(replace.classList.contains('omn-btn-danger'));
    replace.click();
    await h.settle();
    assert.deepStrictEqual(posts(h), ['/api/db/restore?db=notes&file=b2.jsonl']);
});

test('a backup that fails hides the progress bar and tells the person', async () => {
    const h = await open(function (request) {
        if (request.url.indexOf('/api/db/backup?') === 0) return { status: 'error', message: 'disk full' };
        return null;
    });
    h.press('[data-action="dbb-backup"]');
    await h.settle();
    dialogOf(h, 'db-backup').querySelector('[data-omn-default]').click();
    await h.settle();
    const alert = dialogOf(h, 'alert');
    assert.ok(alert, 'no message for the fault');
    assert.strictEqual(alert.querySelector('.omn-dialog-text').textContent, 'Backup failed: disk full');
    assert.strictEqual(h.$('.omn-progress-overlay').hidden, true);
});
