// Tests of omn-go-dialog.js: the dialogs that replace alert(), confirm() and
// prompt() of the browser. See also the shortcut question of omn-go-core.js.
//
// Most tests set h.keepDialogs, thus the page does not answer the dialog,
// and the test presses the buttons and the keys itself.

const test = require('node:test');
const assert = require('node:assert');
const { newDomPage, notePage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

// dialogPage answers an empty page with the dialog file only.
function dialogPage() {
    const h = newDomPage({ body: '<button id="omn-before">Before</button>' }).load(['omn-go-dialog.js']).ready();
    h.keepDialogs = true;
    return h;
}

// open answers the one open dialog, or null.
function open(h) {
    const all = h.document.querySelectorAll('.omn-dialog');
    assert.ok(all.length <= 1, 'two dialogs show at one time');
    return all[0] || null;
}

// key sends a keydown to an element of the open dialog. more holds the
// state of Shift and Ctrl, for example {shiftKey: true}.
function key(el, name, more) {
    const event = new Event('keydown', Object.assign({ bubbles: true, cancelable: true, key: name }, more || {}));
    el.dispatchEvent(event);
    return event;
}

test('omnConfirm answers true for OK and false for Cancel, and removes the dialog', async () => {
    const h = dialogPage();
    const yes = h.page.omnConfirm('Delete it?', 'Delete', 'Keep');
    const box = open(h);
    assert.strictEqual(box.getAttribute('data-omn-dialog'), 'confirm');
    assert.strictEqual(box.getAttribute('role'), 'dialog');
    assert.strictEqual(box.querySelector('.omn-dialog-text').textContent, 'Delete it?');
    const labels = box.querySelectorAll('button').map(function (b) { return b.textContent; });
    assert.deepStrictEqual(labels, ['Delete', 'Keep']);
    assert.ok(box.querySelector('[data-omn-cancel]').classList.contains('omn-btn-cancel'));
    box.querySelector('[data-omn-default]').click();
    assert.strictEqual(await yes, true);
    assert.strictEqual(open(h), null);

    const no = h.page.omnConfirm('Delete it?');
    open(h).querySelector('[data-omn-cancel]').click();
    assert.strictEqual(await no, false);
    assert.strictEqual(open(h), null);
});

test('Escape gives the cancel button, and Enter gives the default button', async () => {
    const h = dialogPage();
    // A key handler of the page under the dialog does not see the two keys.
    const seen = [];
    h.document.addEventListener('keydown', function (e) { seen.push(e.key); });

    const escape = h.page.omnConfirm('Go on?');
    const esc = key(open(h).querySelector('[data-omn-default]'), 'Escape');
    assert.strictEqual(await escape, false);
    assert.ok(esc.defaultPrevented);

    const enter = h.page.omnConfirm('Go on?');
    key(open(h).querySelector('.omn-dialog-text'), 'Enter');
    assert.strictEqual(await enter, true);
    assert.deepStrictEqual(seen, []);

    // Escape closes an alert, which has no cancel button.
    const alert = h.page.omnAlert('Done.');
    key(open(h).querySelector('button'), 'Escape');
    assert.strictEqual(await alert, undefined);
});

test('omnPrompt shows the start value, and answers the text or null', async () => {
    const h = dialogPage();
    const typed = h.page.omnPrompt('File name:', 'MyNote', 'Create');
    const field = open(h).querySelector('.omn-dialog-input');
    assert.strictEqual(field.value, 'MyNote');
    assert.strictEqual(h.document.activeElement, field, 'the field has no focus');
    field.value = 'Other';
    key(field, 'Enter');
    assert.strictEqual(await typed, 'Other');

    const canceled = h.page.omnPrompt('File name:');
    assert.strictEqual(open(h).querySelector('.omn-dialog-input').value, '');
    open(h).querySelector('[data-omn-cancel]').click();
    assert.strictEqual(await canceled, null);
});

test('a text with markup stays text, and a line break stays in the text', async () => {
    const h = dialogPage();
    const done = h.page.omnAlert('<b>bold</b>\nnext line', 'A <i>title</i>');
    const box = open(h);
    assert.strictEqual(box.querySelector('b'), null);
    assert.strictEqual(box.querySelector('i'), null);
    assert.strictEqual(box.querySelector('.omn-dialog-text').textContent, '<b>bold</b>\nnext line');
    assert.strictEqual(box.querySelector('.omn-dialog-title').textContent, 'A <i>title</i>');
    box.querySelector('button').click();
    await done;
});

test('a second dialog waits until the first one closes', async () => {
    const h = dialogPage();
    const first = h.page.omnConfirm('First?');
    const second = h.page.omnAlert('Second.');
    assert.strictEqual(open(h).querySelector('.omn-dialog-text').textContent, 'First?');
    open(h).querySelector('[data-omn-default]').click();
    assert.strictEqual(await first, true);
    assert.strictEqual(open(h).querySelector('.omn-dialog-text').textContent, 'Second.');
    open(h).querySelector('button').click();
    await second;
    assert.strictEqual(open(h), null);
});

test('the focus goes back to the element that had it before the dialog', async () => {
    const h = dialogPage();
    const before = h.document.getElementById('omn-before');
    before.focus();
    const done = h.page.omnConfirm('Go on?');
    assert.strictEqual(h.document.activeElement, open(h).querySelector('[data-omn-default]'));
    open(h).querySelector('[data-omn-default]').click();
    await done;
    assert.strictEqual(h.document.activeElement, before);
});

test('omnDialog answers the value of the pressed button of a choice', async () => {
    const h = dialogPage();
    const done = h.page.omnDialog({
        kind: 'choice',
        text: 'Which one?',
        buttons: [
            { label: 'A', value: 'a', default: true },
            { label: 'B', value: 'b' },
            { label: 'Cancel', value: null, cancel: true },
        ],
    });
    const b = open(h).querySelectorAll('button').find(function (el) { return el.textContent === 'B'; });
    b.click();
    // The answer comes from the realm of the page, thus the test compares
    // the fields and not the object.
    const answer = await done;
    assert.strictEqual(answer.value, 'b');
    assert.strictEqual(answer.input, null);
});

test('a field can be a password or a textarea, and the answer holds each text', async () => {
    const h = dialogPage();
    const done = h.page.omnDialog({
        kind: 'form',
        fields: [
            { label: 'Password', type: 'password', autocomplete: 'current-password' },
            { label: 'Message', type: 'textarea', rows: 3, placeholder: 'Describe the change' },
        ],
        buttons: [{ label: 'Go', value: true, default: true }, { label: 'Cancel', value: false, cancel: true }],
    });
    const [pwd, text] = open(h).querySelectorAll('.omn-dialog-input');
    assert.strictEqual(pwd.tagName, 'INPUT');
    assert.strictEqual(pwd.type, 'password');
    assert.strictEqual(pwd.getAttribute('autocomplete'), 'current-password');
    assert.strictEqual(text.tagName, 'TEXTAREA');
    assert.strictEqual(text.rows, 3);
    assert.strictEqual(text.placeholder, 'Describe the change');
    pwd.value = 'secret';
    text.value = 'line 1\nline 2';
    open(h).querySelector('[data-omn-default]').click();
    const answer = await done;
    assert.strictEqual(answer.inputs.join('|'), 'secret|line 1\nline 2');
});

test('Enter in a textarea makes a new line, and Ctrl+Enter presses the default button', async () => {
    const h = dialogPage();
    const done = h.page.omnDialog({
        kind: 'form',
        fields: [{ type: 'textarea' }],
        buttons: [{ label: 'Save', value: 'saved', default: true }],
    });
    const text = open(h).querySelector('textarea');
    const enter = key(text, 'Enter');
    assert.strictEqual(enter.defaultPrevented, false, 'the browser gets no new line');
    assert.notStrictEqual(open(h), null, 'Enter closed the dialog');
    key(text, 'Enter', { ctrlKey: true });
    assert.strictEqual((await done).value, 'saved');
});

test('the content blocks show as text: a paragraph, a warning, a table and a list', async () => {
    const h = dialogPage();
    // An unknown block shows nothing, and the console names it.
    const errors = [];
    h.page.console = { error: function () { errors.push(arguments[0]); } };
    const shown = h.page.omnDialog({
        text: '',
        content: [
            { p: 'Replace <b>db</b>?' },
            { warning: 'The data is lost.' },
            { table: [['Current:', '12 KB'], ['Backup:', 'a<i>.jsonl']] },
            { list: ['md/A.md', 'md/<B>.md'] },
            { unknown: 'x' },
            null,
        ],
    });
    const box = open(h);
    assert.strictEqual(box.querySelector('b'), null);
    assert.strictEqual(box.querySelector('i'), null);
    assert.strictEqual(box.querySelector('.omn-dialog-p').textContent, 'Replace <b>db</b>?');
    assert.strictEqual(box.querySelector('.omn-dialog-warning').textContent, 'The data is lost.');
    const cells = box.querySelectorAll('.omn-dialog-table th, .omn-dialog-table td').map(function (c) { return c.textContent; });
    assert.strictEqual(cells.join('|'), 'Current:|12 KB|Backup:|a<i>.jsonl');
    const items = box.querySelectorAll('.omn-dialog-list li').map(function (li) { return li.textContent; });
    assert.strictEqual(items.join('|'), 'md/A.md|md/<B>.md');
    assert.strictEqual(errors.length, 1);
    box.querySelector('button').click();
    await shown;
});

test('a button with a hint has two lines, and a danger button is red', async () => {
    const h = newDomPage({ body: '' }).load(['omn-go-dialog.js']).ready();
    h.choose = function () { return 'Force Pull'; };
    const done = h.page.omnDialog({
        kind: 'choice',
        text: 'Conflict.',
        width: '500px',
        buttons: [
            { label: 'Force Pull', hint: 'Overwrites the tracked files.', value: 'pull_force', danger: true },
            { label: 'Mark Files', hint: 'Puts markers into the files.', value: 'pull_mark' },
            { label: 'Abort', value: 'abort', cancel: true },
        ],
    });
    const box = open(h);
    assert.strictEqual(box.querySelector('.omn-modal').style.width, '500px');
    assert.ok(box.querySelector('.omn-modal-buttons-row').classList.contains('omn-dialog-buttons-column'));
    const first = box.querySelector('button');
    assert.strictEqual(first.querySelector('.omn-dialog-button-label').textContent, 'Force Pull');
    assert.strictEqual(first.querySelector('.omn-dialog-button-hint').textContent, 'Overwrites the tracked files.');
    assert.ok(first.classList.contains('omn-btn-danger'));
    assert.ok(!box.querySelector('[data-omn-cancel]').classList.contains('omn-btn-danger'));
    // h.choose finds a button with a hint by its label.
    await h.settle();
    assert.strictEqual((await done).value, 'pull_force');
});

test('check keeps the dialog open with its error text, and Cancel skips check', async () => {
    const h = dialogPage();
    const seen = [];
    const done = h.page.omnDialog({
        kind: 'form',
        fields: [{ label: 'Message' }],
        check: function (answer) {
            seen.push(answer.value + ':' + answer.input);
            return answer.input ? '' : 'Enter a commit message.';
        },
        buttons: [{ label: 'Commit', value: true, default: true }, { label: 'Cancel', value: false, cancel: true }],
    });
    const box = open(h);
    const error = box.querySelector('.omn-dialog-error');
    assert.strictEqual(error.textContent, '');
    assert.strictEqual(error.getAttribute('role'), 'alert');
    box.querySelector('[data-omn-default]').click();
    await h.settle();
    assert.strictEqual(open(h), box, 'an error closed the dialog');
    assert.strictEqual(error.textContent, 'Enter a commit message.');

    box.querySelector('.omn-dialog-input').value = 'Fix';
    key(box.querySelector('.omn-dialog-input'), 'Enter');
    const answer = await done;
    assert.strictEqual(answer.value, true);
    assert.strictEqual(seen.join(','), 'true:,true:Fix');

    // Cancel closes the dialog without a call of check.
    const canceled = h.page.omnDialog({
        check: function () { throw new Error('check ran for Cancel'); },
        buttons: [{ label: 'OK', value: true, default: true }, { label: 'Cancel', value: false, cancel: true }],
    });
    open(h).querySelector('[data-omn-cancel]').click();
    assert.strictEqual((await canceled).value, false);
});

test('check can answer a Promise, and the buttons wait for it', async () => {
    const h = dialogPage();
    let finishCheck;
    const done = h.page.omnDialog({
        check: function () { return new Promise(function (resolve) { finishCheck = resolve; }); },
        buttons: [{ label: 'Enter', value: true, default: true }],
    });
    const box = open(h);
    const button = box.querySelector('button');
    button.click();
    assert.strictEqual(button.disabled, true, 'a second press can start while check runs');
    assert.strictEqual(box.getAttribute('aria-busy'), 'true');
    // The Back key does nothing while check runs.
    assert.strictEqual(h.page.omnCloseTopLayer(), true);
    finishCheck('Invalid password.');
    await h.settle();
    assert.strictEqual(button.disabled, false);
    assert.strictEqual(box.querySelector('.omn-dialog-error').textContent, 'Invalid password.');
    assert.strictEqual(box.getAttribute('aria-busy'), null);

    button.click();
    finishCheck('');
    assert.strictEqual((await done).value, true);

    // A check that fails shows the message of the error.
    const failed = h.page.omnDialog({
        check: function () { return Promise.reject(new Error('network down')); },
        buttons: [{ label: 'OK', value: true, default: true }, { label: 'Cancel', value: false, cancel: true }],
    });
    open(h).querySelector('[data-omn-default]').click();
    await h.settle();
    assert.strictEqual(open(h).querySelector('.omn-dialog-error').textContent, 'network down');
    open(h).querySelector('[data-omn-cancel]').click();
    await failed;
});

test('omnCloseTopLayer cancels the open dialog, and answers false without one', async () => {
    const h = dialogPage();
    assert.strictEqual(h.page.omnCloseTopLayer(), false);
    const done = h.page.omnConfirm('Go on?');
    assert.strictEqual(h.page.omnCloseTopLayer(), true);
    assert.strictEqual(await done, false);
    assert.strictEqual(open(h), null);
    assert.strictEqual(h.page.omnCloseTopLayer(), false);
});

test('a dialog that is not closable ignores Escape and the Back key', async () => {
    const h = dialogPage();
    const done = h.page.omnDialog({
        kind: 'login',
        closable: false,
        fields: [{ type: 'password' }],
        buttons: [{ label: 'Enter', value: true, default: true }],
    });
    const box = open(h);
    const esc = key(box.querySelector('input'), 'Escape');
    assert.ok(esc.defaultPrevented);
    assert.strictEqual(h.page.omnCloseTopLayer(), true, 'the WebView would go back under the login');
    assert.strictEqual(open(h), box);
    box.querySelector('button').click();
    assert.strictEqual((await done).value, true);
});

test('Tab and Shift+Tab keep the focus in the dialog', async () => {
    const h = dialogPage();
    const done = h.page.omnPrompt('Name:', 'x');
    const box = open(h);
    const field = box.querySelector('input');
    const buttons = box.querySelectorAll('button');
    const last = buttons[buttons.length - 1];
    last.focus();
    assert.ok(key(last, 'Tab').defaultPrevented);
    assert.strictEqual(h.document.activeElement, field);
    assert.ok(key(field, 'Tab', { shiftKey: true }).defaultPrevented);
    assert.strictEqual(h.document.activeElement, last);
    // Inside the dialog, Tab is left to the browser.
    field.focus();
    assert.strictEqual(key(field, 'Tab').defaultPrevented, false);
    last.click();
    await done;
});

// The shortcut button of omn-go-core.js. Android 8 and later has two ways
// to pin a shortcut, thus the page asks which one.
function shortcutPage(userAgent) {
    const h = notePage({ note: 'dir/Shop', title: 'Grocery List' });
    h.page.navigator.userAgent = userAgent;
    return h;
}
const SHORTCUT = '[data-action="add-shortcut"]';
const ANDROID_14 = 'Mozilla/5.0 (Linux; Android 14; Pixel 7) AppleWebKit/537.36';
const ANDROID_7 = 'Mozilla/5.0 (Linux; Android 7.1.2; Nexus 5X) AppleWebKit/537.36';

test('the shortcut button asks for the way on Android 8 and later', async () => {
    const h = shortcutPage(ANDROID_14);
    h.choose = function () { return 'Quick'; };
    h.press(SHORTCUT);
    await h.settle();
    assert.strictEqual(h.dialogs.length, 1);
    assert.strictEqual(h.dialogs[0].kind, 'choice');
    assert.deepStrictEqual(h.went, ['omngo://shortcut?name=dir%2FShop&title=Grocery%20List&mode=quick']);

    // Cancel opens no address.
    const c = shortcutPage(ANDROID_14);
    c.press(SHORTCUT);
    await c.settle();
    assert.strictEqual(c.dialogs.length, 1);
    assert.deepStrictEqual(c.went, []);
});

test('the shortcut button asks nothing on Android 7', async () => {
    const h = shortcutPage(ANDROID_7);
    h.press(SHORTCUT);
    await h.settle();
    assert.deepStrictEqual(h.dialogs, []);
    assert.deepStrictEqual(h.went, ['omngo://shortcut?name=dir%2FShop&title=Grocery%20List']);
});

// The New page control asks with one form: the title and the file name.
// typeInto sets the value of a field and sends the input event of a browser.
function typeInto(field, value) {
    field.value = value;
    field.dispatchEvent(new Event('input', { bubbles: true }));
}
const NEW_PAGE = '[data-action="new-page"]';

test('the file name of a new page follows the title until the person types one', async () => {
    const h = notePage({ note: 'dir/Here' });
    h.keepDialogs = true;
    h.press(NEW_PAGE);
    await h.settle();
    const form = open(h);
    assert.strictEqual(form.getAttribute('data-omn-dialog'), 'form');
    const labels = form.querySelectorAll('.omn-dialog-label').map(function (l) { return l.textContent; });
    assert.deepStrictEqual(labels, ['Title', 'File name']);
    const [title, name] = form.querySelectorAll('.omn-dialog-input');
    assert.strictEqual(h.document.activeElement, title, 'the title field has no focus');

    typeInto(title, 'my new note');
    assert.strictEqual(name.value, 'MyNewNote');
    typeInto(name, 'Other');
    typeInto(title, 'my new note 2');
    assert.strictEqual(name.value, 'Other', 'the title wrote over a typed file name');
    // An empty file name follows the title again.
    typeInto(name, '');
    typeInto(title, 'shop list');
    assert.strictEqual(name.value, 'ShopList');
});

test('Create sends the title and the file name of the form', async () => {
    const h = notePage({ note: 'dir/Here' });
    h.server = function (request) {
        if (request.url === '/api/newpage') return { httpStatus: 200, text: 'dir/ShopList' };
        return { status: 'success' };
    };
    h.fill = function (text, values) {
        assert.deepStrictEqual(values, ['', '']);
        return ['shop list'];
    };
    h.press(NEW_PAGE);
    await h.settle();
    const posts = h.requests.filter(function (r) { return r.url === '/api/newpage'; });
    assert.strictEqual(posts.length, 1);
    assert.strictEqual(String(posts[0].body), 'source=dir%2FHere&target=ShopList&title=shop+list');
    assert.deepStrictEqual(h.went, ['/dir/ShopList.html?edit=true']);

    // Cancel sends nothing.
    const c = notePage();
    c.press(NEW_PAGE);
    await c.settle();
    assert.strictEqual(c.requests.filter(function (r) { return r.url === '/api/newpage'; }).length, 0);
});
