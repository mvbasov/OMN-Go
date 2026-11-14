// The login box of a note page, in the REAL JavaScript and on the markup of
// the templates.
//
// The device itself is always admin, and it never sees the box. A caller
// from the network sees the box until the login. See
// doc/decisions/0018-keep-one-role.md.
//
// checkSession decides at the load of each page. It reads the hint cookie
// first, because a cookie costs no request. With no hint it asks the
// server, and only the answer 401 shows the box.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { newDomPage, shellScripts } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

// loaded answers a note page after its load events. prepare gets the page
// before the scripts run, thus it can set the cookie and the server. The
// test types into the login dialog itself, thus the page keeps it open.
async function loaded(prepare) {
    const h = newDomPage();
    h.keepDialogs = true;
    if (prepare) prepare(h);
    h.load(shellScripts()).ready();
    await h.settle();
    return h;
}

// loginDialog answers the open login dialog, or null.
function loginDialog(h) {
    return h.document.querySelector('.omn-dialog[data-omn-dialog="login"]');
}

function boxShown(h) {
    return loginDialog(h) !== null && h.$('#omn-main-ui').style.display === 'none';
}

function pageShown(h) {
    return loginDialog(h) === null && h.$('#omn-main-ui').style.display === 'flex';
}

// login types a password into the dialog and presses Enter.
async function login(h, password) {
    const dialog = loginDialog(h);
    dialog.querySelector('input').value = password;
    dialog.querySelector('[data-omn-default]').click();
    await h.settle();
}

test('the hint cookie shows the page with no request', async () => {
    const h = await loaded(function (h) {
        h.document.cookie = 'theme=dark; session_role_hint=admin';
    });
    assert.ok(pageShown(h));
    assert.deepStrictEqual(h.requests.filter(function (r) { return r.url === '/api/config'; }), []);
});

test('with no hint the page asks the server, and 401 shows the login box', async () => {
    const refused = await loaded(function (h) {
        h.server = function () { return { httpStatus: 401, text: 'no' }; };
    });
    assert.ok(boxShown(refused), 'a caller with no session reads the page');

    // Each other answer shows the page. The device itself gets 200 here.
    const local = await loaded();
    assert.ok(pageShown(local));
    assert.strictEqual(local.requests.filter(function (r) { return r.url === '/api/config'; }).length, 1);
});

test('a hint of another value is not a login', async () => {
    const h = await loaded(function (h) {
        h.document.cookie = 'session_role_hint=guest';
        h.server = function () { return { httpStatus: 401, text: 'no' }; };
    });
    assert.ok(boxShown(h));
});

test('a correct password closes the box, and the page sends it one time', async () => {
    const h = await loaded(function (h) {
        h.server = function (request) {
            if (request.url === '/login') return { httpStatus: 200, text: 'ok' };
            return { httpStatus: 401, text: 'no' };
        };
    });
    assert.ok(boxShown(h));
    const field = loginDialog(h).querySelector('input');
    assert.strictEqual(field.type, 'password');
    await login(h, 'p&ss word');
    const logins = h.requests.filter(function (r) { return r.url === '/login'; });
    assert.strictEqual(logins.length, 1);
    assert.strictEqual(logins[0].method, 'POST');
    // The password is in the body, with the escape of a form.
    assert.strictEqual(logins[0].body, 'password=p%26ss%20word');
    assert.ok(pageShown(h));
});

test('a wrong password keeps the box and tells the person', async () => {
    const h = await loaded(function (h) {
        h.server = function () { return { httpStatus: 401, text: 'no' }; };
    });
    await login(h, 'wrong');
    assert.ok(boxShown(h));
    // The error is a line of the login dialog. A second dialog would wait
    // behind the login dialog and never show.
    assert.strictEqual(loginDialog(h).querySelector('.omn-dialog-error').textContent, 'Invalid password.');
    assert.strictEqual(h.dialogs.length, 1);
});

test('Escape and the Back key do not close the login dialog', async () => {
    const h = await loaded(function (h) {
        h.server = function () { return { httpStatus: 401, text: 'no' }; };
    });
    const dialog = loginDialog(h);
    dialog.querySelector('input').dispatchEvent(new Event('keydown', { bubbles: true, cancelable: true, key: 'Escape' }));
    assert.strictEqual(h.page.omnCloseTopLayer(), true, 'the WebView would go back');
    await h.settle();
    assert.ok(boxShown(h));
});

test('a page from disk has no login gate and asks the server nothing', async () => {
    // A copy from disk still holds the empty slot of the modals.
    const h = newDomPage({ body: '<div id="omn-main-ui"></div><div id="omn-go-modals-slot"></div>' });
    h.server = function () { return { httpStatus: 401, text: 'no' }; };
    h.load(shellScripts()).ready();
    await h.settle();
    assert.strictEqual(loginDialog(h), null);
    assert.deepStrictEqual(h.requests.filter(function (r) { return r.url === '/api/config'; }), []);
});
