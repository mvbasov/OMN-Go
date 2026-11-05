// The Log page, in the REAL JavaScript and on the markup of
// logs_page.html.
//
// logs.test.js holds the filter rule as a function. This file holds the
// page: the load, the new lines, the boxes of the filter and the copy.
//
// The page opens NO second log stream. It gets each new line from
// window.omnGoOnServerLog of omn-go-api.js, which each page already has.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { systemPage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const LINES = [
    '2026/10/02 10:00:00 [sync] (info) Sync started\n',
    '2026/10/02 10:00:01 [sync] (debug) Opening repo\n',
    '2026/10/02 10:00:02 [assets] (error) cannot read <b>x</b>\n',
    'a line with no tag and no level\n',
];

// logServer answers the history with a copy of lines. A real answer is
// new JSON each time, and the page adds its new lines to the list that it
// got.
function logServer(lines) {
    return function (request) {
        if (request.url === '/api/logs/history') return { lines: lines.slice() };
        return { status: 'success' };
    };
}

async function open(lines) {
    const h = systemPage('logs_page.html', 'omn-go-logs.js', logServer(lines === undefined ? LINES : lines),
        { note: 'OMNGoLogs' });
    await h.settle();
    return h;
}

function shown(h) {
    return h.document.querySelectorAll('#omn-lg-body .omn-lg-row').map(function (r) { return r.textContent; });
}

// box answers the checkbox of the filter with this name.
function box(h, group, name) {
    const label = h.document.querySelectorAll('#' + group + ' label.omn-lg-box')
        .find(function (l) { return l.querySelector('span').textContent === name; });
    assert.ok(label, 'the filter has no box "' + name + '"');
    return label.querySelector('input');
}

// set gives a box its state and sends the change event, the same as a
// press does.
function set(input, checked) {
    input.checked = checked;
    input.dispatchEvent(new Event('change', { bubbles: true }));
}

test('the page loads the history and shows each line as text', async () => {
    const h = await open();
    assert.deepStrictEqual(shown(h), [
        '2026/10/02 10:00:00 [sync] (info) Sync started',
        '2026/10/02 10:00:01 [sync] (debug) Opening repo',
        '2026/10/02 10:00:02 [assets] (error) cannot read <b>x</b>',
        'a line with no tag and no level',
    ]);
    assert.strictEqual(h.$('#omn-lg-count').textContent, '4 of 4');
    // A line of the log can hold a path or a note name. It must not
    // become markup.
    assert.strictEqual(h.document.querySelectorAll('#omn-lg-body b').length, 0);
    // The level of a line is a class of its row.
    const classes = h.document.querySelectorAll('#omn-lg-body .omn-lg-row').map(function (r) { return r.className; });
    assert.deepStrictEqual(classes, ['omn-lg-row omn-lg-info', 'omn-lg-row omn-lg-debug', 'omn-lg-row omn-lg-error', 'omn-lg-row']);
});

test('the page opens no log stream of its own', async () => {
    const h = await open();
    // The one stream is the stream of omn-go-api.js.
    assert.strictEqual(h.streams.length, 1);
});

test('a new line of the server shows at once', async () => {
    const h = await open();
    h.serverLog('2026/10/02 10:00:05 [search] (info) index ready');
    assert.strictEqual(shown(h).length, 5);
    assert.strictEqual(shown(h)[4], '2026/10/02 10:00:05 [search] (info) index ready');
    assert.strictEqual(h.$('#omn-lg-count').textContent, '5 of 5');
});

test('the filter has a box for each level and for each tag of the lines', async () => {
    const h = await open();
    const names = function (group) {
        return h.document.querySelectorAll('#' + group + ' label.omn-lg-box span')
            .map(function (s) { return s.textContent; });
    };
    assert.deepStrictEqual(names('omn-lg-levels'), ['error', 'info', 'debug']);
    // The tags are in the order of the alphabet.
    assert.deepStrictEqual(names('omn-lg-tags'), ['assets', 'sync']);
    // A tag that arrives later gets a box of its own.
    h.serverLog('2026/10/02 10:00:05 [search] (info) index ready');
    assert.deepStrictEqual(names('omn-lg-tags'), ['assets', 'search', 'sync']);
});

test('the Filter button shows and hides the boxes', async () => {
    const h = await open();
    assert.strictEqual(h.hidden('#omn-lg-filter'), true);
    h.press('#omn-lg-filter-toggle');
    assert.strictEqual(h.hidden('#omn-lg-filter'), false);
    h.press('#omn-lg-filter-toggle');
    assert.strictEqual(h.hidden('#omn-lg-filter'), true);
});

test('a box of a level hides the lines of that level, and a line with no level stays', async () => {
    const h = await open();
    set(box(h, 'omn-lg-levels', 'debug'), false);
    assert.deepStrictEqual(shown(h), [
        '2026/10/02 10:00:00 [sync] (info) Sync started',
        '2026/10/02 10:00:02 [assets] (error) cannot read <b>x</b>',
        'a line with no tag and no level',
    ]);
    assert.strictEqual(h.$('#omn-lg-count').textContent, '3 of 4');
    set(box(h, 'omn-lg-levels', 'debug'), true);
    assert.strictEqual(shown(h).length, 4);
});

test('a box of a tag hides the lines of that tag, and a new tag keeps the choice', async () => {
    const h = await open();
    set(box(h, 'omn-lg-tags', 'sync'), false);
    assert.deepStrictEqual(shown(h), [
        '2026/10/02 10:00:02 [assets] (error) cannot read <b>x</b>',
        'a line with no tag and no level',
    ]);
    // A new tag draws the boxes again. The box of sync must stay off.
    h.serverLog('2026/10/02 10:00:05 [search] (info) index ready');
    assert.strictEqual(box(h, 'omn-lg-tags', 'sync').checked, false);
    assert.strictEqual(shown(h).length, 3);
    // A new line of the hidden tag does not show.
    h.serverLog('2026/10/02 10:00:06 [sync] (info) Sync done');
    assert.strictEqual(shown(h).length, 3);
    assert.strictEqual(h.$('#omn-lg-count').textContent, '3 of 6');
});

test('an empty log and a log with each line hidden say so', async () => {
    const empty = await open([]);
    assert.strictEqual(empty.$('#omn-lg-body .omn-lg-none').textContent, 'The log holds no line yet.');
    assert.strictEqual(empty.$('#omn-lg-count').textContent, '0 of 0');

    const h = await open([LINES[0], LINES[1]]);
    set(box(h, 'omn-lg-tags', 'sync'), false);
    assert.strictEqual(h.$('#omn-lg-body .omn-lg-none').textContent, 'Every line of the log is hidden by the filter.');
});

test('Copy copies only the lines that show', async () => {
    const h = await open();
    h.holdTimers();
    set(box(h, 'omn-lg-levels', 'debug'), false);
    h.press('#omn-lg-copy');
    await h.settle();
    assert.deepStrictEqual(h.clipboard, [
        '2026/10/02 10:00:00 [sync] (info) Sync started\n' +
        '2026/10/02 10:00:02 [assets] (error) cannot read <b>x</b>\n' +
        'a line with no tag and no level',
    ]);
    assert.strictEqual(h.$('#omn-lg-copy').textContent, 'Copied');
    // A second press while the button says "Copied" must not keep that
    // word as the label.
    h.press('#omn-lg-copy');
    await h.settle();
    h.runTimers();
    assert.strictEqual(h.$('#omn-lg-copy').textContent, 'Copy');
});

test('a copy that fails says so on the button', async () => {
    const h = await open();
    h.holdTimers();
    h.clipboardRefuses = true;
    h.document.execCommand = function () { return false; };
    h.press('#omn-lg-copy');
    await h.settle();
    assert.strictEqual(h.$('#omn-lg-copy').textContent, 'Cannot copy');
});

test('Reload reads the history again and keeps the filter', async () => {
    let lines = LINES;
    const h = systemPage('logs_page.html', 'omn-go-logs.js', function (request) {
        return request.url === '/api/logs/history' ? { lines: lines.slice() } : {};
    });
    await h.settle();
    set(box(h, 'omn-lg-levels', 'debug'), false);
    lines = LINES.concat(['2026/10/02 10:00:09 [sync] (debug) Staging\n']);
    h.press('#omn-lg-reload');
    await h.settle();
    assert.strictEqual(h.$('#omn-lg-count').textContent, '3 of 5');
    assert.strictEqual(box(h, 'omn-lg-levels', 'debug').checked, false);
});

test('a caller with no session and a fault of the server each get a message', async () => {
    const refused = systemPage('logs_page.html', 'omn-go-logs.js', function (request) {
        return request.url === '/api/logs/history' ? { httpStatus: 401, text: 'no' } : {};
    });
    await refused.settle();
    assert.strictEqual(refused.$('#omn-lg-body').textContent,
        'Cannot read the log: This page is for the admin of this device.');

    const fault = systemPage('logs_page.html', 'omn-go-logs.js', function (request) {
        return request.url === '/api/logs/history' ? { httpStatus: 500, text: 'x' } : {};
    });
    await fault.settle();
    assert.strictEqual(fault.$('#omn-lg-body').textContent, 'Cannot read the log: HTTP 500');
});
