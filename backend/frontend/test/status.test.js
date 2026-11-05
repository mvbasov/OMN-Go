// The Status page, in the REAL JavaScript and on the markup of
// status_page.html.
//
// The page reads /api/status. Two sections cost time on a large storage
// directory: the storage counts and the git worktree state. THE PAGE ASKS
// FOR THEM ONLY ON A PRESS. A page that asks for them at each load makes
// each open of the Status page slow.
//
// Each value is written as text. A path or a remote address of the device
// must not become markup.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { systemPage } = require('./dom-page.js');

// answer is what the server answers for the sections of a first load.
const answer = {
    generated: '2026-10-02T10:00:00Z',
    server: { version: '26.10.15', address: '127.0.0.1:8080', share_lan: false, uptime_seconds: 12 },
    config: { theme: 'dark', search_kinds: ['md', 'json'], log_tags: [] },
    git: { remote: 'git@example.org:<b>me</b>/notes.git', branch: 'master' },
    runtime: { heap_alloc: 5242880, sys: 512, goroutines: 9 },
};

const storage = { storage: { md: { files: 3, bytes: 2048 }, html: { files: 1, bytes: 100 } } };

// statusServer answers /api/status by its sections, and it keeps each
// address in seen.
function statusServer(seen, more) {
    return function (request) {
        if (request.url.indexOf('/api/status') !== 0) return { status: 'success' };
        seen.push(request.url);
        if (request.url.indexOf('format=md') >= 0) return { httpStatus: 200, text: '# Status\n\n| a | b |' };
        if (request.url.indexOf('sections=storage') >= 0 && request.url.indexOf(',') < 0) return storage;
        return Object.assign({}, answer, more || {});
    };
}

async function open(seen, more) {
    const h = systemPage('status_page.html', 'omn-go-status.js', statusServer(seen, more), { note: 'OMNGoStatus' });
    await h.settle();
    return h;
}

// rows answers the rows of the page as "key = value".
function rows(h) {
    return h.document.querySelectorAll('#omn-st-body tr').map(function (tr) {
        return tr.querySelector('.omn-st-key').textContent + ' = ' + tr.querySelector('.omn-st-val').textContent;
    });
}

function heads(h) {
    return h.document.querySelectorAll('#omn-st-body h3').map(function (e) { return e.textContent; });
}

test('the first load asks for no section by name and shows each section that came', async () => {
    const seen = [];
    const h = await open(seen);
    // No "sections": the server then gives the sections that cost no time.
    assert.deepStrictEqual(seen, ['/api/status']);
    assert.deepStrictEqual(heads(h), ['Server', 'Configuration', 'Git', 'Runtime']);
    assert.strictEqual(h.$('.omn-st-note').textContent, 'Generated 2026-10-02T10:00:00Z');
});

test('each kind of value has its form', async () => {
    const h = await open([]);
    const all = rows(h);
    assert.ok(all.indexOf('version = 26.10.15') >= 0, all.join('\n'));
    assert.ok(all.indexOf('share_lan = no') >= 0, 'a boolean must read as yes or no');
    assert.ok(all.indexOf('search_kinds = md, json') >= 0, 'a list must read as its items');
    assert.ok(all.indexOf('log_tags = —') >= 0, 'an empty list must read as a dash');
    assert.ok(all.indexOf('heap_alloc = 5.0 MB (5242880 bytes)') >= 0, 'a large byte count has no unit');
    assert.ok(all.indexOf('sys = 512 bytes') >= 0, 'a small byte count has no unit');
    // A plain number that is not a byte count stays as it is.
    assert.ok(all.indexOf('goroutines = 9') >= 0);
});

test('a value with markup stays text', async () => {
    const h = await open([]);
    assert.ok(rows(h).indexOf('remote = git@example.org:<b>me</b>/notes.git') >= 0);
    assert.strictEqual(h.document.querySelectorAll('#omn-st-body b').length, 0);
});

test('the Storage button asks for its section only on a press, and adds it to the page', async () => {
    const seen = [];
    const h = await open(seen);
    assert.ok(heads(h).indexOf('Storage') < 0, 'the page shows the storage counts before a press');

    h.press('#omn-st-storage');
    assert.strictEqual(h.$('#omn-st-storage').disabled, true, 'a second press can start a second count');
    await h.settle();
    assert.strictEqual(h.$('#omn-st-storage').disabled, false);
    assert.deepStrictEqual(seen.slice(1), ['/api/status?sections=storage']);
    // The other sections stay.
    assert.deepStrictEqual(heads(h), ['Server', 'Configuration', 'Git', 'Runtime', 'Storage']);
    assert.ok(rows(h).indexOf('md = 3 files, 2.0 kB (2048 bytes)') >= 0, rows(h).join('\n'));
    assert.ok(rows(h).indexOf('html = 1 file, 100 bytes') >= 0);
    // The overlay shows while the server counts, and it is gone after.
    assert.strictEqual(h.$('.omn-progress-overlay').hidden, true);
});

test('Reload asks for each section that the person already asked for', async () => {
    const seen = [];
    const h = await open(seen);
    h.press('#omn-st-dirty');
    await h.settle();
    seen.length = 0;
    h.press('#omn-st-reload');
    await h.settle();
    assert.deepStrictEqual(seen, ['/api/status?sections=server%2Cconfig%2Cgit%2Csearch%2Cruntime%2Candroid%2Cgit_dirty']);
});

test('Copy asks for the Markdown form of the same sections and copies it', async () => {
    const seen = [];
    const h = await open(seen);
    h.holdTimers();
    seen.length = 0;
    h.press('#omn-st-copy');
    await h.settle();
    assert.deepStrictEqual(seen, ['/api/status?format=md&sections=server%2Cconfig%2Cgit%2Csearch%2Cruntime%2Candroid']);
    assert.deepStrictEqual(h.clipboard, ['# Status\n\n| a | b |']);
    assert.strictEqual(h.$('#omn-st-copy').textContent, 'Copied');
    // The label of the button comes again.
    assert.deepStrictEqual(h.pendingTimers(), [1400]);
    h.runTimers();
    assert.strictEqual(h.$('#omn-st-copy').textContent, 'Copy as Markdown');
});

test('a first load that fails says so', async () => {
    const h = systemPage('status_page.html', 'omn-go-status.js', function (request) {
        return request.url.indexOf('/api/status') === 0 ? { httpStatus: 500, text: 'fault' } : {};
    });
    await h.settle();
    assert.strictEqual(h.$('#omn-st-body .omn-st-error').textContent, 'Could not read the status: HTTP 500');
});

test('a later load that fails keeps the values of the page', async () => {
    const seen = [];
    let broken = false;
    const good = statusServer(seen);
    const h = systemPage('status_page.html', 'omn-go-status.js', function (request) {
        if (broken && request.url.indexOf('/api/status') === 0) return new Error('connection refused');
        return good(request);
    });
    await h.settle();
    broken = true;
    h.press('#omn-st-storage');
    await h.settle();
    assert.strictEqual(h.document.querySelector('#omn-st-body .omn-st-error'), null);
    assert.deepStrictEqual(heads(h), ['Server', 'Configuration', 'Git', 'Runtime']);
    assert.strictEqual(h.$('#omn-st-storage').disabled, false, 'the button stays off after a fault');
});

test('a copy that fails says so on the button', async () => {
    const h = await open([]);
    h.holdTimers();
    h.clipboardRefuses = true;
    h.document.execCommand = function () { return false; };
    h.press('#omn-st-copy');
    await h.settle();
    assert.strictEqual(h.$('#omn-st-copy').textContent, 'Copy failed');
});
