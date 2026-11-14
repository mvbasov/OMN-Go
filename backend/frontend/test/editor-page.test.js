// The editor page, in the REAL JavaScript and on the markup of editor.html.
// This file holds the load of the note, the save, the exit and the jump to
// a line. It also holds the selection cycle, the two view toggles and the
// upload of a dropped file.
//
// THE RULE THAT MATTERS MOST HERE. A load that failed is not an empty note.
// The editor must refuse the save, because a save would replace a file
// that the editor never received. A 404 is the one exception: it is a new
// note, and a save makes it.
//
// editor-view.test.js holds the view after an expansion, and
// editor-find.test.js holds the find bar.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { editorPage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const NOTE = 'Title: Note\nTags: a\n\nfirst line\nsecond line\nthird line\n';
const BODY = NOTE.indexOf('first line');
const SECOND = NOTE.indexOf('second line');
const SECOND_END = SECOND + 'second line'.length;

const CYCLE = '#omn-editor-tools button[title^="Cycle selection"]';

// noteServer answers the note with one status, and it gives save to each
// POST of /api/save.
function noteServer(status, save) {
    return function (request) {
        if (request.method === 'GET') return { httpStatus: status, text: status === 200 ? NOTE : 'no' };
        return save || { status: 'success' };
    };
}

// saves answers each save request that the page sent.
function saves(h) {
    return h.requests.filter(function (r) { return r.url === '/api/save'; });
}

// selection answers the selection of the textarea as [start, end].
function selection(h) {
    const ta = h.$('#omn-editor-text');
    return [ta.selectionStart, ta.selectionEnd];
}

// drop sends a drop event with one file to the textarea. The file answers
// its name as text, thus the body of the request names the file.
function drop(h, name, type) {
    const file = { name: name, type: type || '', toString: function () { return name; } };
    const event = new Event('drop', { bubbles: true, dataTransfer: { files: [file] } });
    h.$('#omn-editor-text').dispatchEvent(event);
    return event;
}

// ---------------------------------------------------------------------
// The load
// ---------------------------------------------------------------------

// A tap on a word with a spell-check mark opens a menu of Android ("Add to
// dictionary", "Delete"). The menu takes the focus, and the keyboard closes.
// spellcheck="false" removes the marks and thus the menu.
//
// autocomplete="off" would set NO_SUGGESTIONS for the keyboard. The keyboard
// would then show no suggestions, and a non-Latin layout would type nothing.
test('the text has no spell check, and the keyboard keeps its suggestions', async () => {
    const h = await editorPage(NOTE);
    const ta = h.$('#omn-editor-text');
    assert.strictEqual(ta.getAttribute('spellcheck'), 'false');
    assert.notStrictEqual(ta.getAttribute('autocomplete'), 'off');
    assert.notStrictEqual(ta.getAttribute('autocorrect'), 'off');
});

test('the editor asks for the note by name and opens it on its body', async () => {
    const h = await editorPage(NOTE);
    assert.deepStrictEqual(h.requests.map((r) => r.method + ' ' + r.url), ['GET /api/note?name=Note']);
    assert.strictEqual(h.$('#omn-editor-text').value, NOTE);
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Note');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-clean');
    assert.strictEqual(h.document.activeElement, h.$('#omn-editor-text'));
    assert.deepStrictEqual(selection(h), [BODY, BODY], 'the caret is not on the first line of the body');
});

test('a note that does not exist opens empty, and a save makes it', async () => {
    const h = await editorPage('', { server: noteServer(404) });
    assert.strictEqual(h.$('#omn-editor-text').value, '');
    assert.strictEqual(h.$('#omn-editor-save').disabled, false);
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Note');
    h.type('#omn-editor-text', 'new text');
    h.press('#omn-editor-save');
    await h.settle();
    assert.strictEqual(saves(h).length, 1);
});

test('a load that the server refused switches the save off', async () => {
    const h = await editorPage('', { server: noteServer(500) });
    assert.strictEqual(h.$('#omn-editor-text').value, '');
    assert.strictEqual(h.$('#omn-editor-save').disabled, true);
    assert.match(h.$('#omn-editor-status').textContent, /^Could not load Note/);
    assert.strictEqual(h.$('#omn-editor-status').className, 'omn-editor-status-error');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-error');

    // The key does not pass through the button, thus it needs its own guard.
    h.type('#omn-editor-text', 'text that would replace the file');
    h.key('#omn-editor-text', 's', { ctrlKey: true });
    await h.settle();
    assert.strictEqual(saves(h).length, 0, 'the editor saved over a file that it did not load');
});

test('a load that the network refused switches the save off', async () => {
    const h = await editorPage('', { server: function () { return new Error('offline'); } });
    assert.strictEqual(h.$('#omn-editor-save').disabled, true);
    assert.match(h.$('#omn-editor-status').textContent, /^Could not load Note/);
});

// ---------------------------------------------------------------------
// The save and the exit
// ---------------------------------------------------------------------

test('the Save button sends the name and the text, and opens the note', async () => {
    const h = await editorPage(NOTE);
    h.type('#omn-editor-text', NOTE + 'more & more\n');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-dirty');
    h.press('#omn-editor-save');
    await h.settle();

    const sent = saves(h);
    assert.strictEqual(sent.length, 1);
    assert.strictEqual(sent[0].method, 'POST');
    const body = new URLSearchParams(sent[0].body);
    assert.strictEqual(body.get('name'), 'Note');
    assert.strictEqual(body.get('content'), NOTE + 'more & more\n');
    assert.deepStrictEqual(h.went, ['/Note.html']);
    // The Back key must not open the editor again after a save.
    assert.deepStrictEqual(h.replaced, ['/Note.html'], 'the editor stays in the history');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-clean');
});

test('Ctrl+S and Cmd+S save, and the browser does not get the key', async () => {
    for (const init of [{ ctrlKey: true }, { metaKey: true }]) {
        const h = await editorPage(NOTE);
        const event = h.key('#omn-editor-text', 'S', init);
        await h.settle();
        assert.strictEqual(event.defaultPrevented, true);
        assert.strictEqual(saves(h).length, 1);
        assert.deepStrictEqual(h.went, ['/Note.html']);
    }
});

test('a save with no admin session tells the person and stays in the editor', async () => {
    for (const status of [401, 403]) {
        const h = await editorPage(NOTE, { server: noteServer(200, { httpStatus: status, text: 'no' }) });
        h.type('#omn-editor-text', 'changed');
        h.press('#omn-editor-save');
        await h.settle();
        assert.match(h.$('#omn-editor-status').textContent, /^Not authorized/);
        assert.strictEqual(h.$('#omn-editor-status').className, 'omn-editor-status-error');
        assert.deepStrictEqual(h.went, [], 'the editor left the page and the text is lost');
        assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-dirty');
    }
});

test('a save that failed shows the cause and stays in the editor', async () => {
    const h = await editorPage(NOTE, { server: noteServer(200, { httpStatus: 500, text: 'no' }) });
    h.type('#omn-editor-text', 'changed');
    h.press('#omn-editor-save');
    await h.settle();
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Save failed: HTTP 500');
    assert.deepStrictEqual(h.went, []);
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-dirty');

    h.server = function () { return new Error('offline'); };
    h.press('#omn-editor-save');
    await h.settle();
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Save failed: offline');
    assert.deepStrictEqual(h.went, []);
});

test('Cancel with no change leaves at once', async () => {
    const h = await editorPage(NOTE);
    h.press('#omn-editor-cancel');
    assert.deepStrictEqual(h.dialogs, []);
    assert.deepStrictEqual(h.went, ['/Note.html']);
    assert.deepStrictEqual(h.replaced, ['/Note.html'], 'the editor stays in the history');
    assert.strictEqual(saves(h).length, 0);
});

test('Cancel with a change asks first, and No keeps the text', async () => {
    const h = await editorPage(NOTE);
    h.type('#omn-editor-text', 'changed');
    h.press('#omn-editor-cancel');
    assert.deepStrictEqual(h.dialogs, [{ kind: 'confirm', text: 'Discard unsaved changes?' }]);
    await h.settle();
    assert.deepStrictEqual(h.went, []);
    assert.strictEqual(h.$('#omn-editor-text').value, 'changed');
    assert.strictEqual(h.document.querySelector('.omn-dialog'), null, 'the dialog stays on the page after the answer');

    h.confirm = function () { return true; };
    h.press('#omn-editor-cancel');
    await h.settle();
    assert.deepStrictEqual(h.went, ['/Note.html']);
    assert.strictEqual(saves(h).length, 0);
});

test('the browser asks before it closes a page with a change', async () => {
    const h = await editorPage(NOTE);
    const clean = new Event('beforeunload');
    h.page.dispatchEvent(clean);
    assert.strictEqual(clean.defaultPrevented, false);

    h.type('#omn-editor-text', 'changed');
    const dirty = new Event('beforeunload');
    h.page.dispatchEvent(dirty);
    assert.strictEqual(dirty.defaultPrevented, true);
    assert.strictEqual(dirty.returnValue, '');
});

// ---------------------------------------------------------------------
// The jump to a line
// ---------------------------------------------------------------------

test('the address can name a line by its number', async () => {
    const h = await editorPage(NOTE, { search: '?line=5' });
    assert.deepStrictEqual(selection(h), [SECOND, SECOND_END]);
    // Line 5 has the index 4. The view keeps three lines above it.
    assert.strictEqual(h.$('#omn-editor-text').scrollTop, 18);
});

test('the address can name a line by its text', async () => {
    const h = await editorPage(NOTE, { search: '?find=' + encodeURIComponent('cond li') });
    assert.deepStrictEqual(selection(h), [SECOND, SECOND_END]);
});

test('a text that the note does not hold gives the usual start', async () => {
    const h = await editorPage(NOTE, { search: '?find=absent' });
    assert.deepStrictEqual(selection(h), [BODY, BODY]);
});

// ---------------------------------------------------------------------
// The selection cycle
// ---------------------------------------------------------------------

test('the cycle has seven stages and holds the caret of its start', async () => {
    const h = await editorPage(NOTE);
    const ta = h.$('#omn-editor-text');
    const caret = SECOND + 3;
    ta.setSelectionRange(caret, caret);

    const want = [
        [SECOND, SECOND_END],      // the line
        [caret, SECOND_END],       // from the caret to the end of the line
        [SECOND, caret],           // from the start of the line to the caret
        [SECOND, NOTE.length],     // from the line to the end of the note
        [BODY, SECOND_END],        // from the header block to the line
        [BODY, NOTE.length],       // the body
        [0, NOTE.length],          // the whole note
        [SECOND, SECOND_END],      // the cycle starts again
    ];
    want.forEach(function (range, i) {
        h.press(CYCLE);
        assert.deepStrictEqual(selection(h), range, 'stage ' + (i + 1));
    });
    assert.strictEqual(h.document.activeElement, ta);
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-clean', 'a selection is not a change');
});

test('stage five in the header block selects down to the body', async () => {
    const h = await editorPage(NOTE);
    h.$('#omn-editor-text').setSelectionRange(3, 3);
    for (let i = 0; i < 5; i++) h.press(CYCLE);
    assert.deepStrictEqual(selection(h), [0, BODY]);
});

test('a move of the caret starts a new cycle on the new line', async () => {
    const h = await editorPage(NOTE);
    const ta = h.$('#omn-editor-text');
    ta.setSelectionRange(SECOND, SECOND);
    h.press(CYCLE);
    h.press(CYCLE);
    ta.setSelectionRange(BODY + 2, BODY + 2);
    h.press(CYCLE);
    assert.deepStrictEqual(selection(h), [BODY, BODY + 'first line'.length]);
});

// ---------------------------------------------------------------------
// Word wrap and line numbers
// ---------------------------------------------------------------------

test('the editor wraps by default, and line numbers are off while it wraps', async () => {
    const h = await editorPage(NOTE);
    const ta = h.$('#omn-editor-text');
    assert.strictEqual(ta.getAttribute('wrap'), 'soft');
    assert.strictEqual(ta.classList.contains('omn-nowrap'), false);
    assert.strictEqual(h.$('#omn-tool-wrap').classList.contains('omn-active'), true);
    assert.strictEqual(h.$('#omn-tool-ln').disabled, true);
    assert.strictEqual(h.$('#omn-editor-gutter').textContent, '');
});

test('the two toggles change the view and the device keeps them', async () => {
    const h = await editorPage(NOTE);
    const ta = h.$('#omn-editor-text');
    h.press('#omn-tool-wrap');
    assert.strictEqual(ta.getAttribute('wrap'), 'off');
    assert.strictEqual(ta.classList.contains('omn-nowrap'), true);
    assert.strictEqual(h.$('#omn-editor-mirror').classList.contains('omn-nowrap'), true, 'the mirror wraps and the textarea does not');
    assert.strictEqual(h.$('#omn-tool-wrap').classList.contains('omn-active'), false);
    assert.strictEqual(h.$('#omn-tool-ln').disabled, false);
    assert.strictEqual(h.storage.omngo_editor_wrap, '0');
    assert.strictEqual(h.document.body.classList.contains('omn-ln-on'), false);

    h.press('#omn-tool-ln');
    assert.strictEqual(h.document.body.classList.contains('omn-ln-on'), true);
    assert.strictEqual(h.$('#omn-tool-ln').classList.contains('omn-active'), true);
    assert.strictEqual(h.$('#omn-editor-gutter').textContent, '1\n2\n3\n4\n5\n6\n7');
    assert.strictEqual(h.storage.omngo_editor_ln, '1');

    // A new line gets a number, and the gutter follows the scroll.
    h.type('#omn-editor-text', NOTE + 'fourth line\n');
    assert.strictEqual(h.$('#omn-editor-gutter').textContent, '1\n2\n3\n4\n5\n6\n7\n8');
    ta.scrollTop = 40;
    ta.dispatchEvent(new Event('scroll'));
    assert.strictEqual(h.$('#omn-editor-gutter').scrollTop, 40);

    // Word wrap hides the numbers and keeps the wish of the person.
    h.press('#omn-tool-wrap');
    assert.strictEqual(h.document.body.classList.contains('omn-ln-on'), false);
    assert.strictEqual(h.$('#omn-editor-gutter').textContent, '');
    assert.strictEqual(h.storage.omngo_editor_ln, '1');
    assert.strictEqual(h.storage.omngo_editor_wrap, '1');
});

test('the editor starts with the toggles that the device keeps', async () => {
    const h = await editorPage(NOTE, { storage: { omngo_editor_wrap: '0', omngo_editor_ln: '1' } });
    assert.strictEqual(h.$('#omn-editor-text').getAttribute('wrap'), 'off');
    assert.strictEqual(h.document.body.classList.contains('omn-ln-on'), true);
    assert.strictEqual(h.$('#omn-editor-gutter').textContent, '1\n2\n3\n4\n5\n6\n7');
});

test('a browser with no storage gets the default view', async () => {
    const h = await editorPage(NOTE);
    h.page.localStorage.setItem = function () { throw new Error('SecurityError'); };
    h.press('#omn-tool-wrap');
    assert.strictEqual(h.$('#omn-editor-text').getAttribute('wrap'), 'off');
});

// ---------------------------------------------------------------------
// The upload of a dropped file
// ---------------------------------------------------------------------

test('a dropped image goes to the image upload and its link goes to the caret', async () => {
    const h = await editorPage(NOTE);
    h.server = function () { return { httpStatus: 200, text: '![pic](/images/pic.png)' }; };
    const over = new Event('dragover', { bubbles: true });
    h.$('#omn-editor-text').dispatchEvent(over);
    assert.strictEqual(over.defaultPrevented, true, 'the browser opens the file in place of the editor');

    const event = drop(h, 'pic.png', 'image/png');
    assert.strictEqual(event.defaultPrevented, true);
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Uploading pic.png…');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-loading');
    await h.settle();

    const sent = h.requests[h.requests.length - 1];
    assert.strictEqual(sent.method + ' ' + sent.url, 'POST /api/upload');
    assert.strictEqual(sent.body, 'image=pic.png');
    assert.strictEqual(h.$('#omn-editor-text').value, NOTE.slice(0, BODY) + '![pic](/images/pic.png)' + NOTE.slice(BODY));
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Note');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-dirty');
});

test('a dropped JSON file goes to the JSON upload, by its name or by its type', async () => {
    for (const file of [['data.JSON', ''], ['data', 'application/json']]) {
        const h = await editorPage(NOTE);
        h.server = function () { return { httpStatus: 200, text: '[data](/user_json/data.json)' }; };
        drop(h, file[0], file[1]);
        await h.settle();
        const sent = h.requests[h.requests.length - 1];
        assert.strictEqual(sent.url, '/api/upload_json?incoming=1');
        assert.strictEqual(sent.body, 'file=' + file[0]);
        assert.ok(h.$('#omn-editor-text').value.indexOf('[data](/user_json/data.json)') === BODY);
    }
});

// A contact and a calendar have an upload of their own, the same as a JSON
// file. A wrong route here gives "file type is not allowed" for a good file.
test('a dropped contact or calendar goes to the upload of its tree', async () => {
    const cases = [
        ['log.jsonl', '', '/api/upload_json'],
        ['AnnLee.VCF', '', '/api/upload_contacts'],
        ['AnnLee', 'text/x-vcard', '/api/upload_contacts'],
        ['card', 'text/vcard', '/api/upload_contacts'],
        ['invite.ics', 'application/octet-stream', '/api/upload_calendars'],
        ['old.vcs', '', '/api/upload_calendars'],
        ['event', 'text/calendar', '/api/upload_calendars'],
        // The name decides before the type.
        ['invite.ics', 'application/json', '/api/upload_calendars'],
        // Each other file is an image for the server to accept or refuse.
        ['notes.txt', 'text/plain', '/api/upload'],
    ];
    for (const c of cases) {
        const h = await editorPage(NOTE);
        h.server = function () { return { httpStatus: 200, text: '[x](/user_contacts/x.vcf)' }; };
        drop(h, c[0], c[1]);
        await h.settle();
        const sent = h.requests[h.requests.length - 1];
        // incoming=1 asks the server for a line on the Incoming notes page.
        const want = c[2] === '/api/upload' ? c[2] : c[2] + '?incoming=1';
        assert.strictEqual(sent.url, want, c[0] + ' (' + c[1] + ')');
        assert.strictEqual(sent.body, (c[2] === '/api/upload' ? 'image=' : 'file=') + c[0]);
    }
});

test('an upload that the server refused shows the cause and changes no text', async () => {
    const h = await editorPage(NOTE);
    h.server = function () { return { httpStatus: 413, text: ' file too large \n' }; };
    drop(h, 'big.png', 'image/png');
    await h.settle();
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Upload failed: file too large');
    assert.strictEqual(h.$('#omn-editor-status').className, 'omn-editor-status-error');
    assert.strictEqual(h.$('#omn-editor-text').value, NOTE);
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-clean', 'the dot stayed on "loading"');

    h.server = function () { return { httpStatus: 500, text: '' }; };
    drop(h, 'big.png', 'image/png');
    await h.settle();
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Upload failed: HTTP 500');

    h.server = function () { return new Error('offline'); };
    drop(h, 'big.png', 'image/png');
    await h.settle();
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Upload failed: offline');
    assert.strictEqual(h.$('#omn-editor-text').value, NOTE);
});

test('a drop with no file is not an upload', async () => {
    const h = await editorPage(NOTE);
    const before = h.requests.length;
    const event = new Event('drop', { bubbles: true, dataTransfer: { files: [] } });
    h.$('#omn-editor-text').dispatchEvent(event);
    await h.settle();
    assert.strictEqual(event.defaultPrevented, false, 'the browser cannot drop text into the note');
    assert.strictEqual(h.requests.length, before);
});
