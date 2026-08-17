// --- OMN-Go Core Architecture ---
// These modules are strictly for offline viewing, Markdown rendering, and UI manipulation.

// Single source of truth for KaTeX auto-render config, used by the one
// call site in this file (see window.onload below). There used to be a
// SECOND call inside a MutationObserver watching #preview for any DOM
// change - including the very DOM changes KaTeX's own render produces
// (replacing "$...$" text with <span class="katex">...</span> markup).
// That mutation re-triggered the observer, which re-ran renderMathInElement
// over content that now included KaTeX's own freshly-injected output -
// re-scanning already-rendered math markup with the same delimiter regex,
// which is what corrupted unrelated nearby plain text. #preview's content
// is set once by the server and nothing in this file mutates it
// afterward, so the observer wasn't needed for anything - it's removed
// below rather than "fixed", since a feedback-prone mechanism with no job
// to do is just risk with no benefit.
function omnGoRenderMath(container) {
    if (typeof OMN_GO_KATEX === 'undefined' || !OMN_GO_KATEX || !window.renderMathInElement) return;
    renderMathInElement(container, {
        delimiters: [
            {left: '$$', right: '$$', display: true},
            {left: '$', right: '$', display: false},
            {left: '\\(', right: '\\)', display: false},
            {left: '\\[', right: '\\]', display: true}
        ],
        throwOnError: false
    });
}

const UI = (function() {
    function executeScripts(container) {
                const scripts = container.querySelectorAll('script');
                scripts.forEach(oldScript => {
                    const newScript = document.createElement('script');
                    Array.from(oldScript.attributes).forEach(attr => newScript.setAttribute(attr.name, attr.value));
                    newScript.async = false;
                    if (oldScript.innerHTML) newScript.appendChild(document.createTextNode(oldScript.innerHTML));
                    oldScript.parentNode.replaceChild(newScript, oldScript);
                });
            }

    // Export to global scope to preserve HTML onclick attributes
    window.executeScripts = executeScripts;
    return { executeScripts };
})();

// --- Progress overlay ---
// One shared "the server is busy" indicator, used by git sync
// (omn-go-sse.js), the database backup/restore page and the slow-navigation
// guard further down this file. Styling lives in omn-go-core.css
// (.omn-progress-*).
//
// This file is parsed in <head>, so the markup CANNOT be built here - there
// is no document.body yet (see the note below). build() therefore runs on
// first show(), and show() defers itself to DOMContentLoaded if it is called
// before the body exists.
//
// The overlay is intentionally non-blocking: it does not trap clicks, and
// its close button dismisses the indicator WITHOUT cancelling the work
// (go-git offers no safe mid-operation abort). A user who hits a hung sync
// can therefore still reach the rest of the UI.
window.OMNProgress = (function() {
    var el = null, titleEl = null, stageEl = null, detailEl = null,
        trackEl = null, fillEl = null;
    var pendingTitle = null;

    function build() {
        if (el || !document.body) return el;
        el = document.createElement('div');
        el.className = 'omn-progress-overlay';
        el.hidden = true;
        // Static markup only - every caller-supplied string below is written
        // with textContent, never innerHTML.
        el.innerHTML =
            '<div class="omn-progress-card" role="status" aria-live="polite">' +
              '<div class="omn-progress-head">' +
                '<span class="omn-progress-title"></span>' +
                '<button type="button" class="omn-progress-close" aria-label="Hide">' +
                  '<i class="material-icons icon-sm">close</i>' +
                '</button>' +
              '</div>' +
              '<div class="omn-progress-track indeterminate"><div class="omn-progress-fill"></div></div>' +
              '<div class="omn-progress-stage"></div>' +
              '<div class="omn-progress-detail"></div>' +
            '</div>';
        document.body.appendChild(el);
        titleEl  = el.querySelector('.omn-progress-title');
        stageEl  = el.querySelector('.omn-progress-stage');
        detailEl = el.querySelector('.omn-progress-detail');
        trackEl  = el.querySelector('.omn-progress-track');
        fillEl   = el.querySelector('.omn-progress-fill');
        el.querySelector('.omn-progress-close').addEventListener('click', api.hide);
        return el;
    }

    var api = {
        show: function(title) {
            if (!document.body) {
                // Called from a <head> script before the body is parsed.
                pendingTitle = title;
                document.addEventListener('DOMContentLoaded', function() {
                    if (pendingTitle !== null) api.show(pendingTitle);
                }, { once: true });
                return;
            }
            pendingTitle = null;
            build();
            titleEl.textContent = title || 'Working';
            stageEl.textContent = '';
            detailEl.textContent = '';
            api.percent(null);
            el.hidden = false;
        },
        stage: function(text) {
            if (stageEl) stageEl.textContent = text || '';
        },
        detail: function(text) {
            if (detailEl) detailEl.textContent = text || '';
        },
        // percent(null) -> indeterminate sweep; percent(0..100) -> real bar.
        percent: function(n) {
            if (!trackEl) return;
            if (n === null || n === undefined || isNaN(n)) {
                trackEl.classList.add('indeterminate');
                fillEl.style.width = '';
                return;
            }
            n = Math.max(0, Math.min(100, Number(n)));
            trackEl.classList.remove('indeterminate');
            fillEl.style.width = n + '%';
        },
        hide: function() {
            pendingTitle = null;
            if (el) el.hidden = true;
        },
        isVisible: function() {
            return !!el && !el.hidden;
        }
    };
    return api;
})();

// --- Search highlighting ---
//
// Marking query terms inside the rendered page. Two callers, and they are why
// this lives here rather than beside the search dialog:
//
//   - the dialog (omn-go-sse.js), when a page-scope result is chosen;
//   - arriving at a page with ?hl=<term> on the URL, which a search result
//     links to - and which has to work on any page, including one opened from
//     disk where the server half of the app never loads.
//
// Literal matching only, deliberately: a fuzzy or misspelled term does not
// appear in the text as typed, so there is nothing to wrap. In that case
// nothing is highlighted rather than something that is not what matched.

var OMN_HL_MIN = 2;   // 1 character marks half the page

// OMN_HL_SCROLLED records that arrival with ?hl= has scrolled the page to the
// word that matched. The fragment scroll near the end of this file reads it
// and does nothing, so that the coarser target does not cancel the exact one.
var OMN_HL_SCROLLED = false;

// omnClearHighlights puts the DOM back exactly as it was: each
// <mark> is replaced by its own text and the parent normalised, so
// repeated searches cannot leave the page progressively more nested.
function omnClearHighlights() {
    var preview = document.getElementById('preview');
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
    var preview = document.getElementById('preview');
    if (!preview || !terms || !terms.length) return null;

    var needles = terms
        .map(function (t) { return t.toLowerCase(); })
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
        var lower = value.toLowerCase();
        var pieces = null;
        var at = 0;

        while (at < value.length) {
            var bestAt = -1, bestLen = 0;
            for (var i = 0; i < needles.length; i++) {
                var idx = lower.indexOf(needles[i], at);
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

// omnMarkNear finds the highlighted occurrence belonging to one SOURCE line,
// so choosing the third row in the search panel goes to the third match in the
// page rather than back to the first.
//
// It cannot do this by counting, which is the obvious implementation and a
// wrong one. The panel's rows are source lines; the page is compiled HTML, and
// the two do not have the same occurrences in the same order:
//
//   - a note's <script> block is indexed but never rendered as text;
//   - a link's URL is text in the source and absent from the page;
//   - one rendered paragraph can be several source lines.
//
// Any of those makes "the Nth row is the Nth mark" wrong, and wrong by an
// amount that varies silently per note. So the line is located by its TEXT.
// Both sides are flattened the same way and the first mark at or after the
// line's position wins. When the line cannot be found the caller is told so,
// rather than handed a confident wrong answer.

// Markdown syntax that leaves no trace in the rendered page. Flattened to a
// space on BOTH sides, so a character that survives rendering (a literal
// parenthesis in prose, say) is treated identically in the needle and in the
// haystack and cannot cause a miss on its own.
var OMN_HL_SYNTAX = /[*_`~#\[\]()!>|\\\u2026]/;

// omnFlatten lowercases, drops that syntax, and collapses whitespace, while
// recording where every surviving character came from - the map is what turns
// a position in the flattened text back into a position among the marks.
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
        out += c.toLowerCase();
        map.push(i);
        lastSpace = false;
    }
    return { text: out, map: map };
}

// omnPreviewText concatenates the page's visible text and notes where each
// mark starts within it. Same skip rules as the highlighter, minus MARK: here
// the marks' own text is wanted, it is just not to be marked again.
function omnPreviewText() {
    var preview = document.getElementById('preview');
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

    // Shorten from the right on a miss: the tail of a line is the part most
    // likely to carry a link or an entity that rendered differently, and the
    // head is enough to place it.
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
        // The END of the mark, not its start. A snippet is a WINDOW on its
        // line, so it can begin part way through a word - and when that word
        // is the marked one, a test on the start alone steps over the mark
        // the snippet is about and answers with the next one.
        var m = page.marks[i];
        if (m.at + m.el.textContent.length > rawAt) return m.el;
    }
    return page.marks[page.marks.length - 1].el;
}

window.omnMarkNear = omnMarkNear;


// omnAnchorElement returns the element that the URL fragment names, or null
// when there is no fragment or no such element.
//
// The fragment comes back from location.hash percent-encoded when the id holds
// a character outside ASCII - a Cyrillic heading gives "#%D0%9A%D0%BE%D1%82" -
// and getElementById wants the decoded id. The raw form is tried as well,
// because an id may itself contain a percent sign.
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
// DOCUMENT_POSITION_FOLLOWING is true for a mark that comes after the anchor
// AND for a mark inside it, which is what a bookmark entry needs: the entry is
// one <li id="..."> and the hit is in it. A heading anchor gets the other case,
// because the section text is the heading's next sibling, not its child.
//
// It returns null when no mark is at or after the anchor. The caller must not
// read that as "go to the first mark in the page": the hits above belong to a
// section that the reader did not choose.
function omnMarkFrom(anchor) {
    var marks = document.querySelectorAll('#preview mark.omn-search-hit');
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
// A search result links to /Note.html?hl=fetch&hl=json. On load, mark those
// terms, scroll to the first, and strip the parameters from the address bar so
// the URL is clean to copy, bookmark or reload - the highlight has already
// been applied, and leaving the query on would re-apply it on every refresh.
//
// history.replaceState rather than a redirect: no navigation, no extra request,
// and the back button behaves as though the parameters were never there.
//
// Deliberately NOT the #:~:text= scroll-to-text fragment, which browsers
// implement inconsistently and the Android WebView largely does not.
//
// WHEN THIS RUNS MATTERS. It is called at the END of the load listener
// further down this file, after highlight.js and KaTeX have rewritten
// #preview - not from a load listener of its own, which is what it used to
// be and which put it FIRST.
//
// hljs.highlightElement replaces the innerHTML of every "#preview pre code"
// with its own tokenised markup, built from the block's text. A <mark> put
// inside a fenced block before that ran was therefore deleted a moment
// later: a hit in a ```code``` block was listed in the search panel, and
// then could not be found in the page. Prose and an inline `code` span were
// never touched by hljs, so they highlighted correctly - which made it look
// as though code blocks were simply not searched.
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
    //    mark that belongs to this line - by text, because the line number in
    //    the result indexes the markdown SOURCE and this page is compiled
    //    HTML (see the note above omnMarkNear).
    //
    // 2. The fragment says WHICH SECTION. It is the answer for a link that
    //    names a section instead of a line, and the fallback when the text
    //    cannot be found. The anchor alone is not enough for a line: a section
    //    runs to the next heading, and the line that matched can be a screen
    //    or more below it - the reader then gets a page with a highlight that
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

    // target is null when the anchor is good but no mark is at or after it:
    // the note matched on its title or a tag, or every hit is above the chosen
    // section. The anchor scroll stands in that case.
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

// --- Global Listeners & State ---
// This file is loaded synchronously in <head>, BEFORE the body (and any
// classic <script> embedded in a note) is parsed. That is deliberate and
// mirrors classic OMN's functions.js: everything defined here - the
// console interceptor, the uncaught-error handlers and the helper
// globals - must already exist when a note's classic script executes
// during parsing. Nothing at the top level of this file may touch
// document.body or any element: the body does not exist yet. DOM work
// belongs inside a DOMContentLoaded/load listener.
if (typeof currentNote === 'undefined') {
    currentNote = (window.location.pathname.split('/').pop() || 'Welcome').replace(/\.html$/, '').replace(/\.md$/, '');
}

// Try to load console interceptor as early as possible
(function() {
            const originalLog = console.log;
            const originalError = console.error;
            const originalWarn = console.warn;
            const originalInfo = console.info;
	    const originalDebug = console.debug;
            const originalTrace = console.trace;
            const originalTable = console.table;
            const originalDir = console.dir;
            const originalTime = console.time;
            const originalTimeEnd = console.timeEnd;

            let logs = [];
            let consoleBtn = null;
            let consoleModal = null;
            let logsContainer = null;

            function initConsoleUI() {
                if (consoleBtn) return;

                consoleModal = document.createElement('div');
                consoleModal.id = 'omn-go-console-modal';
                consoleModal.className = 'console-modal';

                const header = document.createElement('div');
                header.className = 'console-header';
                header.innerHTML = '<span>JS Console Output</span><div class="console-actions"><button id="omn-go-console-clear" class="btn-console btn-console-clear" title="Clear Console"><i class="material-icons icon-sm">delete_sweep</i></button><button id="omn-go-console-close" class="btn-console btn-console-close" title="Close Console"><i class="material-icons icon-sm">close</i></button></div>';

                logsContainer = document.createElement('div');
                logsContainer.className = 'console-logs';

                consoleModal.appendChild(header);
                consoleModal.appendChild(logsContainer);
                document.body.appendChild(consoleModal);

                document.getElementById('omn-go-console-close').onclick = () => {
                    consoleModal.style.display = 'none';
                };
                let clrBtn = document.getElementById('omn-go-console-clear');
                if (clrBtn) {
                    clrBtn.onclick = () => {
                        logs = [];
                        if (logsContainer) logsContainer.innerHTML = '';
                        if (consoleBtn) consoleBtn.innerHTML = '<i class="material-icons icon-xs">terminal</i><span>0</span>';
                        updateConsoleFooterDot();
                    };
                }

                consoleBtn = document.createElement('button');
                consoleBtn.id = 'omn-go-console-btn';
                consoleBtn.className = 'btn-console-main';
                consoleBtn.innerHTML = '<i class="material-icons icon-xs">terminal</i><span>0</span>';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };
                // Footer dot tap-to-open is wired in updateConsoleFooterDot,
                // because the footer (#status) is parsed after #preview and so
                // may not exist yet when initConsoleUI first runs.
                updateConsoleFooterDot();

                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {
                    if (el.children.length > 0) return false;
                    const text = (el.textContent || '').toLowerCase();
                    const id = (el.id || '').toLowerCase();
                    const cls = (el.className || '').toLowerCase();
                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');
                });

                //if (metadataEl && metadataEl.parentNode) {
                //    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);
                //} else {
                //    consoleBtn.classList.add('btn-console-main-fixed');
                //    document.body.appendChild(consoleBtn);
                //}
                var target = document.querySelector('.header-actions'); if (target) { target.appendChild(consoleBtn); } else if (document.body) { consoleBtn.classList.add('btn-console-main-fixed'); document.body.appendChild(consoleBtn); }
            }

            // computeJump decides whether an uncaught error can be opened in
            // the editor, and how. Only same-origin editable sources qualify:
            //   - the current note itself: the reported line is a line in the
            //     COMPILED html, so we later map it back to the markdown by
            //     content (kind 'note').
            //   - a served asset under /js /css /json (a verbatim file): its
            //     lines map 1:1, so we jump by number (kind 'asset').
            // Errors from OMN-Go's own bundled scripts, or cross-origin, get
            // no jump.
            function computeJump(filename, line) {
                if (!filename || !line) return null;
                try {
                    const u = new URL(filename, window.location.href);
                    if (u.origin !== window.location.origin) return null;
                    const path = u.pathname;
                    if (path === window.location.pathname) return { kind: 'note', path: path, line: line };
                    if (/^\/(js|css|json|user_json)\//.test(path)) {
                        // Skip OMN-Go's own bundled scripts and minified
                        // libraries - jumping to "edit" those from an error is
                        // never what the user wants.
                        if (/\.min\.(js|css)$/.test(path) || /\/omn-go-[^/]*\.js$/.test(path)) return null;
                        return { kind: 'asset', path: path, line: line };
                    }
                    return null;
                } catch (e) { return null; }
            }

            // jumpToEditor opens the editor positioned on the error's line.
            // For a note it fetches the served page, reads the exact source
            // line text at the error line, and hands it to the editor to
            // locate by CONTENT - avoiding the markdown<->HTML line-number
            // arithmetic entirely.
            async function jumpToEditor(jump) {
                if (jump.kind === 'asset') {
                    window.location.href = jump.path + '?edit=true&line=' + jump.line;
                    return;
                }
                let url = jump.path + '?edit=true';
                try {
                    const res = await fetch(jump.path, { cache: 'no-store' });
                    const lines = (await res.text()).split('\n');
                    const lineText = (lines[jump.line - 1] || '').trim();
                    if (lineText) url += '&find=' + encodeURIComponent(lineText.slice(0, 300));
                    else url += '&line=' + jump.line;
                } catch (e) { /* fall back to just opening the editor */ }
                window.location.href = url;
            }

            // The header console button is hidden while the header is folded
            // (the default). This footer dot is always visible, so it tells
            // the user that console messages exist without unfolding. It's the
            // same orange as the console button (#ff9800) and lives in the
            // page footer (#status), added by the template.
            function updateConsoleFooterDot() {
                var fd = document.getElementById('omn-go-console-footer-dot');
                if (!fd) return;
                // Wire tap-to-open once the footer exists (it is parsed after
                // #preview, so a note's parse-time log can run before it).
                // consoleModal exists once initConsoleUI has run.
                if (!fd._wired && consoleModal) {
                    fd._wired = true;
                    fd.onclick = function () { consoleModal.style.display = 'flex'; };
                }
                fd.style.display = logs.length ? 'inline-block' : 'none';
            }
            // After the body (hence the footer) is parsed, reflect any
            // messages captured during parsing.
            document.addEventListener('DOMContentLoaded', updateConsoleFooterDot);

            function appendLog(type, args, jump) {
                logs.push({type, args, jump});
                if (!document.body) {
                    window.addEventListener('DOMContentLoaded', () => appendLog(type, args, jump));
                    return;
                }
                if (!consoleBtn) initConsoleUI();
                consoleBtn.innerHTML = `<i class="material-icons icon-xs">terminal</i><span>${logs.length}</span>`;
                updateConsoleFooterDot();

                if (logsContainer) {
                    const msg = document.createElement('div');
                    msg.style.marginBottom = '4px';
                    msg.style.paddingBottom = '4px';
                    msg.style.borderBottom = '1px solid #333';
                    const color = type === 'error' ? '#ff5555' : type === 'warn' ? '#ffb86c' : '#f8f8f2';
                    msg.style.color = color;

                    const text = Array.from(args).map(a => {
                        try { return typeof a === 'object' ? JSON.stringify(a) : String(a); }
                        catch(e) { return String(a); }
                    }).join(' ');

                    msg.textContent = `[${type.toUpperCase()}] ${text}`;
                    if (jump) {
                        // Make the entry a tappable "open in editor" link.
                        msg.style.cursor = 'pointer';
                        msg.style.textDecoration = 'underline';
                        msg.title = 'Open in editor at this line';
                        msg.addEventListener('click', function () { jumpToEditor(jump); });
                    }
                    logsContainer.appendChild(msg);
                    logsContainer.scrollTop = logsContainer.scrollHeight;
                }
            }
	    // Wrapper function creator
            function wrapConsole(methodName, originalMethod, level) {
                console[methodName] = function(...args) {
                    // Call original first (or after, depending on your needs)
                    try {
                        // Use .apply with the array directly
                        originalMethod.apply(console, args);
                    } catch (e) {
                        // Fallback if native apply fails
                        originalMethod(...args);
                    }

                    // Capture
                    appendLog(level, args);
               };
            }   

            // Override all major methods
            wrapConsole('log', originalLog, 'log');
            wrapConsole('error', originalError, 'error');
            wrapConsole('warn', originalWarn, 'warn');
            wrapConsole('info', originalInfo, 'info');
            wrapConsole('debug', originalDebug, 'debug');
            wrapConsole('trace', originalTrace, 'trace');
            wrapConsole('table', originalTable, 'table');
            wrapConsole('dir', originalDir, 'dir');
            wrapConsole('time', originalTime, 'time');
            wrapConsole('timeEnd', originalTimeEnd, 'timeEnd');
            // Installed at <head> time, before the body parses, so this
            // catches errors from EVERY note script - including syntax
            // errors in classic inline <script> blocks, which the browser
            // reports while parsing the body (long before DOMContentLoaded).
            window.addEventListener('error', function(e) {
                var where = e.filename ? ' at ' + e.filename + ':' + e.lineno + (e.colno ? ':' + e.colno : '') : '';
                var msg = 'Uncaught Error: ' + e.message + where;
                // Print to the real console and add a clickable, jump-enabled
                // entry to the in-app console (when the error points at an
                // editable source on this page). We call originalError +
                // appendLog directly rather than the wrapped console.error so
                // the jump metadata survives.
                try { originalError.call(console, msg); } catch (_) { }
                appendLog('error', [msg], computeJump(e.filename, e.lineno));
                return;
            });
            // Async note code (fetch(), openDatabase() wrappers, ...) fails
            // via rejected promises, not the error event - capture those too.
            window.addEventListener('unhandledrejection', function(e) {
                var reason = e.reason;
                var msg = (reason && reason.stack) ? reason.stack : String(reason);
                console.error('Unhandled Promise Rejection: ' + msg);
            });
})();

        // Intercept Markdown links for standard browser-side redirects
        function setupPreviewLinkInterceptor() {
            var preview = document.getElementById('preview');
            if (!preview) return;
            preview.addEventListener('click', (e) => {
            let target = e.target.closest('a');
            if(target) {
                const href = target.getAttribute('href');
                if (href) {
                    // Pure anchors and javascript: actions - leave the
                    // browser's native handling completely alone.
                    if (href.startsWith('#') || href.startsWith('javascript:')) {
                        return;
                    }

                    // http(s):// and protocol-relative "//" links are
                    // external - open them in a new tab instead of
                    // navigating the app itself away.
                    if (/^https?:\/\//i.test(href) || href.startsWith('//')) {
                        e.preventDefault();
                        window.open(href, '_blank');
                        return;
                    }

                    // Any other URI scheme (tel:, mailto:, geo:, sms:,
                    // market:, intent://, whatsapp:, ...) isn't a page
                    // reference at all - leave it untouched so the
                    // browser/WebView's own link handling can launch the
                    // matching app. This used to fall through to the
                    // "internal page" rewrite below, which appended a
                    // bogus ".html" onto anything without a literal "."
                    // in it - turning e.g. "tel:5551234" into
                    // "tel:5551234.html" and breaking it outright.
                    if (/^[a-zA-Z][a-zA-Z0-9+.-]*:/.test(href)) {
                        return;
                    }

                    // Everything else is an internal page reference. The
                    // server already normalized this exact href when it
                    // rendered the page (rewriteInternalLink in
                    // markdown.go converts ".md" to ".html", appends
                    // ".html" to bare page names, and leaves any
                    // "?query"/"#fragment" suffix untouched) - there's
                    // nothing left to redo here. The old naive re-check
                    // below used to re-break already-correct hrefs, e.g.
                    // "Page?x=1" became "Page?x=1.html", and
                    // "Page.md#section" was left with a literal ".md"
                    // (which 404s) because it matched neither of its two
                    // branches. Just navigate to exactly what was
                    // rendered.
                    e.preventDefault();
                    window.location.href = href;
                }
            }
        });
        }
        document.addEventListener('DOMContentLoaded', setupPreviewLinkInterceptor);

        let currentMode = 'view';

        // Reveals .android-only controls (hidden by default in CSS - see
        // omn-go-core.css) when the page was served with IS_ANDROID set
        // (COND_SCRIPTS in templates.go, mirroring the IS_MARKDOWN
        // precedent). Runs unconditionally on load, not gated on
        // login/session state like checkRole(), since there's no
        // guest/admin distinction for "can this device pin a shortcut".
        //
        // An explicit "flex", not "" (which only drops the inline style and
        // hands the decision back to the cascade): the CSS now hides these
        // controls with a selector that wins inside .header-actions, so ""
        // would leave the button hidden on Android as well. "flex" is what
        // ".header-actions a, .header-actions button" gives every other
        // control in that bar, so the revealed button matches its neighbours.
        function applyPlatformUI() {
            if (typeof IS_ANDROID !== 'undefined' && IS_ANDROID) {
                document.querySelectorAll('.android-only').forEach(el => {
                    el.style.display = 'flex';
                });
            }
        }

        // Standalone/offline mode: when the compiled page is opened directly
        // from disk (file://) there is no server, so hide the header controls
        // that can only work against the backend (create / quick-note /
        // bookmark, sync, settings, edit - all marked .server-only in
        // index.html). Home, the metadata toggle and Refresh stay: they work
        // offline (Refresh falls back to a plain reload - see refreshPage).
        // Runs on load; the header is collapsed by default so nothing flashes.
        function applyOfflineUI() {
            if (window.location.protocol === 'file:') {
                document.querySelectorAll('.server-only').forEach(function (el) {
                    el.style.display = 'none';
                });
            }
        }

        // Refresh: online, ask the server to recompile the page via
        // ?refresh=1; offline there is no server to recompile, so just reload
        // the file. Wired to the header's refresh button (onclick).
        window.refreshPage = function () {
            if (window.location.protocol === 'file:') {
                window.location.reload();
            } else {
                window.location.href = window.location.pathname + '?refresh=1';
            }
        };

        // Null-safe toggle for the quick-note panel. #quickPanel is a
        // server-injected modal (see injectRuntimeVars), so it is absent on an
        // exported/offline page - guard against that instead of throwing.
        // Wired to the header's quick-note button (onclick).
        window.toggleQuickPanel = function () {
            var p = document.getElementById('quickPanel');
            if (p) p.classList.toggle('hidden');
        };

        // Copies the Quick Note text to the clipboard WITHOUT saving it, so a
        // captured snippet - typed, shared in from another Android app, or
        // pushed in by a barcode scan (see showQuickCapture in omn-go-sse.js) -
        // can be pasted somewhere else. Wired to the panel's Copy button, which
        // passes itself as btn so the label can report the outcome.
        //
        // It lives here rather than beside submitQuickNote in omn-go-sse.js
        // because it never talks to the backend: that file's no-server branch
        // replaces every handler with a printDebug stub, which is right for
        // /api/quick and wrong for a pure clipboard action.
        //
        // select() + execCommand('copy') on purpose, and NOT
        // navigator.clipboard. The async Clipboard API is the modern spelling
        // but it is unusable in the Android WebView this app ships as its main
        // UI: writeText() needs a clipboard-write permission grant, and
        // WebChromeClient denies permission requests by default (MainActivity
        // overrides only the JS dialog callbacks, not onPermissionRequest), so
        // the returned promise simply never delivers a copy. Verified
        // on-device. execCommand is formally deprecated, but it is synchronous,
        // needs no permission, and works in the WebView, plain-http LAN pages
        // (which are not a secure context, so navigator.clipboard is absent
        // there anyway) and desktop browsers alike - one path for all three.
        //
        // No scratch element is needed: the text already sits in a <textarea>,
        // which is exactly what select() wants.
        window.copyQuickNote = function (btn) {
            var q = document.getElementById('quickText');
            if (!q) return;

            // Restores the button's own label after a moment. The original is
            // stashed on first use so repeated clicks (which land while the
            // label still reads "Copied!") can't capture the feedback text as
            // the label to go back to.
            function feedback(msg) {
                if (!btn) return;
                if (typeof btn.dataset.omnLabel === 'undefined') {
                    btn.dataset.omnLabel = btn.textContent;
                }
                btn.textContent = msg;
                clearTimeout(btn._omnCopyTimer);
                btn._omnCopyTimer = setTimeout(function () {
                    btn.textContent = btn.dataset.omnLabel;
                }, 1200);
            }

            if (!q.value) {
                feedback('Empty');
                return;
            }

            var ok = false;
            q.focus();
            q.select();
            try {
                ok = document.execCommand('copy');
            } catch (e) {
                ok = false;
            }

            // Drop the selection once the copy has been taken. The panel stays
            // open afterwards, so leaving the whole note highlighted would both
            // look like it is still "in progress" and let the next keystroke
            // replace the entire text. Collapsing to the end keeps the caret
            // somewhere sensible for continued typing.
            try {
                q.setSelectionRange(q.value.length, q.value.length);
            } catch (e) { /* element does not support selection ranges */ }

            feedback(ok ? 'Copied!' : 'Copy failed');
        };

        // Asks the native shell (MainActivity.shouldOverrideUrlLoading, see
        // the omngo://edit precedent) to pin a home-screen shortcut to the
        // current note. Only reachable via the .android-only button, which
        // applyPlatformUI() only reveals when running inside the Android
        // app - there is no equivalent on desktop. "name" (the on-disk page
        // name) is what MainActivity needs to reopen the right note; "title"
        // (the note's Title: header, already exposed as the global `Title`
        // var - see index.html) is only for the shortcut's on-screen label,
        // so a shortcut reads e.g. "Grocery List" instead of "note-42".
        window.createNoteShortcut = function() {
            if (typeof currentNote === 'undefined' || !currentNote) return;
            var label = (typeof Title !== 'undefined' && Title) ? Title : currentNote;
            window.location.href = 'omngo://shortcut?name=' + encodeURIComponent(currentNote) +
                '&title=' + encodeURIComponent(label);
        };

        window.toggleHeader = function() {
    var header = document.getElementById('hidable_header');
    var arrow = document.getElementById('title_arrow');
    if (header) {
        if (header.classList.contains('hidden')) {
            header.classList.remove('hidden');
            if (arrow) arrow.textContent = '\u2212';
        } else {
            header.classList.add('hidden');
            if (arrow) arrow.textContent = '+';
        }
    }
};
window.updateArrow = function() {
    var header = document.getElementById('hidable_header');
    var arrow = document.getElementById('title_arrow');
    if (header && arrow) {
        arrow.textContent = header.classList.contains('hidden') ? '+' : '\u2212';
    }
};

// addEventListener, NOT "window.onload = ...": classic OMN notes routinely
// assign window.onload themselves (e.g. "window.onload=createTOC();").
// When this file used the assignment form it silently overwrote (or was
// overwritten by) the note's handler depending on load order; with a
// listener both this handler and any note-assigned window.onload run.
window.addEventListener('load', () => {
            checkSession();
            applyPlatformUI();
            applyOfflineUI();

            const params = new URLSearchParams(window.location.search);
            if (params.has('share_text') || params.has('share_subject')) {
                window.handleShare(params.get('share_text'), params.get('share_subject'));
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }
            // Cold start via the "OMN-Go Quick Note" app-drawer icon (the
            // QuickNoteAlias activity-alias - see MainActivity), which
            // always lands on Welcome.html?quicknote=1 (see the omission
            // check MainActivity.isQuickNoteAliasLaunch runs). A warm start
            // (app already running) instead pops the panel directly via
            // evaluateJavascript in MainActivity.onNewIntent - this only
            // covers the cold-start case.
            if (params.has('quicknote')) {
                const qp = document.getElementById('quickPanel');
                if (qp) qp.classList.remove('hidden');
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }
            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            if (typeof OMN_GO_KATEX !== 'undefined' && OMN_GO_KATEX && window.renderMathInElement) {
                omnGoRenderMath(document.getElementById('preview') || document.body);
            }
            // AFTER hljs and KaTeX, never before: highlightElement rebuilds
            // the innerHTML of each "pre code", which used to delete every
            // <mark> a search had just put inside a fenced block. See the
            // note above omnApplyArrivalHighlight.
            omnApplyArrivalHighlight();
            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            // Note: ?edit=true is now handled entirely server-side (it serves
            // the standalone editor page), so a rendered view page never
            // carries that query and there is no in-page edit toggle to fire.
            // This listener is registered after the ?hl= one above, so it runs
            // after it. When that handler scrolled to the word that matched,
            // the fragment must not pull the page back to the top of the
            // section and put the highlight off screen again.
            if (!OMN_HL_SCROLLED) {
                let el = omnAnchorElement();
                if (el) el.scrollIntoView();
            }
        });

document.addEventListener("DOMContentLoaded", () => {
            const footer = document.getElementById('omn-go-version-footer');
            let v = 'xx.xx.xx';
            try { if (APP_VERSION) v = APP_VERSION; } catch(e) {}
            if (footer) footer.innerText = 'OMN-Go v' + v;
        });

// --- Config page: menu navigation + unsaved-changes tracking ---
// The config form itself is untouched: each settings group is a
// show/hide .config-screen block inside the ONE <form>, so
// FormData(form) in saveConfig() (omn-go-sse.js) still collects every
// field no matter which screen is open. No-ops on pages without a
// #configForm.
document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById('configForm');
    if (!form) return;
    const panel = form.closest('.config-panel') || document;
    const menu = document.getElementById('configMenu');
    const screens = panel.querySelectorAll('.config-screen');

    // -- Navigation --
    // Driven by the URL hash rather than plain click handlers so that
    // Android's hardware Back button works: MainActivity.onBackPressed
    // forwards Back to webView.goBack() whenever there is history, and
    // each hash change is a history entry. Back therefore walks
    // sub-screen -> menu -> whatever page preceded Config, matching what
    // a native settings screen does. Desktop browser Back behaves the
    // same way for free.
    const HASH_PREFIX = 'cfg-';

    function currentScreen() {
        const h = (window.location.hash || '').replace(/^#/, '');
        return h.indexOf(HASH_PREFIX) === 0 ? h.slice(HASH_PREFIX.length) : '';
    }

    function applyHash() {
        const want = currentScreen();
        let matched = false;
        screens.forEach(s => {
            // The menu block has no data-screen attribute; it is the
            // fallback shown when the hash names no known sub-screen.
            const name = s.getAttribute('data-screen');
            const active = !!name && name === want;
            s.classList.toggle('active', active);
            if (active) matched = true;
        });
        if (menu) menu.classList.toggle('active', !matched);
        // A sub-screen replaces the menu at the top of the panel, so start
        // it at the top instead of inheriting the menu's scroll offset.
        window.scrollTo(0, 0);
    }

    panel.querySelectorAll('[data-goto]').forEach(btn => {
        btn.addEventListener('click', () => {
            window.location.hash = HASH_PREFIX + btn.getAttribute('data-goto');
        });
    });
    panel.querySelectorAll('[data-back]').forEach(btn => {
        // history.back() rather than clearing the hash, so returning to the
        // menu consumes the history entry instead of adding another one -
        // otherwise Back would bounce between menu and sub-screen.
        btn.addEventListener('click', () => window.history.back());
    });

    window.addEventListener('hashchange', applyHash);
    applyHash();

    // -- Unsaved-changes tracking --
    // Mirrors the dirty/clean dot in omn-go-editor.js. Any input/change
    // anywhere in the form marks dirty; beforeunload then covers every way
    // of leaving, since this is a real full-page load and not an SPA
    // (following a link, browser Back out of the page, closing the tab).
    // Moving between sub-screens only changes the hash, so it never
    // triggers the prompt. saveConfig() calls window.configMarkClean()
    // before its reload paths so a successful save doesn't prompt.
    const dots = panel.querySelectorAll('.config-dirty-dot');
    const labels = panel.querySelectorAll('.config-dirty-indicator .config-dirty-label');
    const menuBanner = document.getElementById('configMenuDirty');
    let dirty = false;

    function markDirty() {
        if (dirty) return;
        dirty = true;
        dots.forEach(d => d.classList.add('dirty'));
        labels.forEach(l => { l.textContent = 'Unsaved changes'; });
        if (menuBanner) menuBanner.hidden = false;
    }
    window.configMarkClean = function() {
        dirty = false;
        dots.forEach(d => d.classList.remove('dirty'));
        labels.forEach(l => { l.textContent = ''; });
        if (menuBanner) menuBanner.hidden = true;
    };

    form.addEventListener('input', markDirty);
    form.addEventListener('change', markDirty);

    window.addEventListener('beforeunload', function(e) {
        if (dirty) { e.preventDefault(); e.returnValue = ''; }
    });
});

// --- Slow-navigation guard ---
// Some pages are generated by the server AT NAVIGATION TIME, so the wait
// happens after the browser has left this document and no in-page spinner
// can cover it: OMNGoTags rebuilds by scanning every note (tags.go), and any
// note whose .md is newer than its cached .html is recompiled on first view
// (serveHTMLPage) - which is every changed note after a pull.
//
// Nothing here can shorten that wait; what it can do is stop the UI looking
// dead while it happens. On a link click we arm a short timer and only show
// the overlay if the new document still has not taken over by then, so quick
// navigations never flash. The overlay dies with the document, so there is
// nothing to clean up on the way out.
//
// On Android the native ProgressBar in MainActivity covers the same gap
// (onPageStarted/onPageFinished) including back/forward and direct URL
// loads, which a click handler cannot see. This is the desktop counterpart.
document.addEventListener("DOMContentLoaded", () => {
    // Exported pages opened from disk have no server and no generated pages.
    if (window.location.protocol === 'file:') return;

    const DELAY_MS = 300;
    let timer = null;

    document.addEventListener('click', function(ev) {
        if (ev.defaultPrevented || ev.button !== 0) return;
        if (ev.metaKey || ev.ctrlKey || ev.shiftKey || ev.altKey) return;
        const a = ev.target && ev.target.closest ? ev.target.closest('a') : null;
        if (!a || !a.href) return;
        // New tab/window and downloads leave this document in place.
        if (a.target && a.target !== '_self') return;
        if (a.hasAttribute('download')) return;

        let url;
        try { url = new URL(a.href, window.location.href); } catch (e) { return; }
        // Skip cross-origin and non-http schemes (mailto:, intent:, ...).
        if (url.origin !== window.location.origin) return;
        if (url.protocol !== 'http:' && url.protocol !== 'https:') return;
        // A bare in-page anchor (including the Config sub-screen hashes)
        // changes no document and must not raise an overlay.
        if (url.pathname === window.location.pathname &&
            url.search === window.location.search) return;

        clearTimeout(timer);
        timer = setTimeout(function() {
            window.OMNProgress.show('Loading');
            window.OMNProgress.stage('Preparing page…');
            // decodeURIComponent can throw on a malformed %-sequence.
            let where = url.pathname;
            try { where = decodeURIComponent(url.pathname); } catch (e) {}
            window.OMNProgress.detail(where);
        }, DELAY_MS);
    }, true);

    // The results page's own form is a navigation too, and the wait behind it
    // is the largest one this guard covers: submitting the query reads every
    // note the index holds and renders the answer server-side. The click
    // handler above cannot see it, because a submit is not an <a>.
    //
    // NO debounce here, unlike the search dialog. A submit IS the "go" that
    // the dialog has to wait for a typist to mean. The DELAY_MS below is not
    // a delay before searching - the request is already on its way while it
    // runs - it is the same anti-flash arming the click handler uses, so a
    // fast answer arrives without an overlay having appeared at all.
    //
    // Matched by class rather than by "any GET form". A note may contain a
    // form of its own (raw HTML is allowed, see ScriptRules), and the word
    // "Searching" would be a lie over someone else's. This is the one form
    // the app ships that navigates; search_page.html and the .search-page-*
    // rules in omn-go-core.css already name it the same way.
    //
    // Bubble phase, not capture, so a handler that cancels the submit has
    // already run and set defaultPrevented.
    document.addEventListener('submit', function(ev) {
        if (ev.defaultPrevented) return;
        const form = ev.target;
        if (!form || !form.classList || !form.classList.contains('search-page-form')) return;

        clearTimeout(timer);
        timer = setTimeout(function() {
            window.OMNProgress.show('Searching');
            window.OMNProgress.stage('Reading your notes…');
        }, DELAY_MS);
    });

    // Same-document hash navigation (Config sub-screens) must never leave a
    // pending timer armed behind it.
    window.addEventListener('hashchange', () => clearTimeout(timer));
});

// --- Sending this note to someone ---
//
// The controls live on the metadata panel's "File:" line and not in the
// header actions, which is full. See claude/note-exchange-plan.md.
//
// Both fetch the SAME URL, /api/export/note, which answers the note's source
// with a "FileName:" line added to its header block - the only place the
// note's path survives a transport that delivers a flat file name. The
// stored note does not change; an export is a read.

// omnGoExportURL is the one address both controls use, and the one
// MainActivity fetches for the Android share sheet.
function omnGoExportURL(note) {
    return '/api/export/note?name=' + encodeURIComponent(note);
}

// omnGoSendNote hands the note to whatever can carry it.
//
// On Android that is the share sheet, which reaches Telegram, e-mail,
// LocalSend and everything else installed; MainActivity answers the
// omngo:// scheme, as it already does for omngo://edit and
// omngo://shortcut. Elsewhere there is no share sheet, so the browser
// downloads the file and the user attaches it to whatever they want -
// Content-Disposition on the endpoint is what makes it a download.
function omnGoSendNote(note) {
    if (typeof IS_ANDROID !== 'undefined' && IS_ANDROID) {
        window.location.href = 'omngo://share?name=' + encodeURIComponent(note);
        return;
    }
    window.location.href = omnGoExportURL(note);
}

// omnGoCopyText puts one string on the clipboard.
//
// navigator.clipboard needs a secure context. http://localhost counts as one,
// so the Clipboard API works on the device itself and inside the Android
// WebView. A LAN guest on http://192.168.x.x does not get it, and the older
// execCommand path takes the copy. This function throws when both ways fail.
// Each caller shows the failure in the status text of the metadata panel.
async function omnGoCopyText(text) {
    if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        return;
    }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.setAttribute('readonly', '');
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand('copy');
    document.body.removeChild(ta);
    if (!ok) throw new Error('the browser refused the copy');
}

// omnGoCopyNote puts the same Markdown on the clipboard, for pasting into a
// chat or a mail body.
async function omnGoCopyNote(note, say) {
    try {
        const res = await fetch(omnGoExportURL(note), { cache: 'no-store' });
        if (!res.ok) throw new Error('HTTP ' + res.status);
        await omnGoCopyText(await res.text());
        say('Copied');
    } catch (e) {
        say('Copy failed: ' + e.message);
    }
}

// --- A link to the page on screen ---

// omnGoPageTitle gives the text of that link. The "Title:" line of the note is
// the first choice, because that is the name the reader knows. Title and
// PageName are the fallbacks, in that order, for a view with no note behind
// it - the Config page, for example.
function omnGoPageTitle() {
    var out = '';
    document.querySelectorAll('meta[name]').forEach(function (m) {
        if (out) return;
        if (m.getAttribute('name').toLowerCase() === 'title') {
            out = (m.getAttribute('content') || '').trim();
        }
    });
    if (!out && typeof Title !== 'undefined' && Title) out = String(Title).trim();
    if (!out && typeof PageName !== 'undefined' && PageName) out = String(PageName).trim();
    return out || 'link';
}

// omnGoPageLink builds a Markdown link to the page on screen, for pasting into
// another note.
//
// The target is the address of this page WITHOUT the scheme and the host: the
// absolute path, the query string and the fragment, as the address bar holds
// them. A path keeps its meaning on each device that opens the same notes. A
// host does not: the Android application and the desktop application both
// serve the pages at 127.0.0.1, so a link that carries the host works on the
// one device that made it and nowhere else.
//
// The browser encodes the path, so a space is already %20. The two
// parentheses are the characters that the browser leaves alone and that
// Markdown reads as the end of a link, so this function encodes them. In the
// link text, a backslash and the two brackets get a backslash in front of
// them.
//
// rewriteInternalLink (backend/markdown.go) cuts the query and the fragment
// off before it looks at the extension, and it leaves a name that has an
// extension alone. A link of this shape thus reaches the reader as it was
// written.
function omnGoPageLink() {
    var loc = window.location;
    var target = (loc.pathname + loc.search + loc.hash)
        .replace(/\(/g, '%28').replace(/\)/g, '%29');
    var text = omnGoPageTitle().replace(/([\\\[\]])/g, '\\$1');
    return '[' + text + '](' + target + ')';
}

// omnGoCopyPageLink puts that link on the clipboard.
async function omnGoCopyPageLink(say) {
    try {
        await omnGoCopyText(omnGoPageLink());
        say('Link copied');
    } catch (e) {
        say('Copy failed: ' + e.message);
    }
}

// --- Dynamic Metadata Panel Extractor ---
//
// Built from ELEMENTS, not from a string of HTML.
//
// Every value here comes from the note's own meta tags, which come from its
// header block - so the old "metaHtml += `<strong>${name}</strong> ${content}`"
// let a note write markup into its own metadata panel. textContent cannot.
// The inline "color:#0056b3" and "border-bottom:#ccc" went the same way: they
// are theme tokens now, so the panel is legible on the dark theme, which is
// the fault the database backup dialog had before 26.08.29.
document.addEventListener("DOMContentLoaded", () => {
    const panel = document.getElementById('metadataPanel');
    if (!panel) return;

    const noteName = (typeof PageName !== 'undefined') ? PageName : '';

    // Also update the header name display
    var nameDisplay = document.getElementById('pageNameDisplay');
    if (nameDisplay && noteName) {
        nameDisplay.textContent = '/' + noteName;
    }
    // Populate header metadata line (Author, Date, Modified) from meta tags
    var hMeta = document.getElementById('headerMetadata');
    if (hMeta) {
        var parts = [];
        document.querySelectorAll('meta[name]').forEach(function(m) {
            var n = m.getAttribute('name').toLowerCase();
            if (n === 'author' || n === 'date' || n === 'modified') {
                parts.push(m.getAttribute('name') + ': ' + m.getAttribute('content'));
            }
        });
        if (parts.length) {
            hMeta.textContent = ' — ' + parts.join(' · ');
        }
    }

    panel.textContent = '';

    // The panel is one row: the metadata at the left, a column of controls at
    // the right. A control goes under the control before it, so a new control
    // makes the column longer. It does not make the "File:" line narrower,
    // which is what a third control in that line did.
    const layout = document.createElement('div');
    layout.className = 'metadata-layout';
    const body = document.createElement('div');
    body.className = 'metadata-body';
    const actions = document.createElement('div');
    actions.className = 'metadata-actions';

    const fileRow = document.createElement('div');
    fileRow.className = 'metadata-file-row';
    const fileLabel = document.createElement('span');
    fileLabel.className = 'metadata-file-name';
    fileLabel.textContent = 'File: ' + noteName;
    fileRow.appendChild(fileLabel);

    // A page opened from disk (file:) has no server to ask, and no address
    // that another note can point at. Such a page gets no controls.
    const online = window.location.protocol !== 'file:';

    // The two note controls need a note: IS_MARKDOWN is off for the Config
    // dashboard, the search page and the other views that borrow this page
    // shell, and those have no Markdown source to send. The link control has
    // no such condition, because each of those views has an address.
    const sendable = noteName &&
        (typeof IS_MARKDOWN !== 'undefined' && IS_MARKDOWN) && online;

    if (online) {
        const status = document.createElement('span');
        status.className = 'metadata-send-status';
        const say = function (msg) {
            status.textContent = msg || '';
            if (msg) setTimeout(function () { status.textContent = ''; }, 4000);
        };

        const button = function (icon, label, onClick) {
            const b = document.createElement('button');
            b.type = 'button';
            b.className = 'metadata-send';
            b.title = label;
            b.setAttribute('aria-label', label);
            const i = document.createElement('i');
            i.className = 'material-icons icon-sm';
            i.textContent = icon;
            b.appendChild(i);
            b.addEventListener('click', onClick);
            return b;
        };

        fileRow.appendChild(status);
        if (sendable) {
            actions.appendChild(button('share', 'Send this note',
                function () { omnGoSendNote(noteName); }));
            actions.appendChild(button('content_copy', 'Copy this note as text',
                function () { omnGoCopyNote(noteName, say); }));
        }
        actions.appendChild(button('link', 'Copy a link to this page',
            function () { omnGoCopyPageLink(say); }));
    }

    body.appendChild(fileRow);

    document.querySelectorAll('meta').forEach(m => {
        const name = m.getAttribute('name');
        const content = m.getAttribute('content');
        if (!name || !content || ['viewport', 'charset'].includes(name.toLowerCase())) return;
        const row = document.createElement('div');
        row.className = 'metadata-row';
        const key = document.createElement('strong');
        key.textContent = name.charAt(0).toUpperCase() + name.slice(1) + ':';
        row.appendChild(key);
        row.appendChild(document.createTextNode(' ' + content));
        body.appendChild(row);
    });

    layout.appendChild(body);
    if (actions.childNodes.length) layout.appendChild(actions);
    panel.appendChild(layout);
});

window.addEventListener('pageshow', function(event) {
    if (event.persisted) {
        window.location.reload();
    }
});