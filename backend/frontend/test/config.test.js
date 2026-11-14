// The Config page, in the REAL JavaScript and on the markup of the
// templates.
//
// THE RULE THAT MATTERS MOST. The page shows each password box and each SSH
// key box empty, thus the HTML holds no secret. An empty box must then mean
// "keep the stored value". A save that sends an empty admin_password would
// clear the password of the device at each save of any other setting.
//
// The page sends a secret box only when the person typed in it. These
// tests press Save and read the body that the server gets.
//
// The server half is TestConfigPostKeepsAnUnsentGitSecret and its
// neighbors in backend/internal/app/config_secrets_test.go.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { configPage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const SAVE = '[data-screen="general"] [data-action="config-save"]';
const REVEAL = '[data-screen="network"] [data-action="config-reveal"]';

// saved answers the fields of the last POST to /api/config.
function saved(h) {
    const posts = h.requests.filter(function (r) { return r.url === '/api/config' && r.method === 'POST'; });
    assert.ok(posts.length > 0, 'no save reached the server');
    return new URLSearchParams(posts[posts.length - 1].body);
}

// stored is the configuration that GET /api/config answers in these tests.
const stored = {
    admin_password: 'hunter2',
    git_servers: [
        { ssh_key_data: 'KEY-ZERO', password: 'pass-zero' },
        { ssh_key_data: '', password: 'pass-one' },
    ],
};

function configServer(request) {
    if (request.url === '/api/config' && request.method === 'GET') return stored;
    if (request.url === '/api/config') return { httpStatus: 200, text: 'OK' };
    return { status: 'success' };
}

test('the menu shows first, and a menu line opens its screen', () => {
    const h = configPage();
    assert.strictEqual(h.$('#omn-config-menu').classList.contains('omn-active'), true);
    assert.strictEqual(h.document.querySelectorAll('.omn-config-screen.omn-active').length, 1);

    h.press('[data-goto="network"]');
    assert.strictEqual(h.page.location.hash, '#cfg-network');
    const active = h.document.querySelectorAll('.omn-config-screen.omn-active');
    assert.deepStrictEqual(active.map(function (s) { return s.getAttribute('data-screen'); }), ['network']);
    assert.strictEqual(h.$('#omn-config-menu').classList.contains('omn-active'), false);
});

test('each menu line has a screen', () => {
    const h = configPage();
    const lines = h.document.querySelectorAll('[data-goto]');
    assert.ok(lines.length >= 7, 'the scan found ' + lines.length + ' menu lines');
    for (const line of lines) {
        const name = line.getAttribute('data-goto');
        line.click();
        const active = h.document.querySelectorAll('.omn-config-screen.omn-active');
        assert.deepStrictEqual(active.map(function (s) { return s.getAttribute('data-screen'); }), [name],
            'the menu line ' + name + ' opens no screen of that name');
    }
});

test('the Back control uses the history, and an unknown hash shows the menu', () => {
    const h = configPage();
    h.press('[data-goto="git"]');
    h.press('[data-screen="git"] [data-back]');
    // The browser changes the hash on Back. The page must not do it
    // itself, or the Android Back button would need two presses.
    assert.strictEqual(h.page.history.backs, 1);
    assert.strictEqual(h.page.location.hash, '#cfg-git');

    h.page.location.hash = '';
    assert.strictEqual(h.$('#omn-config-menu').classList.contains('omn-active'), true);
    // A hash of a note heading is not a screen.
    h.page.location.hash = '#some-heading';
    assert.strictEqual(h.$('#omn-config-menu').classList.contains('omn-active'), true);
    h.page.location.hash = '#cfg-no-such-screen';
    assert.strictEqual(h.$('#omn-config-menu').classList.contains('omn-active'), true);
});

test('a change marks the page, and the browser then asks before it leaves', () => {
    const h = configPage();
    const leave = function () {
        const event = new Event('beforeunload');
        h.page.dispatchEvent(event);
        return event.defaultPrevented;
    };
    assert.strictEqual(leave(), false, 'a page with no change stops the person');
    assert.strictEqual(h.$('#omn-config-menu-dirty').hidden, true);

    h.type('#omn-cfg-author', 'A new name');
    assert.strictEqual(h.$('#omn-config-menu-dirty').hidden, false);
    const dots = h.document.querySelectorAll('.omn-config-dirty-indicator .omn-config-dirty-dot');
    assert.ok(dots.length >= 7);
    assert.ok(dots.every(function (d) { return d.classList.contains('omn-dirty'); }));
    assert.strictEqual(h.$('[data-screen="git"] .omn-config-dirty-indicator .omn-config-dirty-label').textContent,
        'Unsaved changes');
    assert.strictEqual(leave(), true, 'a changed page lets the person leave with no question');
});

test('a checkbox change marks the page too', () => {
    const h = configPage();
    h.$('#omn-cfg-internal-ed').dispatchEvent(new Event('change', { bubbles: true }));
    assert.strictEqual(h.$('#omn-config-menu-dirty').hidden, false);
});

test('Save sends each setting and no secret that the person did not type', async () => {
    const h = configPage({ AUTHOR: 'Me', SERVER_PORT: '8080', USE_INTERNAL_EDITOR_CHECKED: 'checked' });
    h.server = configServer;
    h.type('#omn-cfg-author', 'Another name');
    h.press(SAVE);
    await h.settle();

    const fields = saved(h);
    assert.strictEqual(fields.get('author'), 'Another name');
    assert.strictEqual(fields.get('server_port'), '8080');
    assert.strictEqual(fields.get('use_internal_editor'), 'true');
    for (const secret of ['admin_password', 'git_key_0', 'git_pass_0', 'git_key_1', 'git_pass_1']) {
        assert.strictEqual(fields.has(secret), false,
            'the save sent an empty ' + secret + ', and the server would clear it');
    }
    // An unchecked box sends nothing, which is how a form says "off".
    assert.strictEqual(fields.has('share_lan'), false);

    // After the save the page is clean and loads again.
    assert.strictEqual(h.$('#omn-config-menu-dirty').hidden, true);
    assert.strictEqual(h.page.location.reloads, 1);
    // The result is a notice for the next page, and not a dialog.
    assert.strictEqual(h.session['omn-notice'], 'Configuration saved.');
});

test('Save sends a secret that the person typed, also an empty one', async () => {
    const h = configPage();
    h.server = configServer;
    h.type('#omn-cfg-admin-pwd', 'a new password');
    // The person empties this box by hand. That means "clear the stored
    // value", and the server must get the empty field.
    h.type('#omn-git-pass-1', '');
    h.press(SAVE);
    await h.settle();

    const fields = saved(h);
    assert.strictEqual(fields.get('admin_password'), 'a new password');
    assert.strictEqual(fields.get('git_pass_1'), '');
    assert.strictEqual(fields.has('git_pass_0'), false);
    assert.strictEqual(fields.has('git_key_0'), false);
});

test('the reveal button fills the boxes, and a reveal alone sends no secret', async () => {
    const h = configPage();
    h.server = configServer;
    h.press(REVEAL);
    await h.settle();

    assert.strictEqual(h.$('#omn-cfg-admin-pwd').value, 'hunter2');
    assert.strictEqual(h.$('#omn-cfg-admin-pwd').type, 'text', 'the password stays behind dots');
    assert.strictEqual(h.$('#omn-git-key-0').value, 'KEY-ZERO');
    assert.strictEqual(h.$('#omn-git-pass-0').value, 'pass-zero');
    assert.strictEqual(h.$('#omn-git-key-1').value, '');
    assert.strictEqual(h.$('#omn-git-pass-1').value, 'pass-one');
    assert.strictEqual(h.$(REVEAL + ' [data-reveal-label]').textContent, 'Hide');
    // A reveal is not a change.
    assert.strictEqual(h.$('#omn-config-menu-dirty').hidden, true);

    h.press(SAVE);
    await h.settle();
    const fields = saved(h);
    for (const secret of ['admin_password', 'git_key_0', 'git_pass_0', 'git_pass_1']) {
        assert.strictEqual(fields.has(secret), false, 'a reveal alone sent ' + secret);
    }
});

test('the second press hides the secrets again, and keeps what the person typed', async () => {
    const h = configPage();
    h.server = configServer;
    h.press(REVEAL);
    await h.settle();
    h.type('#omn-git-pass-0', 'typed by hand');
    h.press(REVEAL);
    await h.settle();

    assert.strictEqual(h.$('#omn-cfg-admin-pwd').value, '');
    assert.strictEqual(h.$('#omn-cfg-admin-pwd').type, 'password');
    assert.strictEqual(h.$('#omn-git-pass-0').value, 'typed by hand', 'the hide removed the work of the person');
    assert.strictEqual(h.$(REVEAL + ' [data-reveal-label]').textContent, 'Show passwords');
    // One request for the two presses: the hide asks the server nothing.
    const reads = h.requests.filter(function (r) { return r.url === '/api/config' && r.method === 'GET'; });
    assert.strictEqual(reads.length, 1 + configReadsAtLoad(h));
});

// configReadsAtLoad answers the count of GET /api/config that the page
// sends while it loads. checkSession of omn-go-api.js sends one.
function configReadsAtLoad() {
    const fresh = configPage();
    return fresh.requests.filter(function (r) { return r.url === '/api/config' && r.method === 'GET'; }).length;
}

test('a reveal does not replace the text of a box that the person typed in', async () => {
    const h = configPage();
    h.server = configServer;
    h.type('#omn-cfg-admin-pwd', 'my new one');
    h.press(REVEAL);
    await h.settle();
    assert.strictEqual(h.$('#omn-cfg-admin-pwd').value, 'my new one');
    assert.strictEqual(h.$('#omn-git-pass-0').value, 'pass-zero');
});

test('a reveal that the server refuses fills nothing and says why', async () => {
    const h = configPage();
    h.server = function (request) {
        if (request.url === '/api/config' && h.refuse) return { httpStatus: 401, text: 'no' };
        return configServer(request);
    };
    h.refuse = true;
    h.press(REVEAL);
    await h.settle();
    assert.strictEqual(h.$('#omn-cfg-admin-pwd').value, '');
    assert.strictEqual(h.$(REVEAL + ' [data-reveal-label]').textContent, 'Show passwords');
    assert.strictEqual(h.dialogs[h.dialogs.length - 1].text, 'Log in as admin on a note page to read the passwords.');
});

test('a save that the server refuses keeps the page marked and does not load again', async () => {
    const h = configPage();
    h.server = function (request) {
        if (request.url === '/api/config' && request.method === 'POST') {
            return { httpStatus: 400, text: 'server_port must be a number' };
        }
        return configServer(request);
    };
    h.type('#omn-cfg-author', 'x');
    h.press(SAVE);
    await h.settle();
    assert.strictEqual(h.dialogs[h.dialogs.length - 1].text,
        'Could not save the configuration: server_port must be a number');
    assert.strictEqual(h.page.location.reloads, 0);
    assert.strictEqual(h.$('#omn-config-menu-dirty').hidden, false, 'the page reads as saved after a refusal');
});

test('a change of the LAN setting asks the server to restart', async () => {
    const h = configPage();
    h.server = function (request) {
        if (request.url === '/api/config' && request.method === 'POST') {
            return { httpStatus: 200, text: 'RestartRequired' };
        }
        // The connection ends while the server stops.
        if (request.url === '/api/restart') return new Error('connection reset');
        return configServer(request);
    };
    // The timers of the page, thus the test does not wait for them.
    const timers = [];
    h.page.setTimeout = function (fn, ms) { timers.push({ fn: fn, ms: ms }); };
    h.press(SAVE);
    await h.settle();
    const posts = h.requests.filter(function (r) { return r.method === 'POST'; }).map(function (r) { return r.url; });
    assert.deepStrictEqual(posts, ['/api/config', '/api/restart']);
    assert.ok(h.dialogs[h.dialogs.length - 1].text.indexOf('restart') >= 0);
    // The page loads again after 3 seconds, and not at once: the server is
    // not there yet.
    assert.strictEqual(h.page.location.reloads, 0);
    assert.deepStrictEqual(timers.map(function (t) { return t.ms; }), [3000]);
    timers[0].fn();
    assert.strictEqual(h.page.location.reloads, 1);
});
