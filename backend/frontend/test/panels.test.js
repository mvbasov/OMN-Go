// The panels of modals.html and the top layer, in the REAL JavaScript and on
// the markup of the templates.
//
// Quick Note and Bookmark are panels. Only one panel shows at a time. Each
// way to open a panel goes through omnShowPanel or omnTogglePanel of
// omn-go-core.js. These ways are the header buttons, a share from Android,
// a capture, and the Quick Note icon of Android.
//
// omnCloseTopLayer closes the top layer for the Back key of Android and for
// Escape: first a dialog, then the search overlay, then a panel.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

function shown(h, id) {
    return !h.$('#' + id).classList.contains('omn-hidden');
}

function escape(h) {
    const event = new Event('keydown', { bubbles: true, cancelable: true, key: 'Escape' });
    h.document.body.dispatchEvent(event);
    return event;
}

test('one panel shows at a time, and the header buttons toggle them', async () => {
    const h = notePage();
    h.press('[data-action="toggle-panel"][data-arg="omn-quick-panel"]');
    assert.ok(shown(h, 'omn-quick-panel'));
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    assert.ok(shown(h, 'omn-bm-panel'));
    assert.ok(!shown(h, 'omn-quick-panel'), 'two panels show');
    h.press('[data-action="bookmark-panel"]');
    await h.settle();
    assert.ok(!shown(h, 'omn-bm-panel'));
    h.press('[data-action="toggle-panel"][data-arg="omn-quick-panel"]');
    h.press('[data-action="hide-panel"][data-arg="omn-quick-panel"]');
    assert.ok(!shown(h, 'omn-quick-panel'));
});

test('the metadata panel does not close a panel of modals.html', () => {
    const h = notePage();
    h.page.omnShowPanel('omn-quick-panel');
    h.press('#omn-meta-toggle-btn');
    assert.ok(shown(h, 'omn-metadata-panel'));
    assert.ok(shown(h, 'omn-quick-panel'));
});

test('a capture and a share open the right panel and close the other one', async () => {
    const h = notePage();
    h.page.omnShowPanel('omn-bm-panel');
    assert.strictEqual(h.page.omnGoInsertCapture('12345', 'Barcode'), true);
    assert.ok(shown(h, 'omn-quick-panel'));
    assert.ok(!shown(h, 'omn-bm-panel'));
    assert.strictEqual(h.$('#omn-quick-text').value, 'Barcode\n\n12345');

    h.page.handleShare('see https://example.org/a', 'A page');
    await h.settle();
    assert.ok(shown(h, 'omn-bm-panel'));
    assert.ok(!shown(h, 'omn-quick-panel'));
    assert.strictEqual(h.$('#omn-bm-url').value, 'https://example.org/a');
});

test('a page without the panels answers false and does not throw', () => {
    const h = notePage({ body: '<div id="omn-main-ui"></div>' });
    assert.strictEqual(h.page.omnShowPanel('omn-quick-panel'), false);
    assert.strictEqual(h.page.omnTogglePanel('omn-quick-panel'), false);
    h.page.omnHidePanel('omn-quick-panel');
    assert.strictEqual(h.page.omnGoInsertCapture('x'), false);
    assert.strictEqual(h.page.omnCloseTopLayer(), false);
});

test('the top layer closes first: a dialog, then the search overlay, then a panel', async () => {
    const h = notePage();
    h.keepDialogs = true;
    h.page.omnShowPanel('omn-quick-panel');
    let searchOpen = true;
    h.page.omnSearchIsOpen = function () { return searchOpen; };
    h.page.omnSearchClose = function () { searchOpen = false; };
    const answer = h.page.omnConfirm('Go on?');
    await h.settle();
    assert.ok(h.document.querySelector('.omn-dialog'), 'the dialog did not open');

    assert.strictEqual(h.page.omnCloseTopLayer(), true);
    assert.strictEqual(await answer, false);
    assert.ok(searchOpen && shown(h, 'omn-quick-panel'), 'the Back key closed two layers');

    assert.strictEqual(h.page.omnCloseTopLayer(), true);
    assert.ok(!searchOpen && shown(h, 'omn-quick-panel'));

    assert.strictEqual(h.page.omnCloseTopLayer(), true);
    assert.ok(!shown(h, 'omn-quick-panel'));

    assert.strictEqual(h.page.omnCloseTopLayer(), false, 'the WebView would not go back');
});

test('Escape closes an open panel, and a used Escape does not', () => {
    const h = notePage();
    h.page.omnShowPanel('omn-quick-panel');
    const used = new Event('keydown', { bubbles: true, cancelable: true, key: 'Escape' });
    used.preventDefault();
    h.document.body.dispatchEvent(used);
    assert.ok(shown(h, 'omn-quick-panel'), 'an Escape of the tag list closed the panel');
    assert.ok(escape(h).defaultPrevented);
    assert.ok(!shown(h, 'omn-quick-panel'));
    assert.strictEqual(escape(h).defaultPrevented, false, 'Escape with no layer is not used');
});
