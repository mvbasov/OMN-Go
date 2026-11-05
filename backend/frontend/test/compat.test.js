// The notice of a browser that is too old, in the REAL JavaScript.
//
// omn-go-compat.js is the one ES5 script of the application. A WebView
// that cannot read the other scripts drops each of them. This notice is
// then the only text that tells the person what to update. See the banner
// of the script.
//
// THE NUMBER IS 85. TestCompatScriptIsFirstAndES5 holds the place of the
// script and its syntax. This file holds what the script does.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { newDomPage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

// loaded answers a page with this user agent after the script ran and the
// document is ready.
function loaded(userAgent) {
    const h = newDomPage({ body: '<div id="omn-main-ui">the page</div>' });
    h.page.navigator.userAgent = userAgent;
    h.load(['omn-go-compat.js']);
    return h;
}

function notice(h) {
    return h.document.getElementById('omn-compat-bar');
}

const OLD = 'Mozilla/5.0 (Linux; Android 6.0; wv) AppleWebKit/537.36 Chrome/44.0.2403.119 Mobile Safari/537.36';
const LIMIT = 'Mozilla/5.0 (Linux; Android 10) AppleWebKit/537.36 Chrome/85.0.4183.81 Mobile Safari/537.36';
const BELOW = 'Mozilla/5.0 (Linux; Android 9) AppleWebKit/537.36 Chrome/84.0.4147.125 Mobile Safari/537.36';
const FIREFOX = 'Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0';

test('an old Chromium gets the notice, with its version and the version that it needs', () => {
    const h = loaded(OLD).ready();
    const bar = notice(h);
    assert.ok(bar, 'an old browser gets no notice');
    assert.ok(bar.textContent.indexOf('Chromium 44') >= 0, bar.textContent);
    assert.ok(bar.textContent.indexOf('needs 85') >= 0, bar.textContent);
    // The notice is the first element of the body, above the page.
    assert.strictEqual(h.document.body.firstChild, bar);
});

test('the limit is 85: version 84 gets the notice and version 85 does not', () => {
    assert.ok(notice(loaded(BELOW).ready()), 'version 84 gets no notice');
    assert.strictEqual(notice(loaded(LIMIT).ready()), null, 'version 85 gets the notice');
});

test('a browser with no Chrome token gets no notice', () => {
    // The number does not measure Firefox or Safari.
    assert.strictEqual(notice(loaded(FIREFOX).ready()), null);
    assert.strictEqual(notice(loaded('').ready()), null);
});

test('the notice shows one time, also when both load events run', () => {
    const h = loaded(OLD);
    // ready sends DOMContentLoaded and then load. The script listens to
    // both, because a page of the cache can have sent the first one.
    h.ready();
    assert.strictEqual(h.document.querySelectorAll('#omn-compat-bar').length, 1);
});

test('the load event alone is enough', () => {
    const h = loaded(OLD);
    h.page.dispatchEvent(new Event('load'));
    assert.ok(notice(h));
});

test('the close mark removes the notice', () => {
    const h = loaded(OLD).ready();
    const mark = notice(h).querySelector('span');
    assert.ok(mark, 'the notice has no close mark');
    mark.click();
    assert.strictEqual(notice(h), null);
});

test('the notice styles itself, because an old browser cannot read the stylesheet', () => {
    const h = loaded(OLD).ready();
    // omn-go-core.css uses custom properties, which Chromium 44 does not
    // have. The notice is thus the one place where an inline style is
    // correct.
    assert.ok((notice(h).getAttribute('style') || '').indexOf('background:') >= 0);
});
