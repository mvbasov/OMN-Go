// --- The core of each page ---
//
// This file holds the parts that each other script of the application
// uses. They are the KaTeX call, the actions of the controls and the
// progress overlay. It also holds the controls of the page header and the
// work at the load of a page. Each part works on a page from disk too.
//
// THE ORDER OF THE SCRIPTS IN index.html:
//
//	omn-go-compat.js     The notice of a browser that is too old.
//	omn-go-console.js    The console of the page.
//	omn-go-core.js       This file.
//	omn-go-highlight.js  The marks of a search.
//	omn-go-nav.js        The links of a note and the slow-page overlay.
//	omn-go-share.js      Send, copy, page link and the metadata panel.
//	omn-go-api.js        Each call of the backend.
//
// A top-level name of one of these files is a global name, and a later file
// can read it. The load listener of this file runs after each file loaded,
// thus it can call omnApplyArrivalHighlight of omn-go-highlight.js.

// The one authority for the KaTeX auto-render configuration. One call
// site in this file reads it. See window.onload below.
//
// A SECOND call stood inside a MutationObserver. That observer watched
// #omn-preview for a change of the DOM, and the render of KaTeX itself makes
// such a change. It replaces the text "$...$" with the markup
// <span class="katex">...</span>.
//
// That change started the observer again. renderMathInElement then ran
// over content that held the fresh output of KaTeX, and it read already
// rendered math with the same delimiter pattern. That is what damaged
// the plain text nearby.
//
// The server sets the content of #omn-preview one time, and no code of this
// file changes it after that. The observer therefore had no work at all.
// It is REMOVED below and not repaired. A mechanism that feeds itself
// and has no work to do is a risk with no gain.
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
// The export is explicit, because the User Manual documents this name. A
// note script that writes new content to the page calls this function. The
// function then sets the math in that content. See the "Useful functions"
// section.
window.omnGoRenderMath = omnGoRenderMath;

// --- The actions of the controls ---
// A control names its work in a data-action attribute, and it has no
// inline onclick. OMN.action(name, fn) gives the name its function. ONE
// click listener on the document finds the control and calls fn(control,
// event). The control can give one value in data-arg.
//
// A file registers its actions when it loads, and the listener reads the
// table at the click. The order of the script elements thus does not
// matter. A click on a name that no file registered writes a console
// warning.
//
// The listener also stops the default work of a link, because a link with
// an action has href="#".
window.OMN = window.OMN || {};
(function () {
    const actions = {};
    // With a name alone, OMN.action answers the function of that action.
    window.OMN.action = function (name, fn) {
        if (arguments.length < 2) return actions[name];
        actions[name] = fn;
    };
    document.addEventListener('click', function (event) {
        const target = event.target;
        const el = target && target.closest ? target.closest('[data-action]') : null;
        if (!el) return;
        const name = el.getAttribute('data-action');
        const fn = actions[name];
        if (typeof fn !== 'function') {
            console.warn('OMN-Go: no action "' + name + '"');
            return;
        }
        if (el.tagName === 'A') event.preventDefault();
        fn(el, event);
    });
})();

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

    // ScriptRules.md names executeScripts as a helper of a note script, thus
    // the export stays.
    window.executeScripts = executeScripts;
    return { executeScripts };
})();

// --- Progress overlay ---
// One shared indicator that says that the server is busy. Three parts
// use it: the git sync of omn-go-api.js, the database backup page, and
// the slow-navigation guard further down this file. The style lives in
// omn-go-core.css, under the .omn-progress- names.
//
// This file is parsed in <head>, so the markup CANNOT be built here - there
// is no document.body yet (see the note below). build() therefore runs on
// first show(), and show() defers itself to DOMContentLoaded if it is called
// before the body exists.
//
// The overlay blocks nothing on purpose. It traps no click, and its
// close button hides the indicator WITHOUT stopping the work. go-git
// offers no safe way to abort in the middle.
//
// A person who meets a sync that hangs can therefore still reach the
// rest of the interface.
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
                  '<i class="material-icons omn-icon-sm">close</i>' +
                '</button>' +
              '</div>' +
              '<div class="omn-progress-track omn-indeterminate"><div class="omn-progress-fill"></div></div>' +
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
        // percent(null) -> indeterminate sweep. percent(0..100) -> real bar.
        percent: function(n) {
            if (!trackEl) return;
            if (n === null || n === undefined || isNaN(n)) {
                trackEl.classList.add('omn-indeterminate');
                fillEl.style.width = '';
                return;
            }
            n = Math.max(0, Math.min(100, Number(n)));
            trackEl.classList.remove('omn-indeterminate');
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

// --- Global Listeners & State ---
//
// This file is loaded synchronously in <head>, BEFORE the body (and any
// classic <script> embedded in a note) is parsed. That is deliberate, and
// it mirrors the functions.js of classic OMN. The helper globals must
// already exist when the classic script of a note runs during parsing. The
// console hooks and the uncaught-error handlers are in omn-go-console.js,
// which loads before this file. Nothing at the top level of this file may
// touch document.body or any element, because the body does not exist yet.
// DOM work belongs inside a DOMContentLoaded or load listener.
if (typeof currentNote === 'undefined') {
    currentNote = (window.location.pathname.split('/').pop() || 'Welcome').replace(/\.html$/, '').replace(/\.md$/, '');
}

        // applyPlatformUI shows the .omn-android-only controls. omn-go-core.css
        // hides them. The server sets IS_ANDROID through COND_SCRIPTS in
        // internal/render/pages.go, the same way as IS_MARKDOWN. The function
        // runs at load, and it needs no login and no session.
        //
        // Set "flex", and not "". An empty value removes the inline style,
        // and the cascade decides again. The CSS hides these controls with a
        // selector that wins inside .omn-header-actions, thus "" keeps the
        // button hidden on Android too. ".omn-header-actions a, .omn-header-actions
        // button" gives "flex" to each other control of that bar, thus the
        // button matches its neighbors.
        function applyPlatformUI() {
            if (typeof IS_ANDROID !== 'undefined' && IS_ANDROID) {
                document.querySelectorAll('.omn-android-only').forEach(el => {
                    el.style.display = 'flex';
                });
            }
        }

        // Standalone or offline mode. When the compiled page is opened
        // directly from disk (file://) there is no server. Hide the header
        // controls that work only against the backend: create, quick-note,
        // bookmark, sync, settings and edit, all marked .omn-server-only in
        // index.html. Home, the metadata toggle and Refresh stay, because
        // they work offline. Refresh falls back to a plain reload, see
        // refreshPage. This runs on load, and the header is collapsed by
        // default, thus nothing flashes.
        function applyOfflineUI() {
            if (window.location.protocol === 'file:') {
                document.querySelectorAll('.omn-server-only').forEach(function (el) {
                    el.style.display = 'none';
                });
            }
        }

        // Refresh. Online, ask the server to recompile the page with
        // ?refresh=1. Offline there is no server to recompile, thus reload
        // the file. The refresh-page action of the header button calls it.
        window.refreshPage = function () {
            if (window.location.protocol === 'file:') {
                window.location.reload();
            } else {
                window.location.href = window.location.pathname + '?refresh=1';
            }
        };

        // Asks the native shell (WebViewSetup.shouldOverrideUrlLoading, see
        // the omngo://edit precedent) to pin a home-screen shortcut to the
        // current note. The .omn-android-only button is the only way in, and
        // applyPlatformUI() reveals it inside the Android app only. There
        // is no equivalent on desktop. "name" is the on-disk page name, and
        // MainActivity needs it to reopen the right note. "title" is the
        // Title: header of the note, already exposed as the global `Title`
        // var, see index.html. The label uses "title" alone, thus a
        // shortcut reads "Grocery List" and not "note-42".
        //
        // Android 8 (API 26) and later has two ways to pin a shortcut. The
        // dialog asks which one, and "mode" gives the answer to
        // Shortcuts.createNoteShortcut. An older Android has one way only,
        // thus the page asks nothing there.
        window.OMN.action('add-shortcut', async function () {
            if (typeof currentNote === 'undefined' || !currentNote) return;
            var label = (typeof Title !== 'undefined' && Title) ? Title : currentNote;
            var url = 'omngo://shortcut?name=' + encodeURIComponent(currentNote) +
                '&title=' + encodeURIComponent(label);
            var android = /Android (\d+)/.exec(navigator.userAgent || '');
            if (android && Number(android[1]) >= 8 && typeof window.omnDialog === 'function') {
                var answer = await window.omnDialog({
                    kind: 'choice',
                    title: 'Add "' + label + '" to the Home screen',
                    text: 'Reliable always works and shows the OMN-Go icon. ' +
                        'Android opens the Home screen, and you confirm there.\n\n' +
                        'Quick tries to add the shortcut while you stay in OMN-Go. ' +
                        'It often shows a generic icon, and some launchers do nothing. ' +
                        'If the icon is wrong or no shortcut appears, use Reliable.',
                    buttons: [
                        { label: 'Reliable', value: 'reliable', default: true },
                        { label: 'Quick', value: 'quick' },
                        { label: 'Cancel', value: null, cancel: true },
                    ],
                });
                if (!answer.value) return;
                url += '&mode=' + answer.value;
            }
            window.location.href = url;
        });

// The title of the page opens and closes the header. The arrow beside the
// title shows the state.
window.OMN.action('toggle-header', function () {
    var header = document.getElementById('omn-hidable-header');
    var arrow = document.getElementById('omn-title-arrow');
    if (!header) return;
    if (header.classList.contains('omn-hidden')) {
        header.classList.remove('omn-hidden');
        if (arrow) arrow.textContent = '\u2212';
    } else {
        header.classList.add('omn-hidden');
        if (arrow) arrow.textContent = '+';
    }
});

// The other actions of this file. Each action of this file works on a page
// from disk too. See OMN.action at the top of this file. The actions that
// need the server are in omn-go-api.js.
//
// window.refreshPage keeps its name, because the bundled note AppApiTest
// calls it.
window.OMN.action('refresh-page', function () { window.refreshPage(); });
// replace-location opens the address of data-arg in place of this page. The
// Back button then does not return to this page.
window.OMN.action('replace-location', function (el) {
    window.location.replace(el.getAttribute('data-arg'));
});
window.OMN.action('edit-page', function () {
    window.location.href = window.location.pathname + '?edit=true';
});
// --- The panels ---
//
// Quick Note and Bookmark are panels of modals.html, with the class
// .omn-panel. A panel does not stop the page under it. Only one panel
// shows at a time. These three functions are the only code that shows or
// hides a panel. omn-go-api.js, omn-go-bookmark.js and MainActivity.java
// call them.
//
// The metadata panel of index.html has no class .omn-panel. It opens and
// closes with the same functions, but it does not hide the other panels.
//
// A page from disk has no panel, because the server adds modals.html when
// it sends a page. Thus each function accepts a missing panel.

// omnShowPanel shows a panel and hides each other .omn-panel. It answers
// false when the page has no such panel.
window.omnShowPanel = function (id) {
    var panel = document.getElementById(id);
    if (!panel) return false;
    if (panel.classList.contains('omn-panel')) {
        Array.prototype.forEach.call(document.querySelectorAll('.omn-panel'), function (p) {
            if (p !== panel) p.classList.add('omn-hidden');
        });
    }
    panel.classList.remove('omn-hidden');
    return true;
};
window.omnHidePanel = function (id) {
    var panel = document.getElementById(id);
    if (panel) panel.classList.add('omn-hidden');
};
// omnTogglePanel answers true when the panel is open after the call.
window.omnTogglePanel = function (id) {
    var panel = document.getElementById(id);
    if (!panel) return false;
    if (panel.classList.contains('omn-hidden')) return window.omnShowPanel(id);
    panel.classList.add('omn-hidden');
    return false;
};

// omnCloseTopLayer closes the top layer of the page and answers true. With
// no open layer it answers false. The Back key of Android (MainActivity.java)
// and Escape (omn-go-api.js) call it. The order is the order on the screen:
//
//   1. A dialog of omn-go-dialog.js (z-index 80).
//   2. The search overlay of omn-go-search.js (70).
//   3. A panel (40).
//
// Each of the two files loads on demand. Before it loads, its layer cannot
// be open, and the typeof test skips it. omnCloseDialog also answers true
// for a dialog that it does not close, for example the login dialog. The
// page under such a dialog must not change.
window.omnCloseTopLayer = function () {
    if (typeof window.omnCloseDialog === 'function' && window.omnCloseDialog() === true) return true;
    if (typeof window.omnSearchIsOpen === 'function' && window.omnSearchIsOpen()) {
        window.omnSearchClose();
        return true;
    }
    var open = Array.prototype.find.call(document.querySelectorAll('.omn-panel'), function (p) {
        return !p.classList.contains('omn-hidden');
    });
    if (!open) return false;
    open.classList.add('omn-hidden');
    return true;
};

// --- The notice ---
//
// A dialog is for a question or a fault. A result that needs no answer is a
// notice: one short line at the bottom of the screen. It goes away after
// OMN_NOTICE_MS, or at a press on it. A second notice replaces the first,
// thus the page never holds two. The text goes in with textContent.

const OMN_NOTICE_MS = 2500;
const OMN_NOTICE_KEY = 'omn-notice';
let omnNoticeTimer = null;

window.omnNotice = function (text) {
    if (!document.body) return;
    let el = document.querySelector('.omn-notice');
    if (!el) {
        el = document.createElement('div');
        el.className = 'omn-notice';
        el.setAttribute('role', 'status');
        el.setAttribute('aria-live', 'polite');
        el.addEventListener('click', function () { el.remove(); });
        document.body.appendChild(el);
    }
    el.textContent = String(text);
    clearTimeout(omnNoticeTimer);
    omnNoticeTimer = setTimeout(function () { el.remove(); }, OMN_NOTICE_MS);
};

// omnNoticeAfterLoad keeps a notice for the next page, for a result that
// loads the page again. The load listener below shows it. A WebView can
// refuse sessionStorage. The notice is then lost, and nothing fails.
window.omnNoticeAfterLoad = function (text) {
    try {
        window.sessionStorage.setItem(OMN_NOTICE_KEY, String(text));
    } catch (e) { /* no storage: no notice */ }
};

function omnShowKeptNotice() {
    let text = null;
    try {
        text = window.sessionStorage.getItem(OMN_NOTICE_KEY);
        window.sessionStorage.removeItem(OMN_NOTICE_KEY);
    } catch (e) { /* no storage: no notice */ }
    if (text) window.omnNotice(text);
}

// toggle-panel and hide-panel take the id of the panel in data-arg.
window.OMN.action('toggle-panel', function (el) { window.omnTogglePanel(el.getAttribute('data-arg')); });
window.OMN.action('hide-panel', function (el) { window.omnHidePanel(el.getAttribute('data-arg')); });

// addEventListener, and NOT "window.onload = ...". A classic OMN note
// often assigns window.onload itself, for example
// "window.onload=createTOC()". With a listener, this handler and the
// window.onload of the note both run.
window.addEventListener('load', () => {
            omnShowKeptNotice();
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
                window.omnShowPanel('omn-quick-panel');
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }
            if (window.hljs) {
                document.querySelectorAll('#omn-preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            if (typeof OMN_GO_KATEX !== 'undefined' && OMN_GO_KATEX && window.renderMathInElement) {
                omnGoRenderMath(document.getElementById('omn-preview') || document.body);
            }
            // AFTER hljs and KaTeX, and never before. highlightElement
            // rebuilds the innerHTML of each "pre code". That used to
            // delete every <mark> that a search put inside a fenced block.
            // See the note above omnApplyArrivalHighlight.
            omnApplyArrivalHighlight();
            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('omn-toggle-btn');
                if (tb) tb.style.display = 'none';
            }
            // Note: ?edit=true is now handled entirely on the server, which
            // serves the standalone editor page. A rendered view page never
            // carries that query, thus there is no in-page edit toggle to
            // fire. This listener is registered after the ?hl= one above,
            // thus it runs after it. The ?hl= handler can scroll to the word
            // that matched. The fragment must then not pull the page back to
            // the top of the section, because the highlight would go off
            // screen again.
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

window.addEventListener('pageshow', function(event) {
    if (event.persisted) {
        window.location.reload();
    }
});
