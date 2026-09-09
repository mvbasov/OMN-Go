// The filter of the Log page, run in the REAL JavaScript.
//
// logLineShows decides whether one line of the log reaches the screen. Two
// of its answers matter more than the rest.
//
// A LINE THAT DOES NOT PARSE ALWAYS SHOWS. Three call sites of the project
// write a log line with no level, because no application can reach them.
// See loadTemplate in templates.go and the two in search_sections.go. Each
// one is a fault. A filter that hides a fault is worse than no filter.
//
// AN UNKNOWN NAME SHOWS. The page builds its tag list from the lines that
// arrive. A tag that arrives after the reader ticked the boxes is
// therefore absent from the hidden set, and it must show.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { load } = require('./dom-stub.js');

const logs = load('omn-go-logs.js');

test('a line with no level always shows', () => {
    // null is what window.omnParseLogLine answers for such a line.
    assert.strictEqual(logs.logLineShows(null, { error: true }, { sync: true }), true);
    assert.strictEqual(logs.logLineShows(undefined, {}, {}), true);
});

test('a hidden level hides the line', () => {
    const parts = { tag: 'sync', level: 'debug' };
    assert.strictEqual(logs.logLineShows(parts, {}, {}), true);
    assert.strictEqual(logs.logLineShows(parts, { debug: true }, {}), false);
    assert.strictEqual(logs.logLineShows(parts, { info: true }, {}), true);
});

test('a hidden tag hides the line', () => {
    const parts = { tag: 'sync', level: 'error' };
    assert.strictEqual(logs.logLineShows(parts, {}, { sync: true }), false);
    assert.strictEqual(logs.logLineShows(parts, {}, { assets: true }), true);
});

test('a tag that the reader never met shows', () => {
    // The reader hid two tags. A third one arrives later, and the page has
    // no box for it yet.
    const hiddenTags = { sync: true, assets: true };
    const fresh = { tag: 'db-restore', level: 'info' };
    assert.strictEqual(logs.logLineShows(fresh, {}, hiddenTags), true);
});

test('the level and the tag are two separate reasons to hide', () => {
    const parts = { tag: 'sync', level: 'debug' };
    assert.strictEqual(logs.logLineShows(parts, { debug: true }, { sync: true }), false);
    assert.strictEqual(logs.logLineShows(parts, { debug: true }, {}), false);
    assert.strictEqual(logs.logLineShows(parts, {}, { sync: true }), false);
    assert.strictEqual(logs.logLineShows(parts, {}, {}), true);
});
