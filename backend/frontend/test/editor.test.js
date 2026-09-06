// The editor expansions, against the note that documents them.
//
// A person types into expandEmmet and expandMarkdownAbbr every day, and
// NOTHING tested either one. H3 gave each an export tail in 26.09.27 and
// no test ever called them.
//
// THE CASES COME FROM backend/frontend/md/Editor.md. That note is what a
// person reads before they type, thus it is the contract.
//
// A test written from the code alone says "the code does what the code
// does". It passes while the manual tells a lie.
//
// Each case below quotes the note. A failure therefore reads in one of
// two ways. The editor broke, or the note is now wrong. Repair the one
// that moved.
//
// WHY THE EXPANSIONS MATTER MORE THAN THEIR SIZE. Tab is the key. An
// abbreviation that this code does not recognize gives a plain tab.
//
// A fault here is therefore silent. The person presses Tab, gets a tab,
// and nothing says why.

'use strict';

const { test } = require('node:test');
const assert = require('node:assert');
const { load } = require('./dom-stub.js');

const editor = load('omn-go-editor.js');

// ----------------------------------------------------------------------
// expandEmmet
// ----------------------------------------------------------------------

// The worked example of the note, word for word.
//
// "You can type a compact abbreviation and press Tab to get the full
// markup. For example, type ul>li*3 and press Tab. The editor produces:"
test('the ul>li*3 example of Editor.md', () => {
    assert.equal(editor.expandEmmet('ul>li*3'),
        '<ul>\n  <li></li>\n  <li></li>\n  <li></li>\n</ul>');
});

// The second worked example of the note.
//
// "In this worked example, the abbreviation div.card>h3{Title}+p{Body}
// expands to:"
test('the div.card example of Editor.md', () => {
    assert.equal(editor.expandEmmet('div.card>h3{Title}+p{Body}'),
        '<div class="card">\n  <h3>Title</h3>\n  <p>Body</p>\n</div>');
});

// "The children of a few container tags get an implied tag name. For
// example, ul>.item becomes <ul><li class="item">…, and table>tr>td also
// works."
test('a container tag gives its child an implied name', () => {
    const ul = editor.expandEmmet('ul>.item');
    assert.ok(ul.indexOf('<li class="item">') >= 0,
        'ul>.item gave ' + JSON.stringify(ul) + '. Editor.md promises an ' +
        'implied li.');

    const table = editor.expandEmmet('table>tr>td');
    assert.ok(table.indexOf('<tr>') >= 0 && table.indexOf('<td>') >= 0,
        'table>tr>td gave ' + JSON.stringify(table));
});

// The parts that "Emmet in a nutshell" lists, one case for each line.
test('each part of the nutshell list', () => {
    const cases = [
        // "#name — an id, .name — a class (repeatable): div#main.box.wide"
        { abbr: 'div#main.box.wide', holds: ['id="main"', 'class="box wide"'] },
        // "[attr=value] — attributes: a[href=# title=\"Go home\"]"
        { abbr: 'a[href=#]', holds: ['href="#"'] },
        // "{text} — text content: p{Hello}"
        { abbr: 'p{Hello}', holds: ['<p>Hello</p>'] },
        // "> child, + sibling, *N repeat, ( … ) group: nav>ul>li*2>a"
        { abbr: 'nav>ul>li*2>a', holds: ['<nav>', '<ul>', '<li>', '<a></a>'] },
    ];
    for (const c of cases) {
        const got = editor.expandEmmet(c.abbr);
        assert.ok(got !== null, c.abbr + ' answered null, and Editor.md lists it');
        for (const want of c.holds) {
            assert.ok(got.indexOf(want) >= 0,
                c.abbr + ' gave ' + JSON.stringify(got) + ' and it holds no ' +
                JSON.stringify(want));
        }
    }
});

// "$ — the editor replaces this with the item number inside a repeat
// ($$ adds leading zeros): li.item$*3"
test('the dollar sign becomes the item number', () => {
    const got = editor.expandEmmet('li.item$*3');
    for (const want of ['item1', 'item2', 'item3']) {
        assert.ok(got.indexOf(want) >= 0,
            'li.item$*3 gave ' + JSON.stringify(got) + ' and it holds no ' + want);
    }
    const padded = editor.expandEmmet('li.item$$*3');
    assert.ok(padded.indexOf('item01') >= 0,
        'li.item$$*3 gave ' + JSON.stringify(padded) + '. Editor.md says that ' +
        '$$ adds leading zeros.');
});

// "If the editor does not recognize an abbreviation, Tab inserts a tab
// character instead."
//
// The caller reads null for that, thus null is the contract and not an
// empty string.
test('an abbreviation that the editor does not know answers null', () => {
    for (const abbr of ['', '   ', '!!!', '-> arrow', '2 + 2 = 4']) {
        assert.equal(editor.expandEmmet(abbr), null,
            JSON.stringify(abbr) + ' expanded. Editor.md says that Tab then ' +
            'inserts a tab character.');
    }
});

// "It does not support the climb-up operator ^."
//
// The note says so out loud, thus a test holds it. A later version that
// adds the operator changes this test AND that line of the note.
test('the climb-up operator stays unsupported', () => {
    const got = editor.expandEmmet('div>p^span');
    if (got !== null) {
        assert.ok(got.indexOf('<span></span>') < 0 ||
            got.indexOf('</div>\n<span>') < 0,
            'div>p^span expanded as a real climb-up. Editor.md says that the ' +
            'editor does not support it. Repair the note or this test.');
    }
});

// "The editor also limits the repeat count, so a large number cannot lock
// the editor."
test('a large repeat count cannot run away', () => {
    const started = Date.now();
    const got = editor.expandEmmet('li*100000');
    const spent = Date.now() - started;
    assert.ok(spent < 2000,
        'li*100000 took ' + spent + ' ms. Editor.md promises a limit.');
    assert.ok(got !== null, 'li*100000 answered null, thus this case proves nothing');
    const rows = got.split('\n').length;
    // The clamp of parseAbbr is 1000 today. A number far above it means
    // the clamp went away, and a stray "*999999" then locks the editor.
    assert.ok(rows <= 1000,
        'li*100000 gave ' + rows + ' lines. Editor.md promises a limit, and ' +
        'parseAbbr clamps the multiplier.');
});

// A plain word is not an abbreviation while a person writes prose.
//
// This one holds the OUTCOME and not the guard. Two things give it: the
// early test of expandEmmet, and parseAbbr, which fails on a space. A
// probe that removed the early test alone left this case green.
//
// The outcome is what a person feels. Tab inside a paragraph must write
// a tab, and never markup.
test('a plain word is still a one-tag abbreviation, and prose is not', () => {
    assert.notEqual(editor.expandEmmet('note'), null,
        'a lone word is a valid one-tag abbreviation');
    assert.equal(editor.expandEmmet('the quick brown fox'), null,
        'a run of words expanded, thus Tab inside a paragraph writes markup');
});

// ----------------------------------------------------------------------
// parseAbbr
// ----------------------------------------------------------------------

// parseAbbr answers a tree, and expandEmmet answers null when it throws.
//
// A parser that throws on a shape a person types would make Tab dead for
// that line. The test drives the shapes of the note through it directly.
test('parseAbbr answers a tree for each documented shape', () => {
    for (const abbr of ['ul>li*3', 'div.card>h3{Title}+p{Body}',
        'nav>ul>li*2>a', 'div#main.box.wide', 'a[href=#]', 'li.item$*3']) {
        const forest = editor.parseAbbr(abbr);
        assert.ok(Array.isArray(forest) && forest.length > 0,
            'parseAbbr(' + JSON.stringify(abbr) + ') answered ' +
            JSON.stringify(forest));
    }
});

// A damaged abbreviation must not throw out of expandEmmet.
//
// expandEmmet wraps parseAbbr in a try, thus a throw becomes null and Tab
// writes a tab. This test holds the whole chain, because a throw that
// escapes leaves the key dead.
test('a damaged abbreviation never throws out of expandEmmet', () => {
    for (const abbr of ['div>', 'div>>p', 'p{unclosed', 'a[href=', 'li*',
        'div((', '*3', '>>>', 'p{}{}', '[]']) {
        let threw = null;
        try {
            editor.expandEmmet(abbr);
        } catch (e) {
            threw = e;
        }
        assert.equal(threw, null,
            'expandEmmet(' + JSON.stringify(abbr) + ') threw ' +
            (threw && threw.message) + '. Tab is then dead on that line.');
    }
});

// ----------------------------------------------------------------------
// expandMarkdownAbbr
// ----------------------------------------------------------------------

// "A line with --- and nothing else becomes three lines."
//
// The note prints the shape. The date and the time come from the device,
// thus the test reads the shape and not the stamp.
test('the divider of Editor.md', () => {
    const got = editor.expandMarkdownAbbr('---');
    assert.ok(got !== null, '--- did not expand');
    const text = typeof got === 'string' ? got : got.text;
    assert.ok(/^---\n##### \d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\n/.test(text),
        'the divider gave ' + JSON.stringify(text) + '. Editor.md promises ' +
        '--- then a level five heading with the date and the time.');
});

// "A line with !!! and nothing else becomes an empty table."
test('the table of Editor.md', () => {
    const got = editor.expandMarkdownAbbr('!!!');
    assert.ok(got !== null, '!!! did not expand');
    const text = typeof got === 'string' ? got : got.text;
    const rows = text.split('\n').filter((l) => l.trim().length > 0);
    assert.ok(rows.length >= 4,
        'the table gave ' + rows.length + ' rows. Editor.md prints four.');
    assert.ok(rows[1].indexOf('---') >= 0,
        'the second row is ' + JSON.stringify(rows[1]) + '. Editor.md puts ' +
        'the separator there.');
});

// An ordinary line is not an abbreviation.
test('an ordinary line does not expand', () => {
    for (const line of ['', 'a note', '--', '!!', '----']) {
        const got = editor.expandMarkdownAbbr(line);
        assert.ok(got === null || got === undefined || got === false ||
            (typeof got === 'string' && got === line),
            JSON.stringify(line) + ' expanded to ' + JSON.stringify(got));
    }
});
