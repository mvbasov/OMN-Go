// The links of a note and the overlay of a slow page, in the REAL
// JavaScript and on the markup of the templates.
//
// A PRESS ON A LINK OF A NOTE HAS FOUR ANSWERS, and each one had a fault
// before:
//
//   - A link to a note opens the address that the server wrote.
//   - A link to another site opens a new tab, and the app stays.
//   - A link with another scheme, for example tel:, stays with the browser.
//   - A link inside the page stays with the browser.
//
// The overlay of a slow page must not show for a fast page. It must never
// show for a press that leaves this page in place.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const NOTE =
    '<p><a id="note" href="Other.html?x=1#part">a note</a> ' +
    '<a id="deep" href="/dir/Deep.html"><b id="bold">a deep note</b></a> ' +
    '<a id="site" href="https://example.org/a">a site</a> ' +
    '<a id="site2" href="//example.org/b">a site with no scheme</a> ' +
    '<a id="tel" href="tel:5551234">a phone number</a> ' +
    '<a id="mail" href="mailto:a@example.org">a mail address</a> ' +
    '<a id="intent" href="intent://scan/#Intent;scheme=zxing;end">an intent</a> ' +
    '<a id="anchor" href="#part">a place in this page</a> ' +
    '<a id="script" href="javascript:void(0)">a script link</a> ' +
    '<a id="same" href="/Note.html">this page</a> ' +
    '<a id="tab" href="/Other.html" target="_blank">a new tab</a> ' +
    '<a id="file" href="/files/a.zip" download>a download</a> ' +
    '<a id="empty">no address</a></p>';

// page answers a note page with the links above. Its timers wait for the
// test.
function page(opts) {
    return notePage(Object.assign({ preview: NOTE }, opts || {})).holdTimers();
}

// press sends a click to a link and answers the event.
function press(h, id, init) {
    const event = new Event('click', Object.assign({ bubbles: true }, init || {}));
    h.$('#' + id).dispatchEvent(event);
    return event;
}

test('a link to a note opens the address that the server wrote', () => {
    const h = page();
    const event = press(h, 'note');
    assert.strictEqual(event.defaultPrevented, true);
    // The query and the fragment stay as they are. An old fault added
    // ".html" at the end of them.
    assert.deepStrictEqual(h.went, ['Other.html?x=1#part']);
    assert.deepStrictEqual(h.opened, []);
});

test('a press on an element inside a link is a press on the link', () => {
    const h = page();
    press(h, 'bold');
    assert.deepStrictEqual(h.went, ['/dir/Deep.html']);
});

test('a link to another site opens a new tab, and the app stays', () => {
    const h = page();
    assert.strictEqual(press(h, 'site').defaultPrevented, true);
    assert.strictEqual(press(h, 'site2').defaultPrevented, true);
    assert.deepStrictEqual(h.opened, [
        { url: 'https://example.org/a', target: '_blank' },
        { url: '//example.org/b', target: '_blank' },
    ]);
    assert.deepStrictEqual(h.went, []);
});

test('a link with another scheme stays with the browser', () => {
    const h = page();
    for (const id of ['tel', 'mail', 'intent']) {
        // An old fault made "tel:5551234.html" of such an address.
        assert.strictEqual(press(h, id).defaultPrevented, false, id + ' did not reach the browser');
    }
    assert.deepStrictEqual(h.went, []);
    assert.deepStrictEqual(h.opened, []);
});

test('a link inside the page and a script link stay with the browser', () => {
    const h = page();
    assert.strictEqual(press(h, 'anchor').defaultPrevented, false);
    assert.strictEqual(press(h, 'script').defaultPrevented, false);
    assert.strictEqual(press(h, 'empty').defaultPrevented, false);
    assert.deepStrictEqual(h.went, []);
});

test('a link to another page arms the overlay, and it shows after 300 ms', () => {
    const h = page();
    press(h, 'deep');
    assert.deepStrictEqual(h.pendingTimers(), [300]);
    // Nothing shows before the time is over, thus a fast page shows no
    // overlay at all.
    assert.strictEqual(h.document.querySelector('.omn-progress-overlay'), null);
    h.runTimers();
    assert.strictEqual(h.$('.omn-progress-overlay').hidden, false);
    assert.strictEqual(h.$('.omn-progress-title').textContent, 'Loading');
    assert.strictEqual(h.$('.omn-progress-detail').textContent, '/dir/Deep.html');
});

test('a press that leaves this page in place arms no overlay', () => {
    const h = page();
    for (const id of ['anchor', 'same', 'tab', 'file', 'site', 'tel', 'empty']) {
        press(h, id);
        assert.deepStrictEqual(h.pendingTimers(), [], 'the link ' + id + ' armed the overlay');
    }
});

test('a press with a modifier key or another mouse button arms no overlay', () => {
    const h = page();
    // Ctrl and the middle button open a new tab in a browser.
    for (const init of [{ ctrlKey: true }, { metaKey: true }, { shiftKey: true }, { altKey: true }, { button: 1 }]) {
        const event = new Event('click', Object.assign({ bubbles: true }, init));
        // The body, and not #omn-preview: the link handler of the note is not
        // the subject here.
        const link = h.document.createElement('a');
        link.setAttribute('href', '/Other.html');
        h.document.body.appendChild(link);
        link.dispatchEvent(event);
        assert.deepStrictEqual(h.pendingTimers(), [], JSON.stringify(init) + ' armed the overlay');
    }
});

test('a second press starts the wait again, and a new hash stops it', () => {
    const h = page();
    press(h, 'deep');
    press(h, 'note');
    assert.deepStrictEqual(h.pendingTimers(), [300], 'two overlays wait at one time');
    h.page.location.hash = '#cfg-network';
    assert.deepStrictEqual(h.pendingTimers(), []);
});

test('the form of the results page arms the overlay, and another form does not', () => {
    const h = page();
    const form = h.document.createElement('form');
    form.className = 'omn-search-page-form';
    h.document.body.appendChild(form);
    const other = h.document.createElement('form');
    h.document.body.appendChild(other);

    // A form of a note is not a search, and "Searching" over it is a lie.
    other.dispatchEvent(new Event('submit', { bubbles: true }));
    assert.deepStrictEqual(h.pendingTimers(), []);

    // A handler that stops the submit ran first, thus nothing leaves.
    const stopped = new Event('submit', { bubbles: true });
    form.addEventListener('submit', function (e) { e.preventDefault(); });
    form.dispatchEvent(stopped);
    assert.deepStrictEqual(h.pendingTimers(), []);

    const fresh = h.document.createElement('form');
    fresh.className = 'omn-search-page-form';
    h.document.body.appendChild(fresh);
    fresh.dispatchEvent(new Event('submit', { bubbles: true }));
    assert.deepStrictEqual(h.pendingTimers(), [300]);
    h.runTimers();
    assert.strictEqual(h.$('.omn-progress-title').textContent, 'Searching');
});

test('a page from disk arms no overlay, and its links still work', () => {
    const h = page({ protocol: 'file:' });
    press(h, 'deep');
    assert.deepStrictEqual(h.pendingTimers(), []);
    assert.deepStrictEqual(h.went, ['/dir/Deep.html']);
});
