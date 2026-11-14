// A page with a real document, for the tests that press a control.
//
// page-stub.js builds the window. This file gives that window the document
// of mini-dom.js, the markup of the real templates, and a server that the
// test writes. A test then loads the shipped scripts, presses a control and
// reads the page and the requests.
//
// THE MARKUP IS THE MARKUP OF THE TEMPLATES. shellBody reads index.html and
// modals.html, the same files that the server sends. A control that a
// template loses is thus absent in the test too, and the test fails where
// the page would.
//
// This file is NOT under frontend/html, thus frontend.Static does not embed
// it and no byte of it reaches a device.

'use strict';

const fs = require('fs');
const path = require('path');
const assert = require('node:assert');
const { newPage, run } = require('./page-stub.js');
const { newDocument, Event, EventTarget, NodeFilter, FormData } = require('./mini-dom.js');

const templateDir = path.join(__dirname, '..', 'templates');

// template answers the text of one template. values gives the text of a
// placeholder by its name, and each other placeholder is empty.
function template(name, values) {
    values = values || {};
    return fs.readFileSync(path.join(templateDir, name), 'utf8')
        .replace(/%%([A-Z0-9_]+)%%/g, function (all, key) {
            return Object.prototype.hasOwnProperty.call(values, key) ? values[key] : '';
        });
}

// shellBody answers the body of a note page: the body of index.html and
// the modals that the server adds when it serves the page. values fills
// the placeholders of index.html, for example PREVIEW_BODY.
function shellBody(values) {
    const index = template('index.html', values);
    const open = index.indexOf('<body>');
    const close = index.indexOf('</body>');
    assert.ok(open >= 0 && close > open, 'index.html has no body element');
    return index.slice(open + '<body>'.length, close) + template('modals.html');
}

// shellScripts answers the application scripts that index.html names, in
// its order. omn-go-compat.js is not in the list, because it is the notice
// of an old browser and gives the page nothing. The vendored libraries and
// the user file omn-go-custom.js are not in the list either.
function shellScripts() {
    const src = fs.readFileSync(path.join(templateDir, 'index.html'), 'utf8');
    const out = [];
    const re = /js\/OMN-Go\/(omn-go-[a-z]+\.js)/g;
    let m;
    while ((m = re.exec(src)) !== null) {
        if (m[1] !== 'omn-go-compat.js') out.push(m[1]);
    }
    return out;
}

// newDomPage answers a page that is ready for a test.
//
// opts.body is the markup of the body. The default is shellBody().
// opts.preview is the markup of the note, inside #omn-preview.
// opts.protocol is "http:" or "file:". The default is "http:".
// opts.note is the name of the note, with no extension. The default is
// "Note". The page gets the variables that the script block of index.html
// sets: PageName, currentNote, Title, PAGE_EXT and IS_MARKDOWN.
// opts.markdown is false for a page that is a file and not a note.
// opts.path is the path of the page. The default comes from opts.note.
// opts.meta gives the meta elements of the head, as {name: content}. The
// server writes one for each line of the header block of a note.
//
// The answer holds:
//
//	page       The window, which is also the global object of each script.
//	document   The document of mini-dom.js.
//	requests   Each call of fetch: {url, method, body}.
//	dialogs    Each alert, confirm and prompt: {kind, text}.
//	loads      The src of each script element that a script added.
//	styles     The href of each stylesheet link that a script added.
//	streams    Each EventSource that a script opened.
//	went       Each address that a script opened in place of this page.
//	replaced   Each address of went that took this page out of the history.
//	opened     Each call of window.open: {url, target}.
//	clipboard  Each text that a script wrote with the Clipboard API.
//	timers     The timers of the page, after h.holdTimers().
//
// A test sets h.server to answer a request, and h.confirm, h.prompt,
// h.choose and h.fill to answer a dialog.
function newDomPage(opts) {
    opts = opts || {};
    const page = newPage();
    const doc = newDocument(opts.body === undefined
        ? shellBody({ PREVIEW_BODY: opts.preview || '' }) : opts.body);
    const win = new EventTarget();
    doc.defaultView = win;
    doc.readyState = 'loading';
    for (const name of Object.keys(opts.meta || {})) {
        const meta = doc.createElement('meta');
        meta.setAttribute('name', name);
        meta.setAttribute('content', opts.meta[name]);
        doc.head.appendChild(meta);
    }

    page.document = doc;
    page.NodeFilter = NodeFilter;
    page.Event = Event;
    page.addEventListener = win.addEventListener.bind(win);
    page.removeEventListener = win.removeEventListener.bind(win);
    page.dispatchEvent = win.dispatchEvent.bind(win);
    page.location.protocol = opts.protocol || 'http:';
    const note = opts.note || 'Note';
    page.PageName = note;
    page.currentNote = note;
    page.Title = opts.title || note;
    page.PAGE_EXT = '.md';
    page.IS_MARKDOWN = opts.markdown !== false;
    page.location.pathname = opts.path || '/' + note + '.html';
    page.location.reloads = 0;
    page.location.reload = function () { page.location.reloads++; };
    // A script that sets location.href leaves the page. The document of a
    // test stays, thus href keeps the address of this page, and h.went
    // gets each address that a script tried to open.
    Object.defineProperty(page.location, 'href', {
        get() {
            return page.location.origin + page.location.pathname +
                page.location.search + page.location.hash;
        },
        set(value) { h.went.push(String(value)); },
    });
    // replace also takes this page out of the history, thus the Back key
    // does not open it again. h.replaced gets each such address.
    page.location.replace = function (url) {
        h.went.push(String(url));
        h.replaced.push(String(url));
    };
    page.location.assign = function (url) { h.went.push(String(url)); };
    // A new hash sends hashchange, the same as in a browser.
    let hash = '';
    Object.defineProperty(page.location, 'hash', {
        get() { return hash; },
        set(value) {
            const next = value && value[0] !== '#' ? '#' + value : (value || '');
            if (next === hash) return;
            hash = next;
            win.dispatchEvent(new Event('hashchange'));
        },
    });
    page.FormData = FormData;
    page.scrollTo = function () {};
    // window.open keeps each address, thus a test can read h.opened.
    page.open = function (url, target) { h.opened.push({ url: String(url), target: target }); };
    // The clipboard of the browser. A test reads h.clipboard, and it can
    // set h.clipboardRefuses to model a browser that refuses the write.
    page.isSecureContext = true;
    page.navigator.clipboard = {
        writeText: async function (text) {
            if (h.clipboardRefuses) throw new Error('NotAllowedError');
            h.clipboard.push(String(text));
        },
    };
    page.history.backs = 0;
    page.history.back = function () { page.history.backs++; };

    // A console of its own. omn-go-console.js puts a hook on each console
    // method, and the console of Node is one object for all the tests.
    page.console = { log() {}, info() {}, warn() {}, error() {}, debug() {}, trace() {}, table() {}, dir() {}, time() {}, timeEnd() {} };

    const h = {
        page: page, document: doc, requests: [], dialogs: [], loads: [], styles: [], streams: [],
        opened: [], clipboard: [], timers: [], went: [], replaced: [],
        // server answers one request. A test replaces it. The default is
        // a JSON success with no data.
        server: function () { return { status: 'success' }; },
        confirm: function () { return false; },
        prompt: function () { return null; },
        choose: function () { return null; },
        fill: function () { return null; },
    };

    // fetch gives the request to h.server. The answer of h.server is the
    // JSON body, or {httpStatus, json, text} for another status, or an
    // Error for a request that the network refused.
    page.fetch = async function (url, init) {
        init = init || {};
        // The body is text. URLSearchParams and FormData both give the
        // form of a query.
        const request = {
            url: String(url),
            method: init.method || 'GET',
            body: init.body === undefined ? null : String(init.body),
        };
        h.requests.push(request);
        const answer = await h.server(request);
        if (answer instanceof Error) throw answer;
        const full = answer && answer.httpStatus !== undefined ? answer : { httpStatus: 200, json: answer };
        return {
            ok: full.httpStatus >= 200 && full.httpStatus < 300,
            status: full.httpStatus,
            json: async function () {
                if (full.json === undefined) throw new Error('the answer is not JSON');
                return full.json;
            },
            text: async function () {
                return full.text !== undefined ? full.text : JSON.stringify(full.json);
            },
        };
    };
    // The log stream of the server. A test sends a line with
    // h.serverLog(text).
    page.EventSource = function (url) {
        this.url = url;
        this.close = function () {};
        this.addEventListener = function () {};
        h.streams.push(this);
    };
    h.serverLog = function (text) {
        for (const s of h.streams) {
            if (typeof s.onmessage === 'function') s.onmessage({ data: text });
        }
    };
    page.alert = function (text) { h.dialogs.push({ kind: 'alert', text: String(text) }); };
    page.confirm = function (text) {
        h.dialogs.push({ kind: 'confirm', text: String(text) });
        return h.confirm(String(text));
    };
    page.prompt = function (text, value) {
        h.dialogs.push({ kind: 'prompt', text: String(text) });
        return h.prompt(String(text), value);
    };

    // A dialog of omn-go-dialog.js is an element .omn-dialog in the body.
    // The page records it in h.dialogs, the same as a dialog of the browser.
    // After one timer turn, the page presses the button of the answer:
    //
    //	alert    OK.
    //	confirm  OK when h.confirm answers true, and else Cancel.
    //	prompt   OK with the text of h.prompt, or Cancel for null.
    //	fields   Each other dialog with fields, for example "form" or
    //	         "commit". The texts of h.fill go into the fields, one after
    //	         the other, each with an input event. Then OK, or Cancel for
    //	         null. A field can be an input or a textarea.
    //	choice   Each other dialog. The button with the label of h.choose,
    //	         or else Cancel.
    //
    // h.keepDialogs = true leaves each dialog open for the test.
    const appendToBody = doc.body.appendChild.bind(doc.body);
    doc.body.appendChild = function (el) {
        const out = appendToBody(el);
        if (el.classList && el.classList.contains('omn-dialog')) {
            const kind = el.getAttribute('data-omn-dialog');
            const text = el.querySelector('.omn-dialog-text').textContent;
            h.dialogs.push({ kind: kind, text: text });
            if (!h.keepDialogs) setTimeout(function () { answerDialog(el, kind, text); }, 0);
        }
        return out;
    };
    function answerDialog(el, kind, text) {
        if (!el.parentNode) return;
        const ok = el.querySelector('[data-omn-default]');
        const cancel = el.querySelector('[data-omn-cancel]') || ok;
        if (kind === 'alert') {
            ok.click();
        } else if (kind === 'confirm') {
            (h.confirm(text) ? ok : cancel).click();
        } else if (kind === 'prompt') {
            const input = el.querySelector('.omn-dialog-input');
            const value = h.prompt(text, input.value);
            if (value === null || value === undefined) {
                cancel.click();
            } else {
                input.value = String(value);
                ok.click();
            }
        } else if (el.querySelector('.omn-dialog-input')) {
            // A dialog with fields, for example the New page form or the
            // commit dialog.
            const inputs = el.querySelectorAll('.omn-dialog-input');
            const values = h.fill(text, inputs.map(function (i) { return i.value; }));
            if (!values) {
                cancel.click();
                return;
            }
            values.forEach(function (value, index) {
                if (value === undefined || value === null) return;
                inputs[index].value = String(value);
                inputs[index].dispatchEvent(new Event('input', { bubbles: true }));
            });
            ok.click();
        } else {
            const label = h.choose(text);
            // A button with a hint holds two lines. The label is the first.
            const chosen = el.querySelectorAll('button').find(function (b) {
                const first = b.querySelector('.omn-dialog-button-label');
                return (first ? first.textContent : b.textContent) === label;
            });
            (chosen || cancel).click();
        }
    }

    // A script element that a script adds to the head is a lazy file. The
    // page runs that file and then calls onload, the same as a browser. A
    // stylesheet link goes to h.styles, and it calls onload too.
    const appendToHead = doc.head.appendChild.bind(doc.head);
    const insertIntoHead = doc.head.insertBefore.bind(doc.head);
    function loadStyle(el) {
        if (el.tagName === 'LINK' && el.getAttribute('rel') === 'stylesheet') {
            h.styles.push(el.getAttribute('href'));
            setTimeout(function () {
                if (typeof el.onload === 'function') el.onload();
            }, 0);
        }
    }
    doc.head.insertBefore = function (el, before) {
        insertIntoHead(el, before);
        loadStyle(el);
        return el;
    };
    // appendChild of mini-dom calls insertBefore, thus loadStyle runs there.
    doc.head.appendChild = function (el) {
        appendToHead(el);
        if (el.tagName === 'SCRIPT' && el.getAttribute('src')) {
            const src = el.getAttribute('src');
            h.loads.push(src);
            setTimeout(function () {
                const err = run(page, path.basename(src));
                if (err) {
                    if (typeof el.onerror === 'function') el.onerror(err);
                    throw err;
                }
                if (typeof el.onload === 'function') el.onload();
            }, 0);
        }
        return el;
    };

    // load runs each script in order. A script that throws fails the test.
    h.load = function (files) {
        for (const file of files) {
            const err = run(page, file);
            assert.strictEqual(err, null, file + ' did not load: ' + (err && err.stack));
        }
        return h;
    };
    // ready ends the load of the page: DOMContentLoaded on the document,
    // and then load on the window.
    h.ready = function () {
        doc.readyState = 'interactive';
        doc.dispatchEvent(new Event('DOMContentLoaded', { bubbles: true }));
        doc.readyState = 'complete';
        win.dispatchEvent(new Event('load'));
        return h;
    };
    // $ answers the one element of a selector. An absent element fails
    // the test with the selector in the message.
    h.$ = function (selector) {
        const el = doc.querySelector(selector);
        assert.ok(el, 'the page has no element for ' + selector);
        return el;
    };
    // type puts a value into a box and sends the input event, the same as
    // a key press does.
    h.type = function (selector, value) {
        const el = h.$(selector);
        el.value = value;
        el.dispatchEvent(new Event('input', { bubbles: true }));
        return h;
    };
    // key sends a keydown to the one element of a selector and answers the
    // event. init can hold ctrlKey and the other fields of a key event.
    h.key = function (selector, name, init) {
        const event = new Event('keydown', Object.assign({ bubbles: true, key: name }, init || {}));
        h.$(selector).dispatchEvent(event);
        return event;
    };
    // press sends a click to the one element of a selector.
    h.press = function (selector) {
        h.$(selector).click();
        return h;
    };
    // settle waits until each promise and each short timer of the page
    // ended. rounds is the count of timer turns, and the default is 5.
    h.settle = async function (rounds) {
        for (let i = 0; i < (rounds || 5); i++) {
            await new Promise(function (resolve) { setTimeout(resolve, 0); });
        }
    };
    // holdTimers takes the timers of the page away from the clock. Each
    // setTimeout of a script goes to h.timers as {fn, ms, cleared}, and
    // none runs until the test calls runTimers. A test of a wait of some
    // seconds then ends at once.
    h.holdTimers = function () {
        page.setTimeout = function (fn, ms) {
            const timer = { fn: fn, ms: ms || 0, cleared: false, ran: false };
            h.timers.push(timer);
            return timer;
        };
        page.clearTimeout = function (timer) {
            if (timer && typeof timer === 'object') timer.cleared = true;
        };
        return h;
    };
    // pendingTimers answers the wait of each timer that can still run.
    h.pendingTimers = function () {
        return h.timers.filter(function (t) { return !t.cleared && !t.ran; })
            .map(function (t) { return t.ms; });
    };
    // runTimers runs each timer that can still run.
    h.runTimers = function () {
        for (const t of h.timers.slice()) {
            if (t.cleared || t.ran) continue;
            t.ran = true;
            t.fn();
        }
        return h;
    };
    // hidden tells whether the class "hidden" or the style hides an
    // element.
    h.hidden = function (selector) {
        const el = h.$(selector);
        return el.classList.contains('omn-hidden') || el.style.display === 'none';
    };
    return h;
}

// notePage answers a page with the scripts of index.html, after the load
// events. It is what a person has in front of them when they press a
// control of a note page.
function notePage(opts) {
    return newDomPage(opts).load(shellScripts()).ready();
}

// configPage answers the Config page after the load events. The page has
// two git server cards, with the slot numbers 0 and 1. values fills the
// placeholders of config_page.html.
function configPage(values) {
    const cards = [0, 1].map(function (i) {
        return template('git_server_card.html', { INDEX: String(i), SLOT: String(i + 1) });
    }).join('');
    const body = shellBody() + template('config_page.html', Object.assign({ GIT_SERVERS: cards }, values || {}));
    return newDomPage({ body: body, path: '/Config.html' })
        .load(shellScripts().concat(['omn-go-config.js'])).ready();
}

// systemPage answers a system page after the load events: the shell of
// index.html with one template as its body, and the script of that page.
// The script of such a page asks the server while it loads, thus the
// server function is an argument. See the Status page and the Log page.
function systemPage(templateName, script, server, opts) {
    const h = newDomPage(Object.assign({ preview: template(templateName), markdown: false }, opts || {}));
    if (server) h.server = server;
    return h.load(shellScripts().concat([script])).ready();
}

// editorPage answers the editor page with text as the note, after the load
// events and the load of the note. The editor is a page of its own. It has
// the markup of editor.html and two scripts: omn-go-dialog.js and
// omn-go-editor.js. It has no other script of the application.
//
// opts.search is the query of the address, for example "?line=3".
// opts.storage gives the values that localStorage holds before the load.
// opts.server answers a request, the same as h.server. The default gives
// text for the note and a success for each other request.
// opts.noInsertText is true for a browser with no insertText command.
//
// h.storage is the localStorage of the page, as a plain object.
async function editorPage(text, opts) {
    opts = opts || {};
    const html = template('editor.html');
    const open = html.indexOf('<body');
    const start = html.indexOf('>', open) + 1;
    const close = html.indexOf('</body>');
    assert.ok(open >= 0 && close > start, 'editor.html has no body element');
    const h = newDomPage({ body: html.slice(start, close), path: '/Note.md' });
    h.page.OMN_EDIT_NAME = 'Note';
    h.page.OMN_EDIT_EXT = '.md';
    h.page.OMN_EDIT_VIEW = '/Note.html';
    h.page.location.search = opts.search || '';
    h.page.getComputedStyle = function () { return { lineHeight: '18px', fontSize: '14px' }; };

    h.storage = Object.assign({}, opts.storage || {});
    h.page.localStorage = {
        getItem: function (key) {
            return Object.prototype.hasOwnProperty.call(h.storage, key) ? h.storage[key] : null;
        },
        setItem: function (key, value) { h.storage[key] = String(value); },
        removeItem: function (key) { delete h.storage[key]; },
    };

    // The insertText command of a browser. It puts the text in place of
    // the selection of the control that has the focus. It then puts the
    // caret behind the text. The editor uses it to keep the undo history.
    h.document.execCommand = function (name, _, value) {
        h.document.commands.push(name);
        const el = h.document.activeElement;
        if (opts.noInsertText || name !== 'insertText' || !el || el.selectionStart === undefined) {
            return false;
        }
        const from = el.selectionStart, to = el.selectionEnd;
        el.value = el.value.slice(0, from) + value + el.value.slice(to);
        el.setSelectionRange(from + value.length, from + value.length);
        return true;
    };

    h.server = opts.server || function (request) {
        if (request.url.indexOf('/api/note') === 0 && request.method === 'GET') {
            return { httpStatus: 200, text: text };
        }
        return { status: 'success' };
    };
    h.load(['omn-go-dialog.js', 'omn-go-editor.js']).ready();
    await h.settle();
    return h;
}

// bookmarkPage answers the bookmark page after the load of its script.
// The page is a note page: the scripts of index.html load first, and the
// note then loads Bookmarker.js, the same as md/Bookmarks.md does.
//
// bookmarks is the list that the script block of the note sets. The page
// gets a copy, because the script sorts the list in place.
//
// opts.search is the query of the address, and opts.hash is its fragment.
// opts.storage gives the values that localStorage holds before the load.
// opts.preview is the markup that the note has before the script runs.
//
// h.storage is the localStorage of the page, as a plain object. The
// script reads and writes it with an index, and a plain object allows
// that.
function bookmarkPage(bookmarks, opts) {
    opts = opts || {};
    const h = newDomPage({ note: 'Bookmarks', preview: opts.preview || '' });
    h.page.bookmarks = JSON.parse(JSON.stringify(bookmarks));
    h.page.location.search = opts.search || '';
    if (opts.hash) h.page.location.hash = opts.hash;
    h.storage = Object.assign({}, opts.storage || {});
    h.page.localStorage = h.storage;
    // Bookmarker.css hides the tag cloud until a style shows it.
    h.page.getComputedStyle = function (el) {
        return { display: el.style.display || (el.id === 'omn-bm-tags-cloud' ? 'none' : 'block') };
    };
    return h.load(shellScripts().concat(['Bookmarker.js'])).ready();
}

module.exports = {
    newDomPage, notePage, configPage, systemPage, editorPage, bookmarkPage,
    shellBody, shellScripts, template,
};
