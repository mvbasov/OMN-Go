// The bookmark page, in the REAL JavaScript: Bookmarker.js on a note page.
//
// md/Bookmarks.md holds the bookmarks as a list in a script block, and it
// then loads Bookmarker.js. The script builds the whole page: the search
// box, the buttons, the tag cloud and the list. bookmarkPage of dom-page.js
// does the same with a list that the test gives.
//
// omn-go-bookmark.js is a different file. It is the panel that ADDS a
// bookmark, and bookmark.test.js holds its tests.
//
// The scripts of index.html load before Bookmarker.js, the same as in the
// application. Bookmarker.js is a classic script with names at the top
// level, thus a name that an application script also has stops its load.
// Each test below would then fail.

'use strict';

const test = require('node:test');
const assert = require('node:assert');
const { bookmarkPage } = require('./dom-page.js');
const { Event } = require('./mini-dom.js');

const ALPHA = {
    date: '2026-01-01 10:00:00', url: 'https://a.example/page', title: 'Alpha',
    tags: ['Go', 'Reference'], notes: ['first note', 'second note'],
};
const VIDEO = {
    date: '2026-03-01 10:00:00', url: 'https://www.youtube.com/watch?v=abc&si=one', title: 'Video',
    tags: ['Video'], notes: [],
};
// A second share of the same video, with no title.
const VIDEO_AGAIN = {
    date: '2026-02-01 10:00:00', url: 'https://www.youtube.com/watch?v=abc&si=two', title: '',
    tags: ['Video', 'Go'],
};
const HEDGEHOG = { date: '2026-05-01 10:00:00', url: 'https://e.example/', title: 'Ёжик, ёлка, café' };
const NO_ADDRESS = { date: '2026-04-01 10:00:00', url: '', title: 'no address' };

// The list is not in the order of the dates.
const BOOKMARKS = [ALPHA, VIDEO, VIDEO_AGAIN, HEDGEHOG, NO_ADDRESS];

const KEY = 'OMNBookmarkerConfigG';
const ALL = [HEDGEHOG.title, 'Video', VIDEO_AGAIN.url, 'Alpha'];

// shown answers the link text of each bookmark of the list, in its order.
function shown(h) {
    return h.$('#omn-bm-list').querySelectorAll('ul > li > a').map(function (a) { return a.textContent; });
}

// entry answers the list item of one bookmark.
function entry(h, bm) {
    const li = h.document.getElementById(bm.date.replace(/:/g, '').replace(/ /g, '-'));
    assert.ok(li, 'the list has no item for ' + bm.date);
    return li;
}

// button answers the button with one text inside the element of a selector.
function button(h, selector, text) {
    const found = h.$(selector).querySelectorAll('button').filter(function (b) { return b.textContent === text; });
    assert.ok(found.length > 0, selector + ' has no button "' + text + '"');
    return found[0];
}

// search types a pattern and presses the Search button.
function search(h, pattern) {
    h.$('#omn-bm-search-input').value = pattern;
    h.press('#omn-bm-search-btn');
    return shown(h);
}

// stored answers the options that the page keeps in localStorage.
function stored(h) { return JSON.parse(h.storage[KEY]); }

// alerts answers the text of each alert.
function alerts(h) {
    return h.dialogs.filter(function (d) { return d.kind === 'alert'; }).map(function (d) { return d.text; });
}

// ---------------------------------------------------------------------
// The page
// ---------------------------------------------------------------------

test('the script loads after the scripts of the application and builds the page', () => {
    // The controls go above the text that the note already has.
    const h = bookmarkPage(BOOKMARKS, { preview: '<p id="text">The text of the note.</p>' });
    const ids = h.$('#omn-preview').children.map(function (c) { return c.id; });
    assert.deepStrictEqual(ids, ['omn-bm-search-box', 'omn-bm-buttons', 'omn-bm-tags-cloud', 'omn-bm-list', 'omn-bm-version', 'text']);
    assert.match(h.$('#omn-bm-version').textContent, /^\d+\.\d+\w* \d{4}-\d\d-\d\d /);
    assert.strictEqual(h.$('#omn-bm-search-input').type, 'search');
    assert.deepStrictEqual(h.requests.filter(function (r) { return r.method !== 'GET'; }), [],
        'the page of the list changed something on the server');
});

test('the list shows the newest bookmark first and skips one with no address', () => {
    const h = bookmarkPage(BOOKMARKS);
    assert.deepStrictEqual(shown(h), ALL);
    assert.strictEqual(h.$('#omn-bm-counter').textContent, '4');

    const li = entry(h, ALPHA);
    assert.strictEqual(li.id, '2026-01-01-100000');
    assert.strictEqual(li.querySelector('a').getAttribute('href'), ALPHA.url);
    assert.strictEqual(li.querySelector('.omn-span-date').textContent, ALPHA.date);
    assert.ok(li.textContent.indexOf(ALPHA.url) >= 0, 'the item does not show the address');
});

test('an item shows its tags and its notes only after a press', () => {
    const h = bookmarkPage(BOOKMARKS);
    const li = entry(h, ALPHA);
    const details = li.querySelector('ul');
    assert.strictEqual(details.style.display, 'none');
    assert.deepStrictEqual(details.querySelectorAll('button.omn-tag').map((b) => b.textContent), ['Go', 'Reference']);
    assert.ok(details.textContent.indexOf('"first note", "second note"') >= 0);

    li.querySelector('button.omn-details').click();
    assert.strictEqual(details.style.display, 'block');
    li.querySelector('button.omn-details').click();
    assert.strictEqual(details.style.display, 'none');

    // A bookmark with no tag and no note has nothing to show.
    assert.strictEqual(entry(h, HEDGEHOG).querySelector('button.omn-details'), null);
});

test('Expand and Collapse open and close each item', () => {
    const h = bookmarkPage(BOOKMARKS);
    const blocks = [ALPHA, VIDEO, VIDEO_AGAIN].map(function (bm) { return entry(h, bm).querySelector('ul'); });
    button(h, '#omn-bm-buttons', 'Expand').click();
    assert.deepStrictEqual(blocks.map((b) => b.style.display), ['block', 'block', 'block']);
    button(h, '#omn-bm-buttons', 'Collapse').click();
    assert.deepStrictEqual(blocks.map((b) => b.style.display), ['none', 'none', 'none']);
});

test('a rest of the pointer on a link copies the address after 500 ms', async () => {
    const h = bookmarkPage(BOOKMARKS);
    h.holdTimers();
    entry(h, ALPHA).querySelector('a').dispatchEvent(new Event('mouseover', { bubbles: true }));
    assert.deepStrictEqual(h.clipboard, []);
    assert.deepStrictEqual(h.pendingTimers(), [500]);
    h.runTimers();
    await h.settle(1);
    assert.deepStrictEqual(h.clipboard, [ALPHA.url]);
});

// ---------------------------------------------------------------------
// The values of a bookmark are text
// ---------------------------------------------------------------------
//
// THE FAULT THAT THIS PART HOLDS. The page wrote the title, the address and
// the tags with innerHTML. The title of a bookmark is the title of a page
// of a different site. A title with markup thus ran a script on this page,
// with the session of the reader. A check in Chromium saw the script run.

const MARKUP = {
    date: '2026-06-01 10:00:00',
    url: 'https://m.example/?q=<s>1</s>',
    title: '<img src=x onerror="window.ran=1"> <b>bold</b> &amp;',
    tags: ['<i>tag</i>', '!<em>mark</em>'],
    notes: ['<u>note</u>'],
};

test('markup in a value of a bookmark shows as text and makes no element', () => {
    const h = bookmarkPage([MARKUP, { date: '2026-06-02 10:00:00', url: MARKUP.url, title: '' }]);
    for (const tag of ['img', 'b', 'i', 'u', 's', 'em']) {
        assert.strictEqual(h.$('#omn-preview').querySelectorAll(tag).length, 0,
            'a value of a bookmark made the element <' + tag + '>');
    }
    // The second bookmark has no title, thus its link shows the address.
    assert.deepStrictEqual(shown(h), [MARKUP.url, MARKUP.title]);
    const li = entry(h, MARKUP);
    assert.ok(li.querySelector('span').textContent.indexOf(MARKUP.url) >= 0);
    assert.strictEqual(li.querySelectorAll('span br').length, 1, 'the address is not on a line of its own');
    assert.ok(li.textContent.indexOf('"<u>note</u>"') >= 0);

    // A tag with markup is still a tag: the cloud shows it and filters by it.
    const cloud = h.$('#omn-bm-tags-cloud').querySelectorAll('button').map((b) => b.textContent);
    assert.deepStrictEqual(cloud, ['NoTag', 'Duplicates', '<em>mark</em>', '<i>tag</i>']);
    button(h, '#omn-bm-tags-cloud', '<i>tag</i>').click();
    assert.deepStrictEqual(shown(h), [MARKUP.title]);
    button(h, '#omn-bm-list', '<i>tag</i>').click();
    assert.deepStrictEqual(shown(h), [MARKUP.title]);
});

test('an address that would run a script gets no link', () => {
    const scripts = [
        'javascript:alert(1)',
        ' JaVaScRiPt:alert(1)',
        'java\tscript:alert(1)',
        '\u0001javascript:alert(1)',
        'data:text/html,<script>alert(1)</script>',
        'vbscript:msgbox(1)',
    ];
    const list = scripts.map(function (url, i) {
        return { date: '2026-07-0' + (i + 1) + ' 10:00:00', url: url, title: 'script ' + i };
    });
    const h = bookmarkPage(list);
    assert.strictEqual(shown(h).length, scripts.length, 'the page hides the bookmark, and the person cannot see it');
    for (const bm of list) {
        const li = entry(h, bm);
        assert.strictEqual(li.querySelector('a').hasAttribute('href'), false,
            'a press on the link runs ' + JSON.stringify(bm.url));
        assert.ok(li.textContent.indexOf(bm.url) >= 0, 'the item does not show its address');
    }
});

test('each other address keeps its link', () => {
    const addresses = [
        'https://a.example/page', 'http://a.example/', '/BookmarksHowTo.html', 'Welcome.html',
        'mailto:a@a.example', 'ftp://a.example/file', 'https://a.example/?next=javascript:x',
    ];
    const list = addresses.map(function (url, i) {
        return { date: '2026-08-0' + (i + 1) + ' 10:00:00', url: url, title: 'address ' + i };
    });
    const h = bookmarkPage(list);
    for (const bm of list) {
        assert.strictEqual(entry(h, bm).querySelector('a').getAttribute('href'), bm.url);
    }
});

// ---------------------------------------------------------------------
// The tags
// ---------------------------------------------------------------------

test('the count opens and closes the tag cloud', () => {
    const h = bookmarkPage(BOOKMARKS);
    const cloud = h.$('#omn-bm-tags-cloud');
    assert.deepStrictEqual(cloud.querySelectorAll('button').map((b) => b.textContent),
        ['NoTag', 'Duplicates', 'Go', 'Reference', 'Video']);
    assert.strictEqual(cloud.querySelectorAll('button.omn-tag-special').length, 2);
    h.press('#omn-bm-counter');
    assert.strictEqual(cloud.style.display, 'block');
    h.press('#omn-bm-counter');
    assert.strictEqual(cloud.style.display, 'none');
});

test('a tag of the cloud shows only its bookmarks', () => {
    const h = bookmarkPage(BOOKMARKS);
    h.$('#omn-bm-search-input').value = 'old pattern';
    button(h, '#omn-bm-tags-cloud', 'Go').click();
    assert.deepStrictEqual(shown(h), [VIDEO_AGAIN.url, 'Alpha']);
    assert.strictEqual(h.$('#omn-bm-counter').textContent, '2');
    assert.strictEqual(button(h, '#omn-bm-tags-cloud', 'Go').disabled, true, 'the cloud does not mark the chosen tag');
    assert.strictEqual(button(h, '#omn-bm-tags-cloud', 'Video').disabled, false);
    assert.strictEqual(h.$('#omn-bm-search-input').value, '', 'the box shows a pattern that the list does not use');
});

test('a tag of an item shows only its bookmarks', () => {
    const h = bookmarkPage(BOOKMARKS);
    button(h, '#omn-bm-list', 'Reference').click();
    assert.deepStrictEqual(shown(h), ['Alpha']);
});

test('NoTag shows the bookmarks with no tag, and All shows each one', () => {
    const h = bookmarkPage(BOOKMARKS);
    button(h, '#omn-bm-tags-cloud', 'NoTag').click();
    assert.deepStrictEqual(shown(h), [HEDGEHOG.title]);
    button(h, '#omn-bm-buttons', 'All').click();
    assert.deepStrictEqual(shown(h), ALL);
});

test('Duplicates shows the bookmarks of one address', () => {
    const h = bookmarkPage(BOOKMARKS);
    // The two shares of the video have a different "si" value, and the
    // page reads them as one address.
    button(h, '#omn-bm-tags-cloud', 'Duplicates').click();
    assert.deepStrictEqual(shown(h), ['Video', VIDEO_AGAIN.url]);
});

test('a list with no tag has no tag cloud', () => {
    const h = bookmarkPage([HEDGEHOG]);
    assert.strictEqual(h.$('#omn-bm-tags-cloud').querySelectorAll('button').length, 0);
    assert.strictEqual(h.$('#omn-bm-counter').textContent, '1');
});

// ---------------------------------------------------------------------
// The search
// ---------------------------------------------------------------------

test('the search reads the title, the address, the tags, the notes and the date', () => {
    const h = bookmarkPage(BOOKMARKS);
    assert.deepStrictEqual(search(h, 'alpha'), ['Alpha'], 'the title');
    assert.strictEqual(h.$('#omn-bm-counter').textContent, '1');
    assert.deepStrictEqual(search(h, 'e\\.example'), [HEDGEHOG.title], 'the address');
    assert.deepStrictEqual(search(h, 'reference'), ['Alpha'], 'a tag');
    assert.deepStrictEqual(search(h, 'second'), ['Alpha'], 'a note');
    assert.deepStrictEqual(search(h, '2026-03'), ['Video'], 'the date');
    assert.deepStrictEqual(search(h, 'absent'), []);
    assert.strictEqual(h.$('#omn-bm-counter').textContent, '0');
    // The pattern is a regular expression.
    assert.deepStrictEqual(search(h, '^Al.ha$'), ['Alpha']);
    assert.deepStrictEqual(search(h, ''), ALL);
});

test('Enter in the box is the same as the Search button', () => {
    const h = bookmarkPage(BOOKMARKS);
    h.$('#omn-bm-search-input').value = 'alpha';
    h.$('#omn-bm-search-input').dispatchEvent(new Event('keyup', { bubbles: true, keyCode: 65 }));
    assert.deepStrictEqual(shown(h), ALL, 'a letter key started the search');
    h.$('#omn-bm-search-input').dispatchEvent(new Event('keyup', { bubbles: true, keyCode: 13 }));
    assert.deepStrictEqual(shown(h), ['Alpha']);
});

test('the search ignores an accent and the case by default', () => {
    const h = bookmarkPage(BOOKMARKS);
    assert.deepStrictEqual(search(h, 'ежик'), [HEDGEHOG.title]);
    assert.deepStrictEqual(search(h, 'елка'), [HEDGEHOG.title]);
    assert.deepStrictEqual(search(h, 'CAFE'), [HEDGEHOG.title]);
});

test('the two options of the device change the search', () => {
    const exact = bookmarkPage(BOOKMARKS, { storage: { [KEY]: '{"ignoreCase":false}' } });
    assert.deepStrictEqual(search(exact, 'alpha'), []);
    assert.deepStrictEqual(search(exact, 'Alpha'), ['Alpha']);

    const accents = bookmarkPage(BOOKMARKS, { storage: { [KEY]: '{"stripAccents":false}' } });
    assert.deepStrictEqual(search(accents, 'cafe'), []);
    assert.deepStrictEqual(search(accents, 'café'), [HEDGEHOG.title]);
});

// ---------------------------------------------------------------------
// The address of the page
// ---------------------------------------------------------------------

test('the address can name a tag or a pattern, and the tag comes first', () => {
    const tag = bookmarkPage(BOOKMARKS, { search: '?tag=Go' });
    assert.deepStrictEqual(shown(tag), [VIDEO_AGAIN.url, 'Alpha']);
    assert.strictEqual(tag.$('#omn-bm-search-input').value, '');

    const pattern = bookmarkPage(BOOKMARKS, { search: '?search=alpha' });
    assert.deepStrictEqual(shown(pattern), ['Alpha']);
    assert.strictEqual(pattern.$('#omn-bm-search-input').value, 'alpha');

    const both = bookmarkPage(BOOKMARKS, { search: '?search=alpha&tag=Video' });
    assert.deepStrictEqual(shown(both), ['Video', VIDEO_AGAIN.url]);
    assert.strictEqual(both.$('#omn-bm-search-input').value, '');
});

test('the address can name a bookmark, and the page marks it', () => {
    const h = bookmarkPage(BOOKMARKS, { hash: '#2026-01-01-100000' });
    const li = entry(h, ALPHA);
    assert.strictEqual(li.classList.contains('omn-search-hit-current'), true);
    // A plain scroll puts the item at the top. The page asks for the center.
    assert.ok((li._scrolls || []).some(function (o) { return o && o.block === 'center'; }),
        'the page did not scroll the item to the center');
    assert.strictEqual(h.document.querySelectorAll('.omn-search-hit-current').length, 1);
});

test('a filter that hides the named bookmark goes away', () => {
    const h = bookmarkPage(BOOKMARKS, { search: '?tag=Video', hash: '#2026-01-01-100000' });
    assert.deepStrictEqual(shown(h), ALL, 'the page does not hold the bookmark that the link names');
    assert.strictEqual(entry(h, ALPHA).classList.contains('omn-search-hit-current'), true);
});

test('a name of no bookmark marks nothing', () => {
    const h = bookmarkPage(BOOKMARKS, { hash: '#1999-01-01-000000' });
    assert.strictEqual(h.document.querySelectorAll('.omn-search-hit-current').length, 0);
    assert.deepStrictEqual(shown(h), ALL);
});

// ---------------------------------------------------------------------
// The options
// ---------------------------------------------------------------------

test('the first visit stores the default options and shows no dialog', () => {
    const h = bookmarkPage(BOOKMARKS);
    const options = stored(h);
    assert.strictEqual(options.stripAccents, true);
    assert.strictEqual(options.ignoreCase, true);
    assert.strictEqual(options.configVersion, h.$('#omn-bm-version').textContent);
    assert.deepStrictEqual(Object.keys(h.storage), [KEY], 'the page uses a second key of localStorage');
    assert.deepStrictEqual(h.dialogs, []);
});

test('an option of the address goes to the store and stays there', () => {
    const query = '?config=' + encodeURIComponent('{"ignoreCase":false}');
    const h = bookmarkPage(BOOKMARKS, { search: query });
    assert.strictEqual(stored(h).ignoreCase, false);
    assert.strictEqual(stored(h).stripAccents, true);
    assert.strictEqual(alerts(h)[0], 'URI params: {"ignoreCase":false}');
    assert.match(alerts(h)[1], /^Configuration\n/);
    assert.match(alerts(h)[1], /configKey: OMNBookmarkerConfigG\n/);
    assert.deepStrictEqual(shown(h), ALL);
    assert.deepStrictEqual(search(h, 'alpha'), []);

    // The next visit has no option in the address.
    const next = bookmarkPage(BOOKMARKS, { storage: h.storage });
    assert.strictEqual(stored(next).ignoreCase, false);
    assert.deepStrictEqual(next.dialogs, []);
    assert.deepStrictEqual(search(next, 'alpha'), []);
});

test('"config=show" shows the options and changes none', () => {
    const h = bookmarkPage(BOOKMARKS, { search: '?config=show', storage: { [KEY]: '{"stripAccents":false}' } });
    assert.strictEqual(alerts(h).length, 2);
    assert.match(alerts(h)[1], /Effective: \{[^}]*"stripAccents": false/);
    assert.strictEqual(stored(h).stripAccents, false);
    assert.strictEqual(stored(h).ignoreCase, true);
});

test('an option that is not JSON gives a message and the defaults', () => {
    const h = bookmarkPage(BOOKMARKS, { search: '?config=%7Bbad' });
    assert.match(alerts(h)[1], /^URI Config parsing error: /);
    assert.strictEqual(stored(h).ignoreCase, true);
    assert.deepStrictEqual(shown(h), ALL);
});

test('a store that is not JSON gives a message, and the page repairs it', () => {
    const h = bookmarkPage(BOOKMARKS, { storage: { [KEY]: '{bad' } });
    assert.match(alerts(h)[0], /^Storage Config parsing error: /);
    assert.strictEqual(stored(h).ignoreCase, true);
    assert.deepStrictEqual(shown(h), ALL);

    const next = bookmarkPage(BOOKMARKS, { storage: h.storage });
    assert.deepStrictEqual(next.dialogs, []);
});
