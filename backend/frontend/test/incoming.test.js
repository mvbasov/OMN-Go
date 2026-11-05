// The receive box of the Incoming notes page, in the REAL JavaScript and on
// the markup of the templates.
//
// The box takes a note, and it takes a JSON file, a contact and a calendar.
// A note goes to /api/import/note. Each other file goes to the upload route
// of its tree, and the server puts a line for it in the list.
//
// The server injects OMN_USER_FILE_UPLOADS, the map from an extension to a
// route. A fault here sends a contact to the note import, and the contact
// then becomes a note with no sense.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { newDomPage, shellScripts } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const UPLOADS = {
    '.json': '/api/upload_json', '.jsonl': '/api/upload_json',
    '.vcf': '/api/upload_contacts',
    '.ics': '/api/upload_calendars', '.vcs': '/api/upload_calendars',
};

// incomingPage answers the Incoming notes page after the load events. vars
// gives the values that the server injects.
function incomingPage(vars, note) {
    const h = newDomPage({ note: note || 'incoming/incoming' });
    Object.assign(h.page, { OMN_INCOMING_PAGE: 'incoming/incoming', OMN_USER_FILE_UPLOADS: UPLOADS }, vars || {});
    return h.load(shellScripts()).ready();
}

function file(name) {
    return { name: name, toString: function () { return name; } };
}

// posts answers each request that sent a file. The page also reads the
// settings while it loads, and that request is not the subject here.
function posts(h) {
    return h.requests.filter((r) => r.method === 'POST');
}

// drop gives files to the box, the same as a person who drops them on it.
function drop(h, names) {
    const event = new Event('drop', { bubbles: true, dataTransfer: { files: names.map(file), getData: function () { return ''; } } });
    h.$('#omn-incoming').dispatchEvent(event);
    return event;
}

test('the box shows on the Incoming notes page only, and never on Android', () => {
    let h = incomingPage();
    assert.strictEqual(h.hidden('#omn-incoming'), false);
    assert.strictEqual(h.$('#omn-preview').firstChild, h.$('#omn-incoming'), 'the box is not above the list');

    h = incomingPage({}, 'Note');
    assert.strictEqual(h.document.querySelector('#omn-incoming'), null, 'a note page keeps the box');

    h = incomingPage({ IS_ANDROID: true });
    assert.strictEqual(h.document.querySelector('#omn-incoming'), null, 'Android keeps the box');
});

test('the file dialog offers a note and each extension that the server names', () => {
    const accept = incomingPage().$('#omn-incoming-files').accept.split(',');
    for (const ext of ['.md'].concat(Object.keys(UPLOADS))) {
        assert.ok(accept.indexOf(ext) >= 0, ext + ' is not in ' + accept);
    }
});

test('a note goes to the import, and each other file goes to the upload of its tree', async () => {
    const h = incomingPage();
    const event = drop(h, ['Plan.md', 'Ann Lee.VCF', 'invite.ics', 'old.vcs', 'data.json', 'log.jsonl']);
    assert.strictEqual(event.defaultPrevented, true, 'the browser opens the file in place of the box');
    await h.settle(20);

    assert.deepStrictEqual(posts(h).map((r) => r.method + ' ' + r.url), [
        'POST /api/import/note',
        'POST /api/upload_contacts?incoming=1',
        'POST /api/upload_calendars?incoming=1',
        'POST /api/upload_calendars?incoming=1',
        'POST /api/upload_json?incoming=1',
        'POST /api/upload_json?incoming=1',
    ]);
    assert.ok(posts(h)[1].body.indexOf('file=') === 0, 'the upload reads the field "file"');
    // The server wrote the list, thus the page comes again.
    assert.strictEqual(h.page.location.reloads, 1);
    assert.strictEqual(h.$('#omn-incoming-status').textContent, 'Imported 6. Refreshing…');
});

test('a file that the server refused shows the words of the server', async () => {
    const h = incomingPage();
    h.server = function (request) {
        if (request.url.indexOf('/api/upload_contacts') === 0) {
            return { httpStatus: 400, text: ' file too large (limit is 3.00 MB) \n' };
        }
        if (request.url.indexOf('/api/upload_calendars') === 0) return { httpStatus: 401, text: '' };
        return { status: 'success' };
    };
    drop(h, ['big.vcf', 'a.ics', 'Plan.md']);
    await h.settle(20);

    const status = h.$('#omn-incoming-status');
    assert.strictEqual(status.textContent,
        '1 imported, 2 failed — big.vcf: file too large (limit is 3.00 MB); ' +
        'a.ics: log in as admin to import a file');
    assert.strictEqual(status.classList.contains('omn-is-error'), true);
    assert.strictEqual(h.page.location.reloads, 0, 'the page came again and lost the message');
});

test('a page of an older server sends each file to the note import', async () => {
    const h = incomingPage({ OMN_USER_FILE_UPLOADS: undefined });
    drop(h, ['a.ics']);
    await h.settle(20);
    assert.deepStrictEqual(posts(h).map((r) => r.url), ['/api/import/note']);
});

test('the Import button with no file asks for one', async () => {
    const h = incomingPage();
    h.press('#omn-incoming-import');
    await h.settle();
    assert.strictEqual(h.$('#omn-incoming-status').textContent, 'Choose a file first.');
    assert.strictEqual(posts(h).length, 0);
});
