// The metadata panel, the clipboard and the send of a note, in the REAL
// JavaScript and on the markup of the templates.
//
// The metadata panel shows the header block of the note. Each value comes
// from the note itself, thus the panel writes it as TEXT. A value with
// markup must not become an element of the page.
//
// The panel also holds three controls: send the note, copy the note as
// text, and copy a link to the page. Each one names the note in the form
// that the server reads with no guess. See omnGoCurrentNoteName.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');

const META = {
    Title: 'My [first] note',
    Date: '2026-08-01 11:12:03',
    Modified: '2026-08-03 12:00:00',
    Author: 'A <b>bold</b> name',
    Tags: 'one, two',
    viewport: 'width=device-width',
};

// page answers a note page with the header values above. Its timers wait
// for the test.
function page(opts) {
    return notePage(Object.assign({ note: 'dir/Draft.txt', meta: META }, opts || {})).holdTimers();
}

// control answers the button of the panel with this label.
function control(h, label) {
    const found = h.document.querySelectorAll('#omn-metadata-panel button.omn-metadata-send')
        .find(function (b) { return b.title === label; });
    assert.ok(found, 'the panel has no control "' + label + '"');
    return found;
}

function status(h) {
    return h.$('#omn-metadata-panel .omn-metadata-send-status').textContent;
}

test('the panel shows the file and each line of the header block', () => {
    const h = page();
    assert.strictEqual(h.$('.omn-metadata-file-name').textContent, 'File: dir/Draft.txt');
    const rows = h.document.querySelectorAll('#omn-metadata-panel .omn-metadata-row')
        .map(function (r) { return r.textContent; });
    assert.deepStrictEqual(rows, [
        'Title: My [first] note',
        'Date: 2026-08-01 11:12:03',
        'Modified: 2026-08-03 12:00:00',
        'Author: A <b>bold</b> name',
        'Tags: one, two',
    ]);
});

test('a header value with markup stays text', () => {
    const h = page();
    assert.strictEqual(h.document.querySelectorAll('#omn-metadata-panel b').length, 0,
        'a value of the note became an element of the page');
});

test('the panel has three controls on a note from the server', () => {
    const h = page();
    const labels = h.document.querySelectorAll('#omn-metadata-panel button.omn-metadata-send')
        .map(function (b) { return b.title; });
    assert.deepStrictEqual(labels, ['Send this note', 'Copy this note as text', 'Copy a link to this page']);
    for (const b of h.document.querySelectorAll('#omn-metadata-panel button.omn-metadata-send')) {
        assert.strictEqual(b.getAttribute('aria-label'), b.title);
    }
});

test('a page that is not a note has only the link control, and a page from disk has none', () => {
    const h = notePage({ note: 'site.css', markdown: false });
    const labels = h.document.querySelectorAll('#omn-metadata-panel button.omn-metadata-send')
        .map(function (b) { return b.title; });
    assert.deepStrictEqual(labels, ['Copy a link to this page']);

    const disk = page({ protocol: 'file:' });
    assert.strictEqual(disk.document.querySelectorAll('#omn-metadata-panel button').length, 0);
    assert.strictEqual(disk.$('.omn-metadata-file-name').textContent, 'File: dir/Draft.txt');
});

test('Send opens the export of the note, with the extension that makes the name clear', () => {
    const h = page();
    control(h, 'Send this note').click();
    // The note has the name "Draft.txt". With no ".md" the server reads
    // that name as a file.
    assert.deepStrictEqual(h.went, ['/api/export/note?name=dir%2FDraft.txt.md']);
});

test('Send in the Android app gives the note to the share sheet', () => {
    const h = page();
    h.page.IS_ANDROID = true;
    control(h, 'Send this note').click();
    assert.deepStrictEqual(h.went, ['omngo://share?name=dir%2FDraft.txt.md']);
});

test('Copy this note asks the server and writes the text to the clipboard', async () => {
    const h = page();
    h.server = function (request) {
        if (request.url.indexOf('/api/export/note') === 0) {
            return { httpStatus: 200, text: 'Title: My note\nFileName: dir/Draft.txt.md\n\nThe body.' };
        }
        return { status: 'success' };
    };
    control(h, 'Copy this note as text').click();
    await h.settle();
    const asks = h.requests.filter(function (r) { return r.url.indexOf('/api/export/note') === 0; });
    assert.deepStrictEqual(asks.map(function (r) { return r.url; }), ['/api/export/note?name=dir%2FDraft.txt.md']);
    assert.deepStrictEqual(h.clipboard, ['Title: My note\nFileName: dir/Draft.txt.md\n\nThe body.']);
    assert.strictEqual(status(h), 'Copied');
    // The page removes the status text 4 seconds later.
    assert.deepStrictEqual(h.pendingTimers(), [4000]);
    h.runTimers();
    assert.strictEqual(status(h), '');
});

test('a copy that the server refuses says so and writes nothing', async () => {
    const h = page();
    h.server = function (request) {
        if (request.url.indexOf('/api/export/note') === 0) return { httpStatus: 404, text: 'no' };
        return { status: 'success' };
    };
    control(h, 'Copy this note as text').click();
    await h.settle();
    assert.strictEqual(status(h), 'Copy failed: HTTP 404');
    assert.deepStrictEqual(h.clipboard, []);
    assert.deepStrictEqual(h.document.commands, []);
});

test('Copy a link writes a Markdown link with the title of the note', async () => {
    const h = page({ path: '/dir/Draft.txt.html' });
    h.page.location.search = '?a=(1)';
    control(h, 'Copy a link to this page').click();
    await h.settle();
    // The brackets of the title and of the address have an escape, thus
    // the link stays one link in a note.
    assert.deepStrictEqual(h.clipboard, ['[My \\[first\\] note](/dir/Draft.txt.html?a=%281%29)']);
    assert.strictEqual(status(h), 'Link copied');
});

test('the title of the link comes from the header, then from the page, then from the name', () => {
    const header = page();
    assert.strictEqual(header.page.omnGoPageTitle(), 'My [first] note');

    const noHeader = notePage({ note: 'Plain', title: 'The page title' });
    assert.strictEqual(noHeader.page.omnGoPageTitle(), 'The page title');

    const noTitle = notePage({ note: 'Plain' });
    noTitle.page.Title = '';
    assert.strictEqual(noTitle.page.omnGoPageTitle(), 'Plain');

    noTitle.page.PageName = '';
    assert.strictEqual(noTitle.page.omnGoPageTitle(), 'link');
});

test('omnGoCopyText uses the second way when the browser refuses the Clipboard API', async () => {
    const h = page();
    h.clipboardRefuses = true;
    await h.page.omnGoCopyText('some text');
    assert.deepStrictEqual(h.clipboard, []);
    assert.deepStrictEqual(h.document.commands, ['copy']);
    // The scratch textarea is gone again.
    assert.strictEqual(h.document.querySelectorAll('body > textarea[readonly]').length, 0);
});

test('omnGoCopyText uses the second way on a page that is not a secure context', async () => {
    const h = page();
    // A page of the LAN with plain http is not a secure context.
    h.page.isSecureContext = false;
    await h.page.omnGoCopyText('some text');
    assert.deepStrictEqual(h.clipboard, []);
    assert.deepStrictEqual(h.document.commands, ['copy']);
});

test('omnGoCopyText fails when both ways fail, and the control says so', async () => {
    const h = page();
    h.clipboardRefuses = true;
    h.document.execCommand = function () { return false; };
    await assert.rejects(h.page.omnGoCopyText('x'), /the browser refused the copy/);

    control(h, 'Copy a link to this page').click();
    await h.settle();
    assert.strictEqual(status(h), 'Copy failed: the browser refused the copy');
});

test('omnGoCurrentNoteName adds ".md" only for a note', () => {
    const note = notePage({ note: 'dir/Draft.txt' });
    assert.strictEqual(note.page.omnGoCurrentNoteName(), 'dir/Draft.txt.md');
    note.page.IS_MARKDOWN = false;
    assert.strictEqual(note.page.omnGoCurrentNoteName(), 'dir/Draft.txt');
    note.page.PageName = '';
    assert.strictEqual(note.page.omnGoCurrentNoteName(), '');
});
