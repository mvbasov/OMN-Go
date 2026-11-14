// The actions of the controls, run in the REAL JavaScript.
//
// A control of index.html or modals.html has no inline onclick. It names
// its work in data-action, and ONE click listener of omn-go-core.js calls
// the function of that name. See OMN.action in that file.
//
// EACH TEST LOADS THE SCRIPTS OF index.html, IN THE ORDER OF index.html. The
// list comes from the template, thus a script that the shell gets later is
// in these tests at once. A script that throws while it loads fails each
// test.
//
// A FAULT HERE IS A DEAD BUTTON. Nothing throws, and no test of the server
// sees it. These tests send a click to the real listener and read what it
// called.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const path = require('path');
const { newPage, run, makeElement } = require('./page-stub.js');
const { shellScripts } = require('./dom-page.js');

// loadPage answers a page with the scripts of each note page, and the
// click listeners that they gave to the document.
function loadPage(protocol) {
    const page = newPage();
    if (protocol) page.location.protocol = protocol;
    // A console of its own. omn-go-console.js puts a hook on each console
    // method, and the console of Node is one object for all the tests.
    page.console = { log() {}, info() {}, warn() {}, error() {}, debug() {} };
    const clicks = [];
    page.document.addEventListener = function (type, fn) {
        if (type === 'click') clicks.push(fn);
    };
    for (const file of shellScripts()) {
        const err = run(page, file);
        assert.strictEqual(err, null, file + ' did not load: ' + (err && err.stack));
    }
    return { page, clicks };
}

// control answers an element with a data-action, and with a data-arg when
// arg is not undefined.
function control(name, arg, tagName) {
    const el = makeElement();
    el.tagName = tagName || 'BUTTON';
    el.getAttribute = function (attr) {
        if (attr === 'data-action') return name;
        if (attr === 'data-arg') return arg === undefined ? null : arg;
        return null;
    };
    return el;
}

// click sends one click on a child of el to each click listener. It
// answers the event.
function click(clicks, el) {
    const event = {
        button: 0, defaultPrevented: false,
        target: { closest: function (sel) { return sel === '[data-action]' ? el : null; } },
        preventDefault: function () { this.defaultPrevented = true; },
    };
    for (const fn of clicks) fn(event);
    return event;
}

test('a click calls the function of the action with the control', () => {
    const { page, clicks } = loadPage();
    const seen = [];
    page.OMN.action('probe', function (el, event) { seen.push([el, event]); });
    const el = control('probe');
    const event = click(clicks, el);
    assert.strictEqual(seen.length, 1, 'the action ran ' + seen.length + ' times');
    assert.strictEqual(seen[0][0], el);
    assert.strictEqual(seen[0][1], event);
});

test('a click outside each control calls nothing', () => {
    const { page, clicks } = loadPage();
    let calls = 0;
    page.OMN.action('probe', function () { calls++; });
    click(clicks, null);
    assert.strictEqual(calls, 0);
});

test('an action with no function writes a warning and does not throw', () => {
    const { page, clicks } = loadPage();
    const warnings = [];
    page.console.warn = function (m) { warnings.push(m); };
    click(clicks, control('no-such-action'));
    assert.strictEqual(warnings.length, 1);
    assert.ok(warnings[0].indexOf('no-such-action') >= 0, warnings[0]);
});

test('a link with an action does not follow its href', () => {
    const { page, clicks } = loadPage();
    page.OMN.action('probe', function () {});
    assert.strictEqual(click(clicks, control('probe', undefined, 'A')).defaultPrevented, true);
    assert.strictEqual(click(clicks, control('probe')).defaultPrevented, false);
});

test('a lazy action loads its file one time and then calls the action of the file', async () => {
    const { page, clicks } = loadPage();
    // The script element that omnLoadModule adds to the head. The test
    // gives the action in place of the real file, and then reports the load.
    const loads = [];
    const seen = [];
    page.document.head.appendChild = function (el) {
        loads.push(el.src);
        setTimeout(function () {
            page.OMN.action('sync', function (control) {
                seen.push(control.getAttribute('data-arg'));
            });
            el.onload();
        }, 0);
    };
    click(clicks, control('sync', 'download'));
    click(clicks, control('sync', 'upload'));
    await new Promise(function (resolve) { setTimeout(resolve, 20); });
    assert.deepStrictEqual(loads, ['/js/OMN-Go/omn-go-sync.js']);
    assert.deepStrictEqual(seen, ['download', 'upload']);
    // The third click finds the action of the file, and no stub.
    click(clicks, control('sync', 'download'));
    assert.deepStrictEqual(seen, ['download', 'upload', 'download']);
    assert.strictEqual(loads.length, 1);
});

test('a lazy file that gives no action writes a console fault', async () => {
    const { page, clicks } = loadPage();
    const faults = [];
    page.console.error = function (m) { faults.push(m); };
    page.document.head.appendChild = function (el) { setTimeout(el.onload, 0); };
    click(clicks, control('bookmark-panel'));
    await new Promise(function (resolve) { setTimeout(resolve, 20); });
    assert.strictEqual(faults.length, 1);
    assert.ok(faults[0].indexOf('bookmark-panel') >= 0, faults[0]);
});

// shellActions answers each data-action of index.html and modals.html.
function shellActions() {
    const names = {};
    for (const file of ['index.html', 'modals.html']) {
        const src = fs.readFileSync(path.join(__dirname, '..', 'templates', file), 'utf8');
        const re = /data-action="([^"]+)"/g;
        let m;
        while ((m = re.exec(src)) !== null) names[m[1]] = true;
    }
    return Object.keys(names);
}

test('each control of the page shell has an action, from the server and from disk', () => {
    const names = shellActions();
    // The floor shows that the scan still reads the templates. It is not
    // the number of the actions.
    assert.ok(names.length >= 10, 'the scan found ' + names.length + ' actions in the shell');
    for (const protocol of ['http:', 'file:']) {
        const { page } = loadPage(protocol);
        for (const name of names) {
            assert.strictEqual(typeof page.OMN.action(name), 'function',
                'a page of ' + protocol + ' has no action ' + name);
        }
    }
});

test('the copy control copies the quick note and writes the result on itself', () => {
    const { page, clicks } = loadPage();
    const text = makeElement();
    text.value = 'a quick note';
    let selected = false;
    text.select = function () { selected = true; };
    text.setSelectionRange = function () {};
    page.document.getElementById = function (id) { return id === 'omn-quick-text' ? text : null; };
    const commands = [];
    page.document.execCommand = function (name) { commands.push(name); return true; };
    const el = control('quick-note-copy');
    el.textContent = 'Copy';
    click(clicks, el);
    assert.strictEqual(selected, true, 'the text was not selected');
    assert.deepStrictEqual(commands, ['copy']);
    assert.strictEqual(el.textContent, 'Copied!');
    // The timer that puts the label back must not keep the test process
    // alive.
    clearTimeout(el._omnCopyTimer);

    // An empty note copies nothing.
    text.value = '';
    click(clicks, el);
    assert.deepStrictEqual(commands, ['copy']);
    assert.strictEqual(el.textContent, 'Empty');
    clearTimeout(el._omnCopyTimer);
});

test('the title opens and closes the header', () => {
    const { page, clicks } = loadPage();
    const classes = { 'omn-hidden': true };
    const header = makeElement();
    header.classList = {
        contains: function (c) { return !!classes[c]; },
        add: function (c) { classes[c] = true; },
        remove: function (c) { delete classes[c]; },
    };
    const arrow = makeElement();
    page.document.getElementById = function (id) {
        return id === 'omn-hidable-header' ? header : id === 'omn-title-arrow' ? arrow : null;
    };
    click(clicks, control('toggle-header'));
    assert.strictEqual(classes['omn-hidden'], undefined, 'the first click did not open the header');
    assert.strictEqual(arrow.textContent, '\u2212');
    click(clicks, control('toggle-header'));
    assert.strictEqual(classes['omn-hidden'], true, 'the second click did not close the header');
    assert.strictEqual(arrow.textContent, '+');
});

test('the new page control sends the name to /api/newpage', async () => {
    const { page, clicks } = loadPage();
    // The control asks with one form of omn-go-dialog.js. The person typed
    // the title and kept the file name that came from it.
    const asked = [];
    page.omnDialog = async function (opts) {
        asked.push(opts);
        return { value: true, input: 'my new note', inputs: ['my new note', 'MyNewNote'] };
    };
    page.currentNote = 'dir/Here';
    const requests = [];
    page.fetch = async function (url, opts) {
        requests.push([url, opts.method, String(opts.body)]);
        return { ok: true, text: async function () { return 'dir/MyNewNote'; } };
    };
    click(clicks, control('new-page'));
    await new Promise(function (resolve) { setTimeout(resolve, 10); });
    assert.deepStrictEqual(requests, [[
        '/api/newpage', 'POST', 'source=dir%2FHere&target=MyNewNote&title=my+new+note',
    ]]);
    assert.strictEqual(page.location.href, '/dir/MyNewNote.html?edit=true');
    assert.strictEqual(asked.length, 1, 'the control asks more than one time');
    // The array comes from the realm of the page, thus the test compares text.
    assert.strictEqual(asked[0].fields.map(function (f) { return f.label; }).join('|'), 'Title|File name');
});

test('no internal function of a control is a name of window', () => {
    const { page } = loadPage();
    for (const name of ['toggleHeader', 'toggleQuickPanel', 'copyQuickNote',
        'createNoteShortcut', 'updateArrow', 'login', 'createNewPage',
        'submitQuickNote', 'submitBookmark', 'runSync', 'syncAction',
        'performSync', 'performPushForce', 'hidePushConflictModal',
        'previewAndCommit', 'commitAndUpload', 'hideCommitModal',
        'toggleBookmarkPanel']) {
        assert.strictEqual(typeof page[name], 'undefined', 'window.' + name + ' is back');
    }
});

test('hide-panel and toggle-panel work on the panel of data-arg', () => {
    const { page, clicks } = loadPage();
    const calls = [];
    const panel = makeElement();
    panel.classList = {
        add: function (c) { calls.push('add ' + c); },
        toggle: function (c) { calls.push('toggle ' + c); },
    };
    page.document.getElementById = function (id) { return id === 'omn-quick-panel' ? panel : null; };
    click(clicks, control('hide-panel', 'omn-quick-panel'));
    click(clicks, control('toggle-panel', 'omn-quick-panel'));
    // An id that the page does not have must not throw. An exported page
    // has no modal.
    click(clicks, control('hide-panel', 'absent'));
    assert.deepStrictEqual(calls, ['add omn-hidden', 'toggle omn-hidden']);
});

test('on a page from disk a server action does not throw', () => {
    const { page, clicks } = loadPage('file:');
    const debug = [];
    page.printDebug = function (name) { debug.push(name); };
    const loads = [];
    page.document.head.appendChild = function (el) { loads.push(el.src); };
    click(clicks, control('sync', 'download'));
    click(clicks, control('new-page'));
    assert.deepStrictEqual(debug, ['sync', 'new-page']);
    // No lazy file loads on such a page.
    assert.deepStrictEqual(loads, []);
});

test('replace-location opens the address of data-arg', () => {
    const { page, clicks } = loadPage();
    const seen = [];
    page.location.replace = function (url) { seen.push(url); };
    click(clicks, control('replace-location', '/Config.html'));
    assert.deepStrictEqual(seen, ['/Config.html']);
});

test('index.html names the six application scripts in their order', () => {
    assert.deepStrictEqual(shellScripts(), [
        'omn-go-console.js', 'omn-go-core.js', 'omn-go-highlight.js',
        'omn-go-nav.js', 'omn-go-share.js', 'omn-go-api.js',
    ]);
});
