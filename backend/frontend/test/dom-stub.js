// A browser, reduced to what a script of this project touches while it
// loads.
//
// Each file under frontend/html/js runs in a page. Node has no window and
// no document, thus a plain require of one of those files throws on its
// first line. This module makes the globals that a LOAD needs, and
// nothing more. It is not a browser, and no test here drives a page.
//
// readyState is "loading" ON PURPOSE. omn-go-editor.js calls init() at
// once when the document is ready, and init reads a textarea that no test
// has. With "loading" the file waits for a DOMContentLoaded event that
// never arrives, thus the module loads and defines its functions and
// starts nothing.
//
// This file is NOT under frontend/html, thus staticFS does not embed it
// and no byte of it reaches a device.

'use strict';

function noop() {}

function makeElement() {
    const el = {
        value: '', textContent: '', innerHTML: '', className: '',
        style: {}, dataset: {}, hidden: false, checked: false,
        classList: { add: noop, remove: noop, toggle: noop, contains: () => false },
        addEventListener: noop, removeEventListener: noop,
        appendChild: noop, removeChild: noop, setAttribute: noop,
        getAttribute: () => null, hasAttribute: () => false,
        querySelector: () => null, querySelectorAll: () => [],
        closest: () => null, focus: noop, blur: noop, click: noop,
        getBoundingClientRect: () => ({ top: 0, left: 0, width: 0, height: 0 }),
    };
    return el;
}

function install() {
    const doc = {
        readyState: 'loading',
        addEventListener: noop, removeEventListener: noop,
        getElementById: () => null,
        querySelector: () => null,
        querySelectorAll: () => [],
        createElement: makeElement,
        createTextNode: (t) => ({ textContent: t }),
        head: makeElement(),
        body: makeElement(),
        documentElement: makeElement(),
        cookie: '',
    };
    const win = {
        location: { protocol: 'http:', href: 'http://localhost/Note.html',
                    pathname: '/Note.html', search: '', hash: '', origin: 'http://localhost' },
        document: doc,
        addEventListener: noop, removeEventListener: noop,
        setTimeout: setTimeout, clearTimeout: clearTimeout,
        requestAnimationFrame: (fn) => setTimeout(fn, 0),
        navigator: { userAgent: 'node', clipboard: null },
        localStorage: {
            getItem: () => null, setItem: noop, removeItem: noop, clear: noop,
        },
        matchMedia: () => ({ matches: false, addEventListener: noop }),
        alert: noop, confirm: () => false, prompt: () => null,
    };
    win.window = win;
    // defineProperty and not a plain assignment. Node 22 declares a
    // global navigator with a getter and no setter, thus an assignment
    // throws. A shipped script reads navigator.userAgent.
    for (const [name, value] of [
        ['window', win], ['document', doc],
        ['navigator', win.navigator], ['localStorage', win.localStorage],
    ]) {
        Object.defineProperty(global, name, {
            value: value, writable: true, configurable: true, enumerable: false,
        });
    }
    return win;
}

// load requires one shipped script with the stub in place and answers
// what its export tail exposed.
//
// Each script keeps its export tail behind a check of module, thus a
// browser never sees it. See the tail of omn-go-editor.js.
function load(relPath) {
    install();
    const path = require('path');
    const full = path.join(__dirname, '..', 'html', 'js', 'OMN-Go', relPath);
    delete require.cache[require.resolve(full)];
    const exported = require(full);
    if (!exported || Object.keys(exported).length === 0) {
        throw new Error(relPath + ' exported nothing. Check its export tail.');
    }
    return exported;
}

module.exports = { install, load, makeElement };
