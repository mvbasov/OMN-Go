// Each lazy file must define every name that omn-go-sse.js promises, and
// it must read no name that nothing defines.
//
// THE FAULT THIS FILE EXISTS TO FIND, and which shipped for 17 versions.
//
// F3 moved the sync code out of omn-go-sse.js in 26.09.24. It left the
// SYNC_TITLES map behind. The body of omn-go-sse.js sits inside an
// `if (protocol !== 'file:')` BLOCK, thus a `const` of that block reaches
// no other file. Every press of "Commit & Push" threw
// "SYNC_TITLES is not defined" and the button did nothing. 26.09.41
// repaired it.
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
// accident for those same 17 versions. omn-go-sse.js now exports it by
// hand. The test below cannot tell the two apart, and it does not need
// to: it fails on either one.
//
// WHAT A FAILURE MEANS. A ReferenceError names a variable that the lazy
// file reads and nothing defines. Move the definition into the lazy file,
// or export it from omn-go-sse.js by hand.
//
// A TypeError is NOT a failure. The page here is a stub, and a missing
// element or method of the stub is a gap in the stub.

'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { newPage, run, source } = require('./page-stub.js');

// The scripts that every note page loads, in the order of
// templates/index.html.
const PAGE_SCRIPTS = ['omn-go-core.js', 'omn-go-sse.js'];

// lazyMap reads the omnLazy calls of omn-go-sse.js and answers
// {file: [name, ...]}.
//
// It parses the REAL call and holds no copy of the list. A name added to
// omn-go-sse.js is therefore covered by this test at once, and a hand
// written copy here would drift from it.
function lazyMap() {
    const src = source('omn-go-sse.js');
    const out = {};
    const call = /omnLazy\(\s*'([^']+)'\s*,\s*\[([^\]]*)\]/g;
    let m;
    while ((m = call.exec(src)) !== null) {
        const names = m[2].split(',')
            .map((s) => s.trim().replace(/^'|'$/g, ''))
            .filter((s) => s.length > 0);
        out[m[1]] = names;
    }
    return out;
}

test('omn-go-sse.js declares at least one lazy file', () => {
    const map = lazyMap();
    const files = Object.keys(map);
    assert.ok(files.length >= 3,
        'omnLazy declares ' + files.length + ' files. The parse of the call ' +
        'in omn-go-sse.js is wrong, or the lazy loading went away.');
    for (const f of files) {
        assert.ok(map[f].length > 0, f + ' is declared lazy with no name');
    }
});

test('each lazy file defines every name that omn-go-sse.js promises', () => {
    for (const [file, names] of Object.entries(lazyMap())) {
        // NO omn-go-sse.js HERE, and that is the point. omnLazy writes a
        // stub for each name, thus a page that loaded it answers
        // "function" for a name that the lazy file never defines. The
        // first draft of this test loaded it, and a renamed function
        // passed.
        const page = newPage();
        const loadErr = run(page, file);
        assert.equal(loadErr, null,
            file + ' failed to load on its own: ' + (loadErr && loadErr.message));

        for (const name of names) {
            assert.equal(typeof page[name], 'function',
                file + ' does not define ' + name + ', and omnLazy promises it. ' +
                'The stub of omn-go-sse.js then calls itself and the control is dead.');
        }
    }
});

test('no lazy file reads a name that nothing defines', async () => {
    for (const [file, names] of Object.entries(lazyMap())) {
        // AGAIN WITHOUT omn-go-sse.js. A lazy file may read a global
        // through window, which is a property of an object that exists.
        // A BARE name is a free variable, and it resolves only because
        // some other file put it in the scope by accident.
        //
        // Loading omn-go-sse.js here hides exactly that. Its body sits
        // in an if block. Annex B of the standard hoists a FUNCTION of a
        // block to the global scope of sloppy mode. omn-go-sync.js read
        // applySyncLogLine that way for 17 versions. A const of the same
        // block does not hoist, and SYNC_TITLES broke the upload.
        const page = newPage();
        run(page, file);
        // What a real page has from omn-go-core.js and omn-go-sse.js, as
        // properties of window. A lazy file may use each one.
        // ONLY these two, and each one for a reason. A function of a
        // lazy file opens the progress overlay and subscribes to the log
        // stream before it does anything else. Without them the call ends
        // in a TypeError on the first line, and it never reaches the code
        // that this test reads.
        //
        // NOTHING ELSE GOES HERE. A name written here resolves as a BARE
        // name as well, because window is the global object of a page.
        // The draft of this test listed applySyncLogLine, and the Annex B
        // fault then passed.
        page.OMNProgress = { show() {}, stage() {}, detail() {}, hide() {} };
        page.omnGoOnServerLog = () => () => {};

        for (const name of names) {
            if (typeof page[name] !== 'function') {
                continue; // the test above reports this one
            }
            let err = null;
            try {
                await page[name]('upload');
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
                    '  A const of the if block of omn-go-sse.js reaches no other ' +
                    'file, and a function of it reaches one by accident.\n' +
                    '  Move the definition into ' + file + ', or export it from ' +
                    'omn-go-sse.js as a property of window.');
            }
        }
    }
});
