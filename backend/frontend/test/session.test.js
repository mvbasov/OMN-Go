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

// loaded answers a note page after its load events. prepare gets the page
// before the scripts run, thus it can set the cookie and the server.
async function loaded(prepare) {
    const h = newDomPage();
    if (prepare) prepare(h);
    h.load(shellScripts()).ready();
    await h.settle();
    return h;
}

function boxShown(h) {
    return h.$('#omn-login-overlay').style.display === 'flex' && h.$('#omn-main-ui').style.display === 'none';
}

function pageShown(h) {
    return h.$('#omn-login-overlay').style.display === 'none' && h.$('#omn-main-ui').style.display === 'flex';
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
    h.$('#omn-pwd-input').value = 'p&ss word';
    h.press('[data-action="login"]');
    await h.settle();
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
    h.$('#omn-pwd-input').value = 'wrong';
    h.press('[data-action="login"]');
    await h.settle();
    assert.ok(boxShown(h));
    assert.strictEqual(h.dialogs[h.dialogs.length - 1].text, 'Invalid password.');
});
