// The fold table, run in the REAL JavaScript.
//
// The Go matcher folds a character before it matches, and the page folds
// again before it marks. TestFoldTableHasAFrontendCopy in Go compares the
// two TABLES by reading the source. It does not run the page code.
//
// A row that changes the LENGTH of a string moves every span after it,
// and the marks then land on the wrong words. A table comparison cannot
// see that. This test runs omnFold and measures the length.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { load } = require('./dom-stub.js');

const core = load('omn-go-core.js');

test('omnFold never changes the length of a string', () => {
    // The marks are placed by rune offset. A fold that adds or drops a
    // character moves every offset after it. This is the property the
    // table comparison cannot check.
    for (const s of [
        'Ёлка', 'ёлка', 'ЕЛКА', 'straße', 'Ångström', 'ĲSSEL',
        'a', '', 'plain ascii text', 'ÅÄÖåäö', 'Привет мир',
    ]) {
        assert.strictEqual([...core.omnFold(s)].length, [...s].length,
            `omnFold(${JSON.stringify(s)}) changed the rune count`);
    }
});

test('omnFold folds the pair that the reported fault named', () => {
    // A search for "елка" must find "Ёлка". Before 26.08.79 the page held
    // no table, thus the server found the note and the page marked
    // nothing.
    assert.strictEqual(core.omnFold('Ёлка'), core.omnFold('елка'));
    assert.strictEqual(core.omnFold('ЁЛКА'), core.omnFold('елка'));
});

test('omnFoldChar answers one character for one character', () => {
    for (const c of ['Ё', 'ё', 'A', 'a', 'я', '1', ' ']) {
        const got = core.omnFoldChar(c);
        assert.strictEqual([...got].length, 1,
            `omnFoldChar(${JSON.stringify(c)}) answered ${JSON.stringify(got)}`);
    }
});
