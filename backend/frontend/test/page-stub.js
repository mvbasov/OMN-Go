// A page, reduced to what a shipped script touches while it loads and
// while one exported function runs.
//
// WHY THIS IS NOT dom-stub.js. That module uses require, thus each script
// runs as a Node module. A Node module has its own scope, and `window` is
// an ordinary object inside it.
//
// A BROWSER is not built that way. window IS the global object, thus
// `window.runSync = ...` makes a global name. A free variable then
// resolves against the same object.
//
// That difference hides the exact fault this file exists to find. See
// lazy.test.js.
//
// This module builds a vm context whose global IS the window stub, then
// runs each script in it the way a <script src> tag does.
//
// It is not a browser. A call into one of these functions can throw a
// TypeError because an element or a method of the stub is missing. That is
// not a fault of the script. lazy.test.js reads the KIND of the error.
//
// This file is NOT under frontend/html, thus staticFS does not embed it
// and no byte of it reaches a device.

'use strict';

const fs = require('fs');
const path = require('path');
const vm = require('vm');

const scriptDir = path.join(__dirname, '..', 'html', 'js', 'OMN-Go');

function noop() {}

function makeElement() {
    const el = {
        value: '', textContent: '', innerHTML: '', innerText: '', className: '',
        style: {}, dataset: {}, hidden: false, checked: false, disabled: false,
        classList: { add: noop, remove: noop, toggle: noop, contains: () => false },
        addEventListener: noop, removeEventListener: noop,
        appendChild: noop, removeChild: noop, remove: noop,
        setAttribute: noop, removeAttribute: noop,
        getAttribute: () => null, hasAttribute: () => false,
        querySelector: () => makeElement(), querySelectorAll: () => [],
        closest: () => null, focus: noop, blur: noop, click: noop,
        insertAdjacentHTML: noop, scrollIntoView: noop,
        getBoundingClientRect: () => ({ top: 0, left: 0, width: 0, height: 0 }),
    };
    return el;
}

// newPage answers a context that behaves like a page of this application.
function newPage() {
    const doc = {
        readyState: 'complete',
        addEventListener: noop, removeEventListener: noop,
        getElementById: () => makeElement(),
        querySelector: () => makeElement(),
        querySelectorAll: () => [],
        createElement: makeElement,
        createTextNode: (t) => ({ textContent: t }),
        head: makeElement(), body: makeElement(), documentElement: makeElement(),
        cookie: '',
        execCommand: noop,
    };
    const win = {
        location: {
            protocol: 'http:', href: 'http://127.0.0.1:8080/Note.html',
            pathname: '/Note.html', search: '', hash: '',
            origin: 'http://127.0.0.1:8080', host: '127.0.0.1:8080',
            reload: noop, assign: noop, replace: noop,
        },
        document: doc, console,
        setTimeout, clearTimeout, setInterval, clearInterval,
        requestAnimationFrame: (fn) => setTimeout(fn, 0),
        navigator: { userAgent: 'node', clipboard: null, share: null },
        localStorage: { getItem: () => null, setItem: noop, removeItem: noop, clear: noop },
        sessionStorage: { getItem: () => null, setItem: noop, removeItem: noop },
        matchMedia: () => ({ matches: false, addEventListener: noop, addListener: noop }),
        alert: noop, confirm: () => false, prompt: () => null,
        addEventListener: noop, removeEventListener: noop,
        history: { replaceState: noop, pushState: noop },
        URL, URLSearchParams, Promise, JSON, Date, Math, Error, Object, Array,
        encodeURIComponent, decodeURIComponent, parseInt, parseFloat, isNaN,
        EventSource: function () { this.close = noop; this.addEventListener = noop; },
        AbortController: function () { this.abort = noop; this.signal = null; },
        // Each network call answers a small success, thus a function
        // reaches its own code and not a rejected promise.
        fetch: async () => ({
            ok: true, status: 200,
            json: async () => ({ status: 'success', files: [], unpushed: false }),
            text: async () => '',
        }),
    };
    win.window = win;
    win.self = win;
    win.globalThis = win;
    vm.createContext(win);
    return win;
}

// run evaluates one shipped script in the page, the way a <script src>
// element does. It answers the error of a load that failed, or null.
function run(page, file) {
    try {
        vm.runInContext(fs.readFileSync(path.join(scriptDir, file), 'utf8'),
            page, { filename: file });
        return null;
    } catch (e) {
        return e;
    }
}

// source answers the text of one shipped script.
function source(file) {
    return fs.readFileSync(path.join(scriptDir, file), 'utf8');
}

module.exports = { newPage, run, source, makeElement };
