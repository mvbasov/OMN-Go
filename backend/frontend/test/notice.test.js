// The notice of omn-go-core.js, in the REAL JavaScript and on the markup of
// the templates.
//
// A dialog is for a question or a fault. A result that needs no answer is
// a notice: one line at the bottom that goes away alone. A save of a quick
// note, a bookmark or the configuration loads the page again, thus the
// notice waits in sessionStorage for the next page.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { notePage } = require('./dom-page.js');

function notices(h) {
    return h.document.querySelectorAll('.omn-notice');
}

// The notice shows the text as text, and the screen reader reads it. It
// goes away alone after 2.5 seconds.
test('a notice shows its text and goes away after its time', () => {
    const h = notePage();
    h.holdTimers();
    h.page.omnNotice('Saved <b>x</b>');
    const el = notices(h)[0];
    assert.ok(el, 'no notice');
    assert.strictEqual(el.textContent, 'Saved <b>x</b>');
    assert.strictEqual(el.querySelector('b'), null, 'the text became markup');
    assert.strictEqual(el.getAttribute('role'), 'status');
    const timers = h.timers.filter(function (t) { return t.ms === 2500; });
    assert.strictEqual(timers.length, 1);
    timers[0].fn();
    assert.strictEqual(notices(h).length, 0, 'the notice stays after its time');
});

// Two results in a row must not stack two lines on the screen.
test('a second notice replaces the first one', () => {
    const h = notePage();
    h.page.omnNotice('one');
    h.page.omnNotice('two');
    assert.strictEqual(notices(h).length, 1);
    assert.strictEqual(notices(h)[0].textContent, 'two');
});

// A person who has read the notice can take it away at once.
test('a press on the notice removes it', () => {
    const h = notePage();
    h.page.omnNotice('Saved.');
    notices(h)[0].click();
    assert.strictEqual(notices(h).length, 0);
});

// A save loads the page again. The notice of the save shows on the new
// page, one time.
test('a notice kept for the next page shows after the load, one time', () => {
    const before = notePage();
    before.page.omnNoticeAfterLoad('Saved.');
    assert.strictEqual(before.session['omn-notice'], 'Saved.');

    const h = notePage({ session: { 'omn-notice': 'Saved.' } });
    assert.strictEqual(notices(h).length, 1);
    assert.strictEqual(notices(h)[0].textContent, 'Saved.');
    assert.strictEqual(h.session['omn-notice'], undefined, 'the notice would show again at the next load');
});

// A WebView can refuse sessionStorage. The save and the load must then
// work, with no notice.
test('a refused sessionStorage stops nothing', () => {
    const h = notePage();
    const refuse = function () { throw new Error('SecurityError'); };
    h.page.sessionStorage = { getItem: refuse, setItem: refuse, removeItem: refuse };
    h.page.omnNoticeAfterLoad('Saved.');
    h.page.dispatchEvent(new h.page.Event('load'));
    assert.strictEqual(notices(h).length, 0);
});

// A quick note that the server stored loads the page again, and the new
// page says "Saved." in a notice and not in a dialog.
test('a saved quick note keeps a notice for the next page', async () => {
    const h = notePage();
    h.server = function (request) {
        if (request.url === '/api/quick') return { httpStatus: 200, text: 'Saved' };
        return { status: 'success' };
    };
    h.page.omnShowPanel('omn-quick-panel');
    h.$('#omn-quick-text').value = 'a thought';
    h.press('[data-action="quick-note-save"]');
    await h.settle();
    assert.strictEqual(h.page.location.reloads, 1);
    assert.strictEqual(h.session['omn-notice'], 'Saved.');
    assert.strictEqual(h.dialogs.length, 0, 'the save still shows a dialog');
});
