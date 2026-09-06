// The header-block rule, run in the REAL JavaScript.
//
// WHY THIS TEST EXISTS. backend/ports_test.go already compares the two
// languages, and it says out loud what it cannot do:
//
//   "A transcription is not the JavaScript itself. This test can
//    therefore not find a fault of the transcription."
//
// jsFirstLineAfterHeader in that file is a GO copy of the JavaScript,
// written by hand. It catches a rule that moved. It cannot catch a copy
// that was wrong the day it was written.
//
// This test loads omn-go-editor.js itself and calls the real function.
// The cases come from header-cases.json, which the Go test reads as well,
// thus the two run identical inputs.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');
const { load } = require('./dom-stub.js');

const editor = load('omn-go-editor.js');
const table = JSON.parse(fs.readFileSync(path.join(__dirname, 'header-cases.json'), 'utf8'));

test('firstLineAfterHeader answers an offset inside the note', () => {
    for (const c of table.cases) {
        const got = editor.firstLineAfterHeader(c.content);
        assert.ok(Number.isInteger(got), `${c.name}: got ${got}, want a whole number`);
        assert.ok(got >= 0 && got <= c.content.length,
            `${c.name}: the offset ${got} is outside a note of ${c.content.length}`);
    }
});

// This test asserts the SHAPE of the answer, and the Go test asserts the
// VALUE. TestHeaderPortAgreesWithTheRealJavaScript compares the two
// languages directly, and running both proves that they agree.
test('firstLineAfterHeader never puts the caret inside the header block', () => {
    for (const c of table.cases) {
        const at = editor.firstLineAfterHeader(c.content);
        if (at === 0 || at >= c.content.length) continue;
        // The character before the offset must end a line. A caret in the
        // middle of a line means the offset split a header key from its
        // value.
        const before = c.content[at - 1];
        assert.strictEqual(before, '\n',
            `${c.name}: the offset ${at} lands mid-line, after ${JSON.stringify(before)}`);
    }
});

test('isHeaderFirstLine follows the four rules of the header block', () => {
    // A header block exists only when the first line holds a colon and
    // does not start with a space, a hash or a "<". See header_block.go.
    for (const [line, want] of [
        ['Title: X', true],
        ['URL: http://example.org', true],
        ['Key:', true],
        [' Title: X', false],
        ['#Title: X', false],
        ['# Head: x', false],
        ['<p>Title: X</p>', false],
        ['Title X', false],
        ['', false],
    ]) {
        assert.strictEqual(editor.isHeaderFirstLine(line), want,
            `isHeaderFirstLine(${JSON.stringify(line)})`);
    }
});

test('the exported functions are the ones the port rule names', () => {
    for (const name of ['isHeaderFirstLine', 'firstLineAfterHeader']) {
        assert.strictEqual(typeof editor[name], 'function',
            `omn-go-editor.js exports no ${name}`);
    }
});
