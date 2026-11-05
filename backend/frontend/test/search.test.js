// The search dialog, in the REAL JavaScript and on the markup of the
// templates.
//
// The dialog has two scopes. "This page" marks the words in the note on
// screen. "All notes" lists the notes that match, and a press opens one
// with the words marked.
//
// THE RULE THAT IS EASY TO BREAK. The dialog sends a request only on Enter
// or on a press of the magnifier, and never while the person types. A
// search over all notes reads the whole index, and a request for each key
// would start many of them.
//
// The server half is in backend/internal/search.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const NOTE = '<h2 id="first">First part</h2><p>The alpha line of the note.</p>' +
    '<h2 id="second">Second part</h2><p>Another alpha here, and a beta.</p>';

// searches answers the address of each request to /api/search, in order.
function searches(h) {
    return h.requests.filter(function (r) { return r.url.indexOf('/api/search?') === 0; })
        .map(function (r) { return r.url; });
}

// openDialog answers a note page with the dialog open. global tells the
// page that the search over all notes is on, the same as the server does
// with OMN_SEARCH_GLOBAL.
async function openDialog(global, server) {
    const h = notePage({ preview: NOTE, note: 'dir/Note' });
    h.page.OMN_SEARCH_GLOBAL = !!global;
    if (server) h.server = server;
    h.press('[data-action="search"]');
    await h.settle();
    return h;
}

// pageAnswer is what the server answers for a search in one note.
const pageAnswer = {
    status: 'ok', scope: 'page', highlight: ['alpha'],
    results: [{
        name: 'dir/Note.md', title: 'Note', url: '/dir/Note.html',
        matches: [
            { line: 3, text: 'The alpha line of the note.', spans: [[4, 5]], section: { id: 'first', label: 'First part' } },
            { line: 7, text: 'Another alpha here, and a beta.', spans: [[8, 5]], section: { id: 'second', label: 'Second part' } },
        ],
    }],
};

// allAnswer is what the server answers for a search over all notes.
const allAnswer = {
    status: 'ok', scope: 'all', highlight: ['alpha'], total: 2,
    results: [
        {
            name: 'dir/Note.md', title: 'Note', url: '/dir/Note.html',
            matches: [{ line: 3, text: 'The alpha line', spans: [[4, 5]], section: { id: 'first', label: 'First part' } }],
        },
        {
            name: 'Other.md', title: '', url: '/Other.html#top',
            matches: [{ line: 9, text: 'alpha = 1', spans: [[0, 5]], context: 'script' }],
        },
    ],
};

function rows(h) {
    return h.document.querySelectorAll('.omn-search-results .omn-search-row');
}

function status(h) {
    return h.$('.omn-search-status').textContent;
}

test('the search control loads its file and opens the dialog with the focus in the box', async () => {
    const h = await openDialog(false);
    assert.deepStrictEqual(h.loads, ['/js/OMN-Go/omn-go-search.js']);
    // The panel has a stylesheet of its own, and the load waits for it.
    assert.deepStrictEqual(h.styles, ['/css/OMN-Go/omn-go-search.css']);
    assert.strictEqual(h.$('.omn-search-overlay').hidden, false);
    assert.strictEqual(h.document.activeElement, h.$('.omn-search-input'));
    assert.strictEqual(status(h), 'Type at least 2 characters');
    // With no search over all notes, the dialog has one scope and names
    // the note.
    assert.strictEqual(h.$('.omn-search-input').placeholder, 'Search this page');
    assert.strictEqual(h.document.querySelectorAll('.omn-search-chip').length, 1);
    assert.strictEqual(h.$('.omn-search-where').textContent, 'dir/Note.md');
});

test('the dialog sends no request while the person types', async () => {
    const h = await openDialog(false, function () { return pageAnswer; });
    h.type('.omn-search-input', 'a');
    assert.strictEqual(status(h), 'Type at least 2 characters');
    h.type('.omn-search-input', 'al');
    h.type('.omn-search-input', 'alp');
    await h.settle();
    assert.deepStrictEqual(searches(h), []);
    assert.strictEqual(status(h), 'Press ↵ or the magnifier to search');
});

test('Enter sends the query with the name of the note, and the rows show the lines', async () => {
    const h = await openDialog(false, function () { return pageAnswer; });
    h.type('.omn-search-input', '  alpha ');
    const enter = h.key('.omn-search-input', 'Enter');
    assert.strictEqual(enter.defaultPrevented, true);
    assert.strictEqual(h.$('.omn-search-progress').hidden, false, 'the bar does not show while the server works');
    await h.settle();

    assert.deepStrictEqual(searches(h), ['/api/search?snippets=10&q=alpha&on=dir%2FNote.md']);
    assert.strictEqual(h.$('.omn-search-progress').hidden, true);
    assert.strictEqual(rows(h).length, 2);
    // The section of a line shows in place of its number.
    assert.strictEqual(rows(h)[0].querySelector('.omn-search-line').textContent, '› First part');
    // The mark is the span that the server gave, and nothing more.
    const marks = rows(h)[0].querySelectorAll('mark');
    assert.deepStrictEqual(marks.map(function (m) { return m.textContent; }), ['alpha']);
    assert.strictEqual(rows(h)[0].querySelector('.omn-search-text').textContent, 'The alpha line of the note.');
    assert.ok(status(h).indexOf('2 matching lines') === 0, status(h));
    assert.strictEqual(rows(h)[0].classList.contains('omn-is-active'), true);
});

test('the magnifier sends the query too, and a short query sends nothing', async () => {
    const h = await openDialog(false, function () { return pageAnswer; });
    h.type('.omn-search-input', 'a');
    h.press('.omn-search-go');
    await h.settle();
    assert.deepStrictEqual(searches(h), []);
    h.type('.omn-search-input', 'alpha');
    h.press('.omn-search-go');
    await h.settle();
    assert.strictEqual(searches(h).length, 1);
});

test('the arrow keys move the active row, and they stop at the ends', async () => {
    const h = await openDialog(false, function () { return pageAnswer; });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    const active = function () {
        return rows(h).map(function (r) { return r.classList.contains('omn-is-active'); });
    };
    h.key('.omn-search-input', 'ArrowUp');
    assert.deepStrictEqual(active(), [true, false]);
    h.key('.omn-search-input', 'ArrowDown');
    assert.deepStrictEqual(active(), [false, true]);
    h.key('.omn-search-input', 'ArrowDown');
    assert.deepStrictEqual(active(), [false, true]);
});

test('Enter on a row of this page marks the words and closes the dialog', async () => {
    const h = await openDialog(false, function () { return pageAnswer; });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    h.key('.omn-search-input', 'ArrowDown');
    h.key('.omn-search-input', 'Enter');
    await h.settle();

    assert.strictEqual(h.$('.omn-search-overlay').hidden, true);
    // The second Enter is a choice, and not a second search.
    assert.strictEqual(searches(h).length, 1);
    const marks = h.document.querySelectorAll('#omn-preview mark');
    assert.deepStrictEqual(marks.map(function (m) { return m.textContent; }), ['alpha', 'alpha']);
    // The chosen line is the second one, and the page went to it.
    const current = h.document.querySelectorAll('#omn-preview .omn-search-hit-current');
    assert.strictEqual(current.length, 1);
    assert.strictEqual(current[0].closest('p').textContent, 'Another alpha here, and a beta.');
});

test('a changed query makes Enter search again', async () => {
    const h = await openDialog(false, function () { return pageAnswer; });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    h.type('.omn-search-input', 'beta');
    assert.strictEqual(status(h), 'Press ↵ or the magnifier to search');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    assert.strictEqual(searches(h).length, 2);
    assert.strictEqual(h.$('.omn-search-overlay').hidden, false);
});

test('a note that matches only in its title says so', async () => {
    const h = await openDialog(false, function () {
        return { status: 'ok', scope: 'page', results: [{ name: 'dir/Note.md', url: '/dir/Note.html', matches: [] }] };
    });
    h.type('.omn-search-input', 'note');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    assert.strictEqual(rows(h).length, 0);
    assert.ok(status(h).indexOf('title or tags') >= 0, status(h));
});

test('no match and a fault of the request each have a status text', async () => {
    const none = await openDialog(false, function () { return { status: 'ok', scope: 'page', results: [] }; });
    none.type('.omn-search-input', 'zzz');
    none.key('.omn-search-input', 'Enter');
    await none.settle();
    assert.strictEqual(status(none), 'No matches on this page');

    const down = await openDialog(false, function (r) {
        return r.url.indexOf('/api/search') === 0 ? new Error('connection refused') : {};
    });
    down.type('.omn-search-input', 'zzz');
    down.key('.omn-search-input', 'Enter');
    await down.settle();
    assert.strictEqual(status(down), 'Search failed');
    assert.strictEqual(down.$('.omn-search-progress').hidden, true, 'the bar stays after a fault');
});

test('with the search over all notes, the dialog starts there and lists each note', async () => {
    const h = await openDialog(true, function () { return allAnswer; });
    assert.strictEqual(h.$('.omn-search-input').placeholder, 'Search all notes');
    assert.deepStrictEqual(
        h.document.querySelectorAll('.omn-search-chip').map(function (c) { return c.textContent; }),
        ['All notes', 'This page']);

    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    const docs = h.document.querySelectorAll('.omn-search-doc');
    assert.strictEqual(docs.length, 2);
    // A note with no title shows its name.
    assert.strictEqual(docs[1].querySelector('.omn-search-doc-title').textContent, 'Other.md');
    assert.strictEqual(rows(h).length, 2);
    assert.strictEqual(rows(h)[1].querySelector('.omn-search-ctx').title, 'inside a <script> block');
    assert.ok(status(h).indexOf('2 results') === 0, status(h));
});

test('a row of another note opens that note with the words and the line', async () => {
    const h = await openDialog(true, function () { return allAnswer; });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    rows(h)[0].click();
    // hl is each word, hlt is the text of the line, and the fragment is
    // the section of the line.
    assert.deepStrictEqual(h.went, ['/dir/Note.html?hl=alpha&hlt=The%20alpha%20line#first']);
    assert.strictEqual(h.$('.omn-search-overlay').hidden, true);
});

test('a line of a script block gives no line text, and the fragment of the note stays', async () => {
    const h = await openDialog(true, function () { return allAnswer; });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    rows(h)[1].click();
    assert.deepStrictEqual(h.went, ['/Other.html?hl=alpha#top']);
});

test('a press on the head of a note opens the note with the words alone', async () => {
    const h = await openDialog(true, function () { return allAnswer; });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    h.document.querySelectorAll('.omn-search-doc')[0].click();
    assert.deepStrictEqual(h.went, ['/dir/Note.html?hl=alpha']);
});

test('the scope chips send the scope, and the same chip sends nothing', async () => {
    const h = await openDialog(true, function (r) {
        return r.url.indexOf('scope=page') >= 0 ? pageAnswer : allAnswer;
    });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    const chip = function (label) {
        return h.document.querySelectorAll('.omn-search-chip')
            .find(function (c) { return c.textContent === label; });
    };
    chip('This page').click();
    await h.settle();
    assert.ok(searches(h)[1].endsWith('&scope=page'), searches(h)[1]);
    assert.strictEqual(chip('This page').classList.contains('omn-is-active'), true);
    assert.strictEqual(h.$('.omn-search-input').placeholder, 'Search this page');

    chip('This page').click();
    await h.settle();
    assert.strictEqual(searches(h).length, 2);
});

test('"See all results" shows for all notes with a query, and opens the results page', async () => {
    const h = await openDialog(true, function () { return allAnswer; });
    const timers = [];
    h.page.setTimeout = function (fn, ms) { timers.push(ms); };
    assert.strictEqual(h.$('.omn-search-seeall').hidden, true);
    h.type('.omn-search-input', 'a b');
    assert.strictEqual(h.$('.omn-search-seeall').hidden, false);
    h.press('.omn-search-seeall');
    assert.deepStrictEqual(h.went, ['/OMNGoSearch.html?q=a%20b']);
    assert.strictEqual(h.$('.omn-search-overlay').hidden, true);
    // The overlay of a slow page waits 300 ms, thus a fast page shows none.
    assert.deepStrictEqual(timers, [300]);
});

test('a refusal of the server shows its text and goes back to this page', async () => {
    const h = await openDialog(true, function () {
        return { status: 'error', error: 'The search index is not ready' };
    });
    h.type('.omn-search-input', 'alpha');
    h.key('.omn-search-input', 'Enter');
    await h.settle();
    assert.strictEqual(status(h), 'The search index is not ready');
    assert.strictEqual(h.$('.omn-search-chip.omn-is-active').textContent, 'This page');
});

test('Escape, the close button and a press outside the card close the dialog', async () => {
    const h = await openDialog(false);
    const overlay = h.$('.omn-search-overlay');
    h.key('.omn-search-input', 'Escape');
    assert.strictEqual(overlay.hidden, true);

    h.press('[data-action="search"]');
    assert.strictEqual(overlay.hidden, false);
    h.press('.omn-search-close');
    assert.strictEqual(overlay.hidden, true);

    h.press('[data-action="search"]');
    // A press inside the card is not a press on the overlay.
    h.press('.omn-search-card');
    assert.strictEqual(overlay.hidden, false);
    overlay.click();
    assert.strictEqual(overlay.hidden, true);
});

test('Ctrl-K opens the dialog from the page, and Escape on the page closes it', async () => {
    const h = notePage({ preview: NOTE });
    const k = new Event('keydown', { bubbles: true, key: 'k', ctrlKey: true });
    h.document.body.dispatchEvent(k);
    assert.strictEqual(k.defaultPrevented, true, 'Ctrl-K must not reach the address bar of the browser');
    await h.settle();
    assert.strictEqual(h.$('.omn-search-overlay').hidden, false);

    const escape = new Event('keydown', { bubbles: true, key: 'Escape' });
    h.document.body.dispatchEvent(escape);
    assert.strictEqual(h.$('.omn-search-overlay').hidden, true);
});

test('the search control with the dialog open keeps the dialog and takes the focus', async () => {
    const h = await openDialog(false);
    h.type('.omn-search-input', 'alpha');
    h.$('.omn-search-close').focus();
    h.press('[data-action="search"]');
    assert.strictEqual(h.$('.omn-search-overlay').hidden, false);
    assert.strictEqual(h.document.activeElement, h.$('.omn-search-input'));
});
