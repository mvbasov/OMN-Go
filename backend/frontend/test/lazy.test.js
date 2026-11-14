// Each lazy file must define every name and give every action that
// omn-go-api.js promises, and it must read no name that nothing defines.
//
// THE FAULT THIS FILE EXISTS TO FIND. The body of omn-go-api.js sits inside
// an `if (protocol !== 'file:')` BLOCK, thus a `const` of that block reaches
// no other file. A lazy file that reads such a name throws a ReferenceError,
// for example "SYNC_TITLES is not defined" at each press of "Commit & Push".
// See doc/decisions/0015-load-the-click-driven-scripts-on-demand.md.
//
// WHY NOTHING CAUGHT IT.
//
//   - `node --check` parses a file. A free variable is valid syntax.
//   - A test that READS the source proves what a file says. This one RUNS
//     the code, which is the rule of CLAUDE.md section 8.
//   - dom-stub.js loads a script with require, and a Node module has its
//     own scope. In a browser `window` IS the global object. The stub
//     therefore reported the fault where a browser has none, and hid it
//     where a browser has one. page-stub.js builds a real page context.
//
// A SIBLING FAULT THAT THIS FILE ALSO COVERS. `applySyncLogLine` is a
// FUNCTION of the same block. Annex B of the standard hoists a function
// declared in a block of sloppy mode, thus omn-go-sync.js found it by
// accident for those same 17 versions. omn-go-api.js now exports it by
// hand. The test below cannot tell the two apart, and it does not need
// to: it fails on either one.
//
// WHAT A FAILURE MEANS. A ReferenceError names a variable that the lazy
// file reads and nothing defines. Move the definition into the lazy file,
// or export it from omn-go-api.js by hand.
//
// A TypeError is NOT a failure. The page here is a stub, and a missing
// element or method of the stub is a gap in the stub.

'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { newPage, run, source } = require('./page-stub.js');

// lazyMap reads the omnLazy and omnLazyActions calls of omn-go-api.js and
// answers {file: {names: [...], actions: [...]}}.
//
// It parses the REAL calls and holds no copy of the lists. A name added to
// omn-go-api.js is therefore covered by this test at once, and a hand
// written copy here would drift from it.
function lazyMap() {
    const src = source('omn-go-api.js');
    const out = {};
    const call = /(omnLazy|omnLazyActions)\(\s*'([^']+)'\s*,\s*\[([^\]]*)\]/g;
    let m;
    while ((m = call.exec(src)) !== null) {
        const list = m[3].split(',')
            .map((s) => s.trim().replace(/^'|'$/g, ''))
            .filter((s) => s.length > 0);
        if (!out[m[2]]) out[m[2]] = { names: [], actions: [] };
        const key = m[1] === 'omnLazy' ? 'names' : 'actions';
        out[m[2]][key] = out[m[2]][key].concat(list);
    }
    return out;
}

// loadAlone runs one lazy file in a page of its own and answers the page
// and the actions that the file gave.
//
// NO omn-go-api.js HERE, and that is the point. omnLazy writes a stub for
// each name, thus a page that loaded it answers "function" for a name that
// the lazy file never defines. The first draft of this test loaded it, and
// a renamed function passed.
//
// window.OMN is the one name of omn-go-core.js that each lazy file needs
// while it loads.
function loadAlone(file) {
    const page = newPage();
    const actions = {};
    page.OMN = { action(name, fn) { actions[name] = fn; } };
    const loadErr = run(page, file);
    assert.equal(loadErr, null,
        file + ' failed to load on its own: ' + (loadErr && loadErr.message));
    return { page, actions };
}

test('omn-go-api.js declares at least one lazy file', () => {
    const map = lazyMap();
    const files = Object.keys(map);
    assert.ok(files.length >= 3,
        'omn-go-api.js declares ' + files.length + ' lazy files. The parse of ' +
        'the calls is wrong, or the lazy loading went away.');
    for (const f of files) {
        assert.ok(map[f].names.length + map[f].actions.length > 0,
            f + ' is declared lazy with no name and no action');
    }
});

test('each lazy file defines every name that omn-go-api.js promises', () => {
    for (const [file, promised] of Object.entries(lazyMap())) {
        const { page } = loadAlone(file);
        for (const name of promised.names) {
            assert.equal(typeof page[name], 'function',
                file + ' does not define ' + name + ', and omnLazy promises it. ' +
                'The stub of omn-go-api.js then calls itself and the control is dead.');
        }
    }
});

test('each lazy file gives every action that omn-go-api.js promises, and no other', () => {
    for (const [file, promised] of Object.entries(lazyMap())) {
        const { actions } = loadAlone(file);
        for (const name of promised.actions) {
            assert.equal(typeof actions[name], 'function',
                file + ' gives no action ' + name + ', and omnLazyActions promises it. ' +
                'The control writes a console fault and does nothing.');
        }
        // An action that the list does not name has no stub. Its control
        // does nothing until another control loads the file.
        for (const name of Object.keys(actions)) {
            assert.ok(promised.actions.indexOf(name) >= 0,
                file + ' gives the action ' + name + ', and the omnLazyActions ' +
                'list of omn-go-api.js does not name it.');
        }
    }
});

// notLazy names a function of a lazy file whose answer a caller reads at
// once. omnLazy must not give it a stub. Before the file loads, no dialog
// is open, and omnCloseTopLayer of omn-go-core.js reads "no function" as
// "no dialog".
const notLazy = { 'omn-go-dialog.js': ['omnCloseDialog', 'omnCloseTopLayer'] };

test('no function of a lazy file is a name of window, except the promised names', () => {
    for (const [file, promised] of Object.entries(lazyMap())) {
        const before = Object.keys(newPage());
        const { page } = loadAlone(file);
        const added = Object.keys(page).filter((k) => before.indexOf(k) < 0 && k !== 'OMN');
        for (const name of added) {
            // The search file also puts its own helpers on window. This
            // test holds the files whose functions moved behind actions.
            if (file === 'omn-go-search.js') continue;
            if ((notLazy[file] || []).indexOf(name) >= 0) continue;
            assert.ok(promised.names.indexOf(name) >= 0,
                file + ' puts ' + name + ' on window, and omnLazy does not promise it. ' +
                'Give the work an action, or write a const in the guard block.');
        }
    }
});

test('no lazy file reads a name that nothing defines', async () => {
    for (const [file, promised] of Object.entries(lazyMap())) {
        // AGAIN WITHOUT omn-go-api.js. A lazy file may read a global
        // through window, which is a property of an object that exists.
        // A BARE name is a free variable, and it resolves only because
        // some other file put it in the scope by accident.
        //
        // Loading omn-go-api.js here hides exactly that. Its body sits
        // in an if block. Annex B of the standard hoists a FUNCTION of a
        // block to the global scope of sloppy mode. omn-go-sync.js read
        // applySyncLogLine that way for 17 versions. A const of the same
        // block does not hoist, and SYNC_TITLES broke the upload.
        const { page, actions } = loadAlone(file);
        // What a real page has from omn-go-core.js and omn-go-api.js, as
        // properties of window. A lazy file may use each one.
        // ONLY these names and window.OMN, and each one for a reason. A
        // function of a lazy file opens the progress overlay and subscribes
        // to the log stream before it does anything else. Without them the
        // call ends in a TypeError on the first line, and it never reaches
        // the code that this test reads. The four dialog names are the stubs
        // of omnLazy in omn-go-api.js, thus they are properties of window
        // on a real page too. Each stub answers a closed dialog here.
        //
        // NOTHING ELSE GOES HERE. A name written here resolves as a BARE
        // name as well, because window is the global object of a page.
        // The draft of this test listed applySyncLogLine, and the Annex B
        // fault then passed.
        page.OMNProgress = { show() {}, stage() {}, detail() {}, hide() {} };
        page.omnGoOnServerLog = () => () => {};
        page.omnDialog = async () => ({ value: null, input: null });
        page.omnAlert = async () => {};
        page.omnConfirm = async () => false;
        page.omnPrompt = async () => null;

        // Each promised name gets the argument 'upload'. Each action gets a
        // control whose data-arg is 'upload', which is the path of a sync
        // with the most code.
        const control = { getAttribute: () => 'upload' };
        const calls = [];
        for (const name of promised.names) {
            if (typeof page[name] === 'function') {
                calls.push([name, () => page[name]('upload')]);
            }
            // The test above reports a name that is absent.
        }
        for (const name of Object.keys(actions)) {
            calls.push(['the action ' + name, () => actions[name](control, {})]);
        }

        for (const [name, call] of calls) {
            let err = null;
            try {
                await call();
            } catch (e) {
                err = e;
            }
            // err.name and NOT `err instanceof ReferenceError`. The
            // script runs in a vm context, thus its ReferenceError is the
            // constructor of THAT realm and instanceof answers false. The
            // first draft of this test used instanceof, passed with the
            // real fault put back, and guarded nothing.
            if (err && err.name === 'ReferenceError') {
                assert.fail(file + ': ' + name + ' reads a bare name that ' +
                    'nothing defines: ' + err.message + '\n' +
                    '  A const of the if block of omn-go-api.js reaches no other ' +
                    'file, and a function of it reaches one by accident.\n' +
                    '  Move the definition into ' + file + ', or export it from ' +
                    'omn-go-api.js as a property of window.');
            }
        }
    }
});
