// --- The marks of a search in the page ---
//
// This file marks the words of a search in the rendered page, and it does
// the same on arrival with ?hl= in the address. It also holds the page half
// of the fold table.
//
// index.html loads it after omn-go-core.js. The load listener of
// omn-go-core.js calls omnApplyArrivalHighlight and omnAnchorElement, and it
// reads OMN_HL_SCROLLED. omn-go-search.js calls omnHighlightTerms and
// omnMarkNear. The User Manual promises omnHighlightTerms and
// omnClearHighlights to a note script.
//
// Marking query terms inside the rendered page. Two callers, and they are why
// this lives here rather than beside the search dialog:
//
//   - the dialog (omn-go-api.js), when a page-scope result is chosen,
//   - a page opened with ?hl=<term> on the URL, which a search result
//     links to. It has to work on any page, including one opened from
//     disk, where the server half of the application never loads.
//
// Literal matching only, deliberately: a fuzzy or misspelled term does not
// appear in the text as typed, so there is nothing to wrap. In that case
// nothing is highlighted rather than something that is not what matched.

var OMN_HL_MIN = 2;   // 1 character marks half the page

// OMN_FOLD_TABLE is a port of textmatch.FoldTable in
// backend/internal/textmatch/textmatch.go. The two must stay the same. TestFoldTableHasAFrontendCopy compares them.
//
// The server folds before it matches, thus a search for "elka" finds a note
// titled "Elka" with the Cyrillic yo. The panel marks that word, because the
// server sends the spans. This file marks the word again after the reader
// opens the page. A lowercase alone does not make the yo into an e, thus
// this file needs the table.
//
// Every entry maps ONE character to ONE character. The Go comment gives
// the reason. A fold that changes the length moves every span after it.
// The expanding folds are absent on purpose, and not by an oversight.
//
// The keys are escapes, because the rest of this file is ASCII. The comment
// after each row says what the row holds.
var OMN_FOLD_TABLE = {
    '\u00e0': 'a', '\u00e1': 'a', '\u00e2': 'a', '\u00e3': 'a',
    '\u00e4': 'a', '\u00e5': 'a',                  // a with a mark
    '\u00e8': 'e', '\u00e9': 'e', '\u00ea': 'e',
    '\u00eb': 'e',                                 // e with a mark
    '\u00ec': 'i', '\u00ed': 'i', '\u00ee': 'i',
    '\u00ef': 'i',                                 // i with a mark
    '\u00f2': 'o', '\u00f3': 'o', '\u00f4': 'o', '\u00f5': 'o',
    '\u00f6': 'o', '\u00f8': 'o',                  // o with a mark
    '\u00f9': 'u', '\u00fa': 'u', '\u00fb': 'u',
    '\u00fc': 'u',                                 // u with a mark
    '\u00fd': 'y', '\u00ff': 'y',                  // y with a mark
    '\u00f1': 'n',                                 // n with a tilde
    '\u00e7': 'c',                                 // c with a cedilla
    '\u0451': '\u0435'                             // Cyrillic yo to e
};

// omnFoldChar lowercases one character and then applies the table above.
// One character in, one character out.
//
// A lowercase of one character can give two, for example the Turkish dotted
// capital I. This function keeps the original in that case, because a longer
// result would move every span after it.
function omnFoldChar(c) {
    if (c < '\u0080') {
        return (c >= 'A' && c <= 'Z') ? c.toLowerCase() : c;
    }
    var low = c.toLowerCase();
    if (low.length !== 1) return c;
    return OMN_FOLD_TABLE[low] || low;
}

// omnFold folds a whole string. The result holds one character for each
// character of the input, thus an offset into it is an offset into the
// original.
function omnFold(s) {
    var out = '';
    for (var i = 0; i < s.length; i++) {
        out += omnFoldChar(s.charAt(i));
    }
    return out;
}

// OMN_HL_SCROLLED records that arrival with ?hl= has scrolled the page to the
// word that matched. The fragment scroll near the end of this file reads it
// and does nothing, so that the coarser target does not cancel the exact one.
var OMN_HL_SCROLLED = false;

// omnClearHighlights puts the DOM back exactly as it was. Each <mark>
// becomes its own text again, and the parent is normalized. A run of
// searches therefore leaves the page no deeper than it found it.
function omnClearHighlights() {
    var preview = document.getElementById('omn-preview');
    if (!preview) return;
    var marks = preview.querySelectorAll('mark.omn-search-hit');
    for (var i = 0; i < marks.length; i++) {
        var m = marks[i];
        var parent = m.parentNode;
        if (!parent) continue;
        parent.replaceChild(document.createTextNode(m.textContent), m);
        if (parent.normalize) parent.normalize();
    }
}

// omnHighlightTerms wraps literal occurrences of the query terms in the
// rendered page and returns the first one.
//
// Literal only, on purpose: a fuzzy or misspelled term does not appear
// in the text as typed, so there is nothing to wrap. In that case the
// panel has already shown WHICH lines matched, and this returns null
// rather than highlighting something that is not what matched.
function omnHighlightTerms(terms) {
    omnClearHighlights();
    var preview = document.getElementById('omn-preview');
    if (!preview || !terms || !terms.length) return null;

    var needles = terms
        .map(function (t) { return omnFold(t); })
        .filter(function (t) { return t.length >= OMN_HL_MIN; });
    if (!needles.length) return null;

    // Collect first, mutate after: rewriting text nodes while walking
    // the tree invalidates the walker.
    var walker = document.createTreeWalker(preview, NodeFilter.SHOW_TEXT, null);
    var nodes = [];
    var node;
    while ((node = walker.nextNode())) {
        if (!node.nodeValue || !node.nodeValue.trim()) continue;
        var p = node.parentNode, skip = false;
        while (p && p !== preview) {
            var tag = p.tagName ? p.tagName.toUpperCase() : '';
            // Never touch executable or already-marked content: a note
            // may carry inline <script>, and rewriting its text would
            // corrupt source the console/editor still shows.
            if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'MARK' || tag === 'TEXTAREA') {
                skip = true;
                break;
            }
            p = p.parentNode;
        }
        if (!skip) nodes.push(node);
    }

    var firstMark = null;
    nodes.forEach(function (textNode) {
        var value = textNode.nodeValue;
        // The fold keeps one character for each character, thus an offset
        // into folded is an offset into value.
        var folded = omnFold(value);
        var pieces = null;
        var at = 0;

        while (at < value.length) {
            var bestAt = -1, bestLen = 0;
            for (var i = 0; i < needles.length; i++) {
                var idx = folded.indexOf(needles[i], at);
                if (idx !== -1 && (bestAt === -1 || idx < bestAt)) {
                    bestAt = idx;
                    bestLen = needles[i].length;
                }
            }
            if (bestAt === -1) break;
            if (!pieces) pieces = document.createDocumentFragment();
            if (bestAt > at) {
                pieces.appendChild(document.createTextNode(value.slice(at, bestAt)));
            }
            var mark = document.createElement('mark');
            mark.className = 'omn-search-hit';
            mark.textContent = value.slice(bestAt, bestAt + bestLen);
            pieces.appendChild(mark);
            if (!firstMark) firstMark = mark;
            at = bestAt + bestLen;
        }

        if (pieces) {
            if (at < value.length) {
                pieces.appendChild(document.createTextNode(value.slice(at)));
            }
            textNode.parentNode.replaceChild(pieces, textNode);
        }
    });

    return firstMark;
}

window.omnClearHighlights = omnClearHighlights;
window.omnHighlightTerms = omnHighlightTerms;

// omnMarkNear finds the marked occurrence that belongs to one SOURCE
// line. A press on the third row of the search panel therefore goes to
// the third match in the page, and not back to the first.
//
// IT CANNOT COUNT. That is the obvious implementation and a wrong one.
// The rows of the panel are source lines, and the page is compiled HTML.
// The two do not hold the same occurrences in the same order.
//
//   - the <script> block of a note is indexed and never rendered as text.
//   - the URL of a link is text in the source and absent from the page.
//   - one rendered paragraph can be several source lines.
//
// Each of those makes "the Nth row is the Nth mark" wrong, by an amount
// that changes from note to note and says nothing.
//
// The line is therefore found by its TEXT. Both sides are flattened the
// same way, and the first mark at or after the position of the line
// wins. A line that cannot be found is reported to the caller, who then
// gets no confident wrong answer.

// The markdown syntax that leaves no trace in the rendered page.
//
// It flattens to a space on BOTH sides. A character that survives the
// render, for example a parenthesis in prose, therefore reads the same
// in the needle and in the haystack. It can cause no miss on its own.
var OMN_HL_SYNTAX = /[*_`~#\[\]()!>|\\\u2026]/;

// omnFlatten lowercases the text, drops that syntax and collapses the
// whitespace. It records where each surviving character came from.
//
// That map is what turns a position in the flattened text back into a
// position among the marks.
function omnFlatten(raw) {
    var out = '', map = [], lastSpace = true;
    for (var i = 0; i < raw.length; i++) {
        var c = raw.charAt(i);
        if (OMN_HL_SYNTAX.test(c) || /\s/.test(c)) {
            if (!lastSpace) {
                out += ' ';
                map.push(i);
                lastSpace = true;
            }
            continue;
        }
        out += omnFoldChar(c);
        map.push(i);
        lastSpace = false;
    }
    return { text: out, map: map };
}

// omnPreviewText joins the visible text of the page and records where
// each mark starts inside it.
//
// It skips the same elements as the highlighter, and it keeps MARK. The
// text of a mark belongs in the answer. It only must not be marked a
// second time.
function omnPreviewText() {
    var preview = document.getElementById('omn-preview');
    if (!preview) return null;

    var walker = document.createTreeWalker(preview, NodeFilter.SHOW_TEXT, null);
    var raw = '', marks = [], node;
    while ((node = walker.nextNode())) {
        var p = node.parentNode, skip = false, mark = null;
        while (p && p !== preview) {
            var tag = p.tagName ? p.tagName.toUpperCase() : '';
            if (tag === 'SCRIPT' || tag === 'STYLE' || tag === 'TEXTAREA') {
                skip = true;
                break;
            }
            if (!mark && tag === 'MARK' &&
                (' ' + (p.className || '') + ' ').indexOf(' omn-search-hit ') >= 0) {
                mark = p;
            }
            p = p.parentNode;
        }
        if (skip) continue;
        if (mark && (!marks.length || marks[marks.length - 1].el !== mark)) {
            marks.push({ el: mark, at: raw.length });
        }
        raw += node.nodeValue;
    }
    return { raw: raw, marks: marks };
}

function omnMarkNear(snippet) {
    var page = omnPreviewText();
    if (!page || !page.marks.length || !snippet) return null;

    var flat = omnFlatten(page.raw);
    var needle = omnFlatten(snippet).text.trim();
    if (needle.length < 8) return null;   // too short to identify a line

    // Shorten from the RIGHT on a miss. The tail of a line is the part
    // most likely to hold a link or an entity that rendered another way.
    // The head alone is enough to place the line.
    var at = flat.text.indexOf(needle);
    while (at < 0 && needle.length > 12) {
        var cut = needle.lastIndexOf(' ');
        if (cut < 8) break;
        needle = needle.slice(0, cut);
        at = flat.text.indexOf(needle);
    }
    if (at < 0) return null;

    var rawAt = flat.map[at];
    for (var i = 0; i < page.marks.length; i++) {
        // The END of the mark, and not its start.
        //
        // A snippet is a WINDOW on its line, thus it can begin part way
        // through a word. When that word is the marked one, a test on the
        // start alone steps over the mark that the snippet is about. It
        // answers with the next one.
        var m = page.marks[i];
        if (m.at + m.el.textContent.length > rawAt) return m.el;
    }
    return page.marks[page.marks.length - 1].el;
}

window.omnMarkNear = omnMarkNear;


// omnAnchorElement returns the element that the URL fragment names, or null
// when there is no fragment or no such element.
//
// location.hash answers the fragment percent-encoded when the id holds a
// character outside ASCII. A Cyrillic heading gives "#%D0%9A%D0%BE%D1%82",
// and getElementById wants the decoded id.
//
// The raw form is tried as well, because an id can itself hold a percent
// sign.
function omnAnchorElement() {
    var hash = window.location.hash;
    if (!hash || hash.length < 2) return null;
    var raw = hash.slice(1), id = raw;
    try {
        id = decodeURIComponent(raw);
    } catch (e) { /* not valid escaping: use the fragment as written */ }
    return document.getElementById(id) || document.getElementById(raw);
}

window.omnAnchorElement = omnAnchorElement;

// omnMarkFrom returns the first highlighted word at or after an anchor.
//
// DOCUMENT_POSITION_FOLLOWING is true for a mark that comes after the
// anchor, AND for a mark inside it. A bookmark entry needs the second
// case, because the entry is one <li id="..."> and the hit sits in it.
//
// A heading anchor gets the first case. The text of a section is the
// next sibling of the heading, and not its child.
//
// It answers null when no mark is at or after the anchor. The caller
// must NOT read that as "go to the first mark in the page". The hits
// above belong to a section that the reader did not choose.
function omnMarkFrom(anchor) {
    var marks = document.querySelectorAll('#omn-preview mark.omn-search-hit');
    for (var i = 0; i < marks.length; i++) {
        if (anchor.compareDocumentPosition(marks[i]) &
            Node.DOCUMENT_POSITION_FOLLOWING) {
            return marks[i];
        }
    }
    return null;
}


// --- ?hl= : highlight on arrival ---
//
// A search result links to /Note.html?hl=fetch&hl=json.
//
// At load this code marks those terms, scrolls to the first, and takes
// the parameters out of the address bar. The URL is then clean to copy,
// to bookmark and to reload. The marks are already there, and a query
// left in place would apply them again at each refresh.
//
// history.replaceState rather than a redirect: no navigation, no extra request,
// and the back button behaves as though the parameters were never there.
//
// Deliberately NOT the #:~:text= scroll-to-text fragment, which browsers
// implement inconsistently and the Android WebView largely does not.
//
// WHEN THIS RUNS MATTERS. The load listener further down this file calls
// it at the END, after highlight.js and KaTeX have rewritten #omn-preview.
//
// It had a load listener of its own before, and that put it FIRST.
//
// hljs.highlightElement replaces the innerHTML of every
// "#omn-preview pre code" with its own markup, built from the text of the
// block. A <mark> written inside a fenced block before that ran was
// therefore deleted a moment later.
//
// A hit in a ```code``` block was listed in the search panel and then
// could not be found in the page. hljs touches neither prose nor an
// inline `code` span, thus those two marked correctly. The whole fault
// read as though a code block was never searched at all.
//
// Running last also means the scroll is computed against the final layout,
// instead of one that math and syntax highlighting were still about to
// change.
function omnApplyArrivalHighlight() {
    var terms, wanted;
    try {
        var q = new URLSearchParams(window.location.search);
        terms = q.getAll('hl');
        wanted = q.get('hlt');   // the text of the line the reader chose
    } catch (e) {
        return; // no URLSearchParams, or an unparsable URL: nothing to do
    }
    if (!terms || !terms.length) return;

    var first = window.omnHighlightTerms(terms);

    // Which mark the page goes to. Three things can say, from exact to coarse.
    //
    // 1. ?hlt= is the text of the ONE line the reader clicked. A result lists
    //    each matching line separately, so "the first match in the note" is
    //    the wrong answer for every row but the first. omnMarkNear finds the
    //    mark that belongs to this line, and it finds it by text. The line
    //    number in the result indexes the markdown SOURCE, and this page is
    //    compiled HTML. See the note above omnMarkNear.
    //
    // 2. The fragment says WHICH SECTION. It is the answer for a link that
    //    names a section instead of a line, and the fallback when the text
    //    cannot be found. The anchor alone is not enough for a line. A section
    //    runs to the next heading, and the line that matched can be a screen
    //    or more below it. The reader then gets a page with a highlight that
    //    is not on it.
    //
    // 3. The first mark in the page. Used when there is no fragment, or when
    //    the fragment names no element (a section id that the renderer did not
    //    produce). A match somewhere is better than the top of the page.
    var anchor = omnAnchorElement();
    var target = null;

    if (wanted && window.omnMarkNear) {
        target = window.omnMarkNear(wanted);
        // The same text can occur more than once - two identical list items in
        // two entries, say. A copy found ABOVE the section that the link names
        // is the wrong one, so the section decides instead.
        if (target && anchor &&
            !(anchor.compareDocumentPosition(target) &
              Node.DOCUMENT_POSITION_FOLLOWING)) {
            target = null;
        }
    }
    if (!target) target = anchor ? omnMarkFrom(anchor) : first;

    // target is null when the anchor is good but no mark is at or after it.
    // The note matched on its title or a tag, or every hit is above the
    // chosen section. The anchor scroll stands in that case.
    if (target && target.scrollIntoView) {
        target.scrollIntoView({ block: 'center' });
        target.classList.add('omn-search-hit-current');
        OMN_HL_SCROLLED = true;
    }

    try {
        var url = new URL(window.location.href);
        url.searchParams.delete('hl');
        url.searchParams.delete('hlt');
        window.history.replaceState({}, document.title,
            url.pathname + url.search + url.hash);
    } catch (e) { /* leaving the parameters on is harmless */ }
}

// A test outside a browser reads this file with a Node require. The check
// of module keeps the export out of a page, where module does not exist.
// See backend/frontend/test/, which no build ships.
//
// omnFold and omnFoldChar are the page half of the fold table. The server
// folds before it matches, and the page folds again before it marks. A
// difference between the two makes the marks land on the wrong words, or
// on nothing. TestFoldTableHasAFrontendCopy compares the two TABLES, and
// fold.test.js runs this code against them.
if (typeof module !== 'undefined' && module.exports) {
    module.exports = {
        omnFold: omnFold,
        omnFoldChar: omnFoldChar,
        omnFlatten: omnFlatten,
    };
}
