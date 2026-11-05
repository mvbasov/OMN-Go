// The view of the editor after an expansion, in the REAL JavaScript and on
// the markup of editor.html.
//
// THE FAULT THAT THIS FILE HOLDS. On a desktop browser a person pressed the
// toolbar button that expands an abbreviation. The text and the caret were
// correct, and the view went to the end of the note.
//
// The cause is the order of three steps. An assignment to .value moves the
// caret to the end of the text. A focus of a textarea that does not have it
// scrolls to the caret. The button had the focus, and the editor gave it
// back BEFORE it put the caret in place. A check in Chromium showed each
// step.
//
// mini-dom.js has no layout, thus this file models the one rule of the
// browser that matters. See watchView below. A model is not a browser. A
// check in Chromium measured the fault and the repair.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { editorPage } = require('./dom-page.js');

const END_OF_NOTE = 99999;

// longNote answers a note of many lines with one abbreviation in the
// middle, and the offset of the end of that abbreviation.
function longNote(abbr) {
    const above = [];
    for (let i = 0; i < 200; i++) above.push('line ' + i);
    const head = 'Title: Note\n\n' + above.join('\n') + '\n' + abbr;
    return { text: head + '\n' + above.join('\n') + '\n', caret: head.length };
}

// watchView gives the textarea the scroll rule of a browser: a focus of a
// textarea that does not have it scrolls to the caret. The caret of these
// tests is in the view at scrollTop 1000, or it is at the end of the note.
function watchView(h, ta) {
    const focus = ta.focus.bind(ta);
    ta.focus = function () {
        if (h.document.activeElement !== ta && ta.selectionStart === ta.value.length) {
            ta.scrollTop = END_OF_NOTE;
        }
        focus();
    };
    ta.scrollTop = 1000;
    ta.scrollLeft = 0;
}

// open answers the editor with the note loaded, the caret behind the
// abbreviation and the view at scrollTop 1000.
async function open(abbr) {
    const note = longNote(abbr);
    const h = await editorPage(note.text);
    const ta = h.$('#omn-editor-text');
    assert.strictEqual(ta.value, note.text, 'the editor did not load the note');
    ta.focus();
    ta.setSelectionRange(note.caret, note.caret);
    watchView(h, ta);
    return { h: h, ta: ta, caret: note.caret };
}

const EXPAND = '#omn-editor-tools button[title^="Expand an abbreviation"]';

test('the toolbar button expands an Emmet abbreviation and keeps the view', async () => {
    const { h, ta, caret } = await open('ul>li*2');
    // The press moves the focus to the button, the same as in a browser.
    h.$(EXPAND).focus();
    h.press(EXPAND);

    assert.ok(ta.value.indexOf('<ul>\n  <li></li>\n  <li></li>\n</ul>') >= 0, 'the abbreviation did not expand');
    // The caret is in the first empty pair of tags.
    const at = caret - 'ul>li*2'.length + '<ul>\n  <li>'.length;
    assert.strictEqual(ta.selectionStart, at);
    assert.strictEqual(ta.selectionEnd, at);
    assert.strictEqual(h.document.activeElement, ta, 'the textarea did not get the focus back');
    assert.strictEqual(ta.scrollTop, 1000, 'the view went to the end of the note');
});

test('the toolbar button expands a Markdown abbreviation and keeps the view', async () => {
    const { h, ta } = await open('*[buy milk');
    h.$(EXPAND).focus();
    h.press(EXPAND);
    assert.ok(ta.value.indexOf('\n* [ ] buy milk\n') >= 0, 'the abbreviation did not expand');
    assert.strictEqual(h.document.activeElement, ta);
    assert.strictEqual(ta.scrollTop, 1000, 'the view went to the end of the note');
});

test('Tab expands with the focus in the textarea and keeps the view', async () => {
    const { h, ta } = await open('ul>li*2');
    const tab = h.key('#omn-editor-text', 'Tab');
    assert.strictEqual(tab.defaultPrevented, true, 'Tab moved the focus away');
    assert.ok(ta.value.indexOf('<ul>\n  <li></li>') >= 0);
    assert.strictEqual(ta.scrollTop, 1000);
});

test('a browser that moves the view at the write gets it back', async () => {
    const { h, ta } = await open('ul>li*2');
    // This model moves the view each time a script writes the text.
    let text = ta.value;
    Object.defineProperty(ta, 'value', {
        get() { return text; },
        set(v) {
            text = String(v);
            ta.selectionStart = text.length;
            ta.selectionEnd = text.length;
            ta.scrollTop = END_OF_NOTE;
        },
    });
    h.key('#omn-editor-text', 'Tab');
    assert.ok(text.indexOf('<ul>\n  <li></li>') >= 0);
    assert.strictEqual(ta.scrollTop, 1000);
});

test('Tab with no abbreviation writes a tab character and keeps the view', async () => {
    const { h, ta, caret } = await open('plain text ');
    h.key('#omn-editor-text', 'Tab');
    assert.strictEqual(ta.value.slice(caret, caret + 1), '\t');
    assert.strictEqual(ta.selectionStart, caret + 1);
    assert.strictEqual(ta.scrollTop, 1000);
});

test('an expansion marks the note as changed', async () => {
    const { h } = await open('ul>li*2');
    const dot = h.$('.omn-editor-dot');
    assert.strictEqual(dot.classList.contains('omn-dirty'), false);
    h.key('#omn-editor-text', 'Tab');
    assert.strictEqual(dot.classList.contains('omn-dirty'), true);
});
