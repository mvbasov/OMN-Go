// The find bar of the editor, in the REAL JavaScript and on the markup of
// editor.html. This file holds the count, the steps, the three modes and
// the two replace controls.
//
// WHY THIS FILE EXISTS. A replace rewrites the text of a note. A fault here
// changes characters that the person did not see, or it changes the wrong
// match. Each test below reads the text of the textarea after the press.
//
// The bar waits 120 ms after a key before it searches. find() below holds
// the timers of the page and runs them, thus no test waits.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { editorPage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const NOTE = 'Title: Note\n\nfirst line\nsecond Line\nthird line\nlines\n';
const BODY = NOTE.indexOf('first line');

// docKey sends a keydown to the document, where the find shortcuts are.
function docKey(h, name, init) {
    const event = new Event('keydown', Object.assign({ bubbles: true, key: name }, init || {}));
    h.document.dispatchEvent(event);
    return event;
}

// open answers the editor with the find bar open.
async function open(text, opts) {
    const h = await editorPage(text === undefined ? NOTE : text, opts);
    h.holdTimers();
    docKey(h, 'f', { ctrlKey: true });
    return h;
}

// find types a query into the find box and runs the search.
function find(h, query) {
    h.type('#omn-find-input', query);
    h.runTimers();
    return h;
}

function count(h) { return h.$('#omn-find-count').textContent; }

// total answers the count of the matches, with no position. A press on a
// mode button searches again from the end of the selection. The position
// after that press is thus not the subject of a test of a mode.
function total(h) { return count(h).replace(/^\d+ \/ /, ''); }

// plain answers the text of a mark with no star.
function plain(mark) { return mark.replace(/^\*/, ''); }

// selected answers the text that the textarea has selected.
function selected(h) {
    const ta = h.$('#omn-editor-text');
    return ta.value.slice(ta.selectionStart, ta.selectionEnd);
}

// at answers the offset of the selection.
function at(h) { return h.$('#omn-editor-text').selectionStart; }

// marks answers the text of each mark of the mirror. The current match
// has a star.
function marks(h) {
    return h.$('#omn-editor-mirror').querySelectorAll('mark').map(function (m) {
        return (m.className === 'omn-is-current' ? '*' : '') + m.textContent;
    });
}

// ---------------------------------------------------------------------
// The bar
// ---------------------------------------------------------------------

test('Ctrl+F opens the bar with the find box in focus and no replace row', async () => {
    const h = await editorPage(NOTE);
    assert.strictEqual(h.$('#omn-editor-find').hidden, true);
    const event = docKey(h, 'f', { ctrlKey: true });
    assert.strictEqual(event.defaultPrevented, true, 'the find of the browser opens too');
    assert.strictEqual(h.$('#omn-editor-find').hidden, false);
    assert.strictEqual(h.document.activeElement, h.$('#omn-find-input'));
    assert.strictEqual(h.$('#omn-find-replace-row').hidden, true);
    assert.strictEqual(count(h), '');
});

test('Ctrl+H opens the bar with the replace row, and the device keeps it', async () => {
    const h = await editorPage(NOTE);
    const event = docKey(h, 'H', { metaKey: true });
    assert.strictEqual(event.defaultPrevented, true);
    assert.strictEqual(h.$('#omn-editor-find').hidden, false);
    assert.strictEqual(h.$('#omn-find-replace-row').hidden, false);
    assert.strictEqual(h.$('#omn-find-toggle-replace').title, 'Hide replace');
    assert.strictEqual(h.storage.omngo_find_replace, '1');
});

test('the toolbar button opens the bar', async () => {
    const h = await editorPage(NOTE);
    h.press('#omn-tool-find');
    assert.strictEqual(h.$('#omn-editor-find').hidden, false);
});

test('a selection in one line becomes the query', async () => {
    const h = await editorPage(NOTE);
    const ta = h.$('#omn-editor-text');
    ta.setSelectionRange(NOTE.indexOf('second'), NOTE.indexOf('second') + 6);
    docKey(h, 'f', { ctrlKey: true });
    assert.strictEqual(h.$('#omn-find-input').value, 'second');

    // A selection of more than one line is not a query.
    const g = await editorPage(NOTE);
    g.$('#omn-editor-text').setSelectionRange(BODY, BODY + 15);
    docKey(g, 'f', { ctrlKey: true });
    assert.strictEqual(g.$('#omn-find-input').value, '');
});

test('the chevron shows and hides the replace row', async () => {
    const h = await open();
    h.press('#omn-find-toggle-replace');
    assert.strictEqual(h.$('#omn-find-replace-row').hidden, false);
    assert.strictEqual(h.document.activeElement, h.$('#omn-replace-input'));
    h.press('#omn-find-toggle-replace');
    assert.strictEqual(h.$('#omn-find-replace-row').hidden, true);
    assert.strictEqual(h.$('#omn-find-toggle-replace').title, 'Show replace');
    assert.strictEqual(h.storage.omngo_find_replace, '0');
});

test('Escape and the close button hide the bar and give the textarea the focus', async () => {
    const ways = [
        function (h) { return docKey(h, 'Escape'); },
        function (h) { return h.key('#omn-find-input', 'Escape'); },
        function (h) { return h.key('#omn-replace-input', 'Escape'); },
        function (h) { h.press('#omn-find-close'); return null; },
    ];
    for (const close of ways) {
        const h = await open();
        find(h, 'line');
        assert.strictEqual(marks(h).length, 4);
        const event = close(h);
        if (event) assert.strictEqual(event.defaultPrevented, true);
        assert.strictEqual(h.$('#omn-editor-find').hidden, true);
        assert.strictEqual(h.document.activeElement, h.$('#omn-editor-text'));
        assert.deepStrictEqual(marks(h), [], 'the mirror keeps the marks of a closed bar');
        assert.strictEqual(selected(h), 'line', 'the caret left the last match');
    }
});

test('Escape with the bar closed stays with the browser', async () => {
    const h = await editorPage(NOTE);
    assert.strictEqual(docKey(h, 'Escape').defaultPrevented, false);
    assert.strictEqual(docKey(h, 'F3').defaultPrevented, false);
});

// ---------------------------------------------------------------------
// The search
// ---------------------------------------------------------------------

test('a query counts the matches and selects the first one after the caret', async () => {
    const h = await open();
    h.type('#omn-find-input', 'line');
    assert.strictEqual(count(h), '', 'the bar searched before the wait ended');
    h.runTimers();
    // The search ignores case, thus "Line" and "lines" are matches too.
    assert.strictEqual(count(h), '1 / 4');
    assert.strictEqual(at(h), NOTE.indexOf('line'));
    assert.deepStrictEqual(marks(h), ['*line', 'Line', 'line', 'line']);
    // The mirror holds the text of the note and one more line end.
    assert.strictEqual(h.$('#omn-editor-mirror').textContent, NOTE + '\n');
});

test('the mirror holds the note as text, and not as markup', async () => {
    const note = 'Title: Note\n\n<b>bold</b> & <i>bold</i>\n';
    const h = await open(note);
    find(h, 'bold');
    assert.strictEqual(h.$('#omn-editor-mirror').querySelectorAll('b').length, 0, 'the mirror ran the markup of the note');
    assert.strictEqual(h.$('#omn-editor-mirror').textContent, note + '\n');
});

test('a query with no match and an empty query say so', async () => {
    const h = await open();
    find(h, 'absent');
    assert.strictEqual(count(h), 'no matches');
    assert.deepStrictEqual(marks(h), []);
    h.key('#omn-find-input', 'Enter');
    assert.strictEqual(count(h), 'no matches');
    find(h, '');
    assert.strictEqual(count(h), '');
});

test('Enter, F3 and the buttons step through the matches and wrap', async () => {
    const h = await open();
    find(h, 'line');
    const enter = h.key('#omn-find-input', 'Enter');
    assert.strictEqual(enter.defaultPrevented, true);
    assert.strictEqual(count(h), '2 / 4');
    assert.strictEqual(selected(h), 'Line');
    h.press('#omn-find-next');
    assert.strictEqual(count(h), '3 / 4');
    assert.strictEqual(docKey(h, 'F3').defaultPrevented, true);
    assert.strictEqual(count(h), '4 / 4');
    docKey(h, 'g', { ctrlKey: true });
    assert.strictEqual(count(h), '1 / 4', 'the step did not wrap to the first match');

    h.key('#omn-find-input', 'Enter', { shiftKey: true });
    assert.strictEqual(count(h), '4 / 4', 'the step did not wrap to the last match');
    h.press('#omn-find-prev');
    assert.strictEqual(count(h), '3 / 4');
    docKey(h, 'F3', { shiftKey: true });
    assert.strictEqual(count(h), '2 / 4');
    assert.deepStrictEqual(marks(h), ['line', '*Line', 'line', 'line']);
});

test('a step starts at the caret, and not at the last match', async () => {
    const h = await open();
    find(h, 'line');
    const third = NOTE.indexOf('third');
    h.$('#omn-editor-text').setSelectionRange(third, third);
    h.press('#omn-find-next');
    assert.strictEqual(count(h), '3 / 4');
    assert.strictEqual(at(h), NOTE.indexOf('line', third));
});

test('a step moves the view to the match', async () => {
    const lines = [];
    for (let i = 0; i < 50; i++) lines.push('row ' + i);
    const h = await open('Title: Note\n\n' + lines.join('\n') + '\ntarget\n');
    find(h, 'target');
    // The match is on the line with the index 52. Three lines stay above.
    assert.strictEqual(h.$('#omn-editor-text').scrollTop, 49 * 18);
    assert.strictEqual(h.$('#omn-editor-mirror').scrollTop, 49 * 18, 'the marks are not behind their words');
});

test('the count follows the text while the person types in the note', async () => {
    const h = await open();
    find(h, 'line');
    h.type('#omn-editor-text', NOTE + 'one more line\n');
    h.runTimers();
    assert.match(count(h), / \/ 5$/);
});

test('a note with very many matches gets a count with a plus', async () => {
    const h = await open('Title: Note\n\n' + 'ab '.repeat(1200));
    find(h, 'ab');
    assert.strictEqual(count(h), '1 / 1000+');
    assert.strictEqual(marks(h).length, 1000);
});

// ---------------------------------------------------------------------
// The three modes
// ---------------------------------------------------------------------

test('Match case narrows the search, and the device keeps the mode', async () => {
    const h = await open();
    find(h, 'Line');
    assert.strictEqual(count(h), '1 / 4');
    h.press('#omn-find-case');
    assert.strictEqual(total(h), '1');
    assert.strictEqual(selected(h), 'Line');
    assert.strictEqual(h.$('#omn-find-case').classList.contains('omn-active'), true);
    assert.strictEqual(h.$('#omn-find-case').getAttribute('aria-pressed'), 'true');
    assert.strictEqual(h.storage.omngo_find_case, '1');
    h.press('#omn-find-case');
    assert.strictEqual(total(h), '4');
    assert.strictEqual(h.$('#omn-find-case').getAttribute('aria-pressed'), 'false');
});

test('Whole word refuses a match inside a word, in Cyrillic too', async () => {
    const h = await open('Title: Note\n\nline lines in-line\nприв привет\n');
    find(h, 'line');
    assert.strictEqual(count(h), '1 / 3');
    h.press('#omn-find-word');
    assert.strictEqual(total(h), '2');
    assert.deepStrictEqual(marks(h).map(plain), ['line', 'line']);
    assert.strictEqual(h.storage.omngo_find_word, '1');

    find(h, 'прив');
    assert.strictEqual(count(h), '1 / 1', 'the word limit does not know a Cyrillic letter');
});

test('a query is plain text until the person asks for a regular expression', async () => {
    const h = await open('Title: Note\n\na.c abc a.c\n');
    find(h, 'a.c');
    assert.strictEqual(count(h), '1 / 2');
    h.press('#omn-find-regex');
    assert.strictEqual(total(h), '3');
    assert.strictEqual(h.storage.omngo_find_regex, '1');
});

test('the bar starts with the modes that the device keeps', async () => {
    const h = await editorPage(NOTE, { storage: { omngo_find_regex: '1', omngo_find_word: '1', omngo_find_replace: '1' } });
    assert.strictEqual(h.$('#omn-find-regex').classList.contains('omn-active'), true);
    assert.strictEqual(h.$('#omn-find-word').getAttribute('aria-pressed'), 'true');
    assert.strictEqual(h.$('#omn-find-case').classList.contains('omn-active'), false);
    assert.strictEqual(h.$('#omn-find-replace-row').hidden, false);
    assert.strictEqual(h.$('#omn-editor-find').hidden, true, 'a kept mode opened the bar');
});

test('a pattern that does not compile shows as bad, and Replace does nothing', async () => {
    const h = await open(NOTE, { storage: { omngo_find_regex: '1', omngo_find_replace: '1' } });
    assert.strictEqual(h.$('#omn-find-regex').classList.contains('omn-active'), true, 'the bar lost the mode of the device');
    find(h, '(line');
    assert.strictEqual(count(h), 'bad pattern');
    assert.strictEqual(h.$('#omn-find-input').classList.contains('omn-is-invalid'), true);
    h.press('#omn-replace-one');
    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-text').value, NOTE);

    find(h, 'line');
    assert.strictEqual(h.$('#omn-find-input').classList.contains('omn-is-invalid'), false);
});

test('a pattern that can match nothing does not hang the page', async () => {
    const h = await open('Title: Note\n\nxx y xxx\n', { storage: { omngo_find_regex: '1' } });
    find(h, 'x*');
    assert.strictEqual(count(h), '1 / 2');
    assert.deepStrictEqual(marks(h), ['*xx', 'xxx']);
});

// ---------------------------------------------------------------------
// Replace
// ---------------------------------------------------------------------

// openReplace answers the editor with the bar and the replace row open.
async function openReplace(text, query, replacement, opts) {
    const h = await open(text, opts);
    h.press('#omn-find-toggle-replace');
    h.$('#omn-replace-input').value = replacement;
    return find(h, query);
}

test('Replace changes the selected match and goes to the next one', async () => {
    const h = await openReplace('Title: Note\n\none two one two one\n', 'one', '1');
    assert.strictEqual(count(h), '1 / 3');
    h.press('#omn-replace-one');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\n1 two one two one\n');
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Replaced 1 match');
    assert.strictEqual(h.$('#omn-editor-status').className, 'omn-editor-status-ok');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-dirty');
    assert.strictEqual(count(h), '1 / 2');
    assert.strictEqual(at(h), 'Title: Note\n\n1 two '.length);

    // Enter in the replace box is the same as the button.
    const enter = h.key('#omn-replace-input', 'Enter');
    assert.strictEqual(enter.defaultPrevented, true);
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\n1 two 1 two one\n');
});

test('Replace with no selected match only goes to a match', async () => {
    const h = await openReplace('Title: Note\n\none two one\n', 'one', '1');
    h.$('#omn-editor-text').setSelectionRange(0, 0);
    h.press('#omn-replace-one');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\none two one\n', 'Replace changed a match that the person did not see');
    assert.strictEqual(selected(h), 'one');
    h.press('#omn-replace-one');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\n1 two one\n');
});

test('Replace does not find its own output', async () => {
    const h = await openReplace('Title: Note\n\na a\n', 'a', 'aa', { storage: { omngo_find_case: '1' } });
    h.press('#omn-replace-one');
    h.press('#omn-replace-one');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\naa aa\n');
});

test('Replace all changes each match in one write', async () => {
    const h = await openReplace('Title: Note\n\none two one two one\n', 'one', '1');
    h.document.commands.length = 0;
    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\n1 two 1 two 1\n');
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Replaced 3 matches');
    assert.strictEqual(count(h), 'no matches');
    assert.deepStrictEqual(marks(h), []);
    // One insertText command is one step of the undo history.
    assert.deepStrictEqual(h.document.commands, ['insertText']);

    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'No matches to replace');
});

test('Replace all of one match says "match"', async () => {
    const h = await openReplace('Title: Note\n\none two\n', 'two', '2');
    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-status').textContent, 'Replaced 1 match');
});

test('a browser with no insertText command gets the same text', async () => {
    const h = await openReplace('Title: Note\n\none two one\n', 'one', '1', { noInsertText: true });
    h.press('#omn-replace-one');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\n1 two one\n');
    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\n1 two 1\n');
    assert.strictEqual(h.$('#omn-editor-dot').className, 'omn-editor-dot omn-dirty');
});

test('a dollar sign in the replacement is plain text outside a regular expression', async () => {
    const h = await openReplace('Title: Note\n\nprice: ten\n', 'ten', '$1 & $& and $$');
    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\nprice: $1 & $& and $$\n');
});

test('a regular expression gives the replacement its groups', async () => {
    const h = await openReplace('Title: Note\n\njohn smith\nmary jones\n', '(\\w+) (\\w+)$', '$2, $1 [$&] $$ $7',
        { storage: { omngo_find_regex: '1' } });
    // The pattern has no "m" flag, thus only the last line is a match.
    assert.strictEqual(count(h), 'no matches');
    find(h, '(\\w+) (\\w+)');
    h.press('#omn-replace-all');
    assert.strictEqual(h.$('#omn-editor-text').value,
        'Title: Note\n\nsmith, john [john smith] $ $7\njones, mary [mary jones] $ $7\n');
});

test('Replace of one match uses the same groups as Replace all', async () => {
    const h = await openReplace('Title: Note\n\nab ab\n', '(a)(b)', '$2$1', { storage: { omngo_find_regex: '1' } });
    h.press('#omn-replace-one');
    assert.strictEqual(h.$('#omn-editor-text').value, 'Title: Note\n\nba ab\n');
});
