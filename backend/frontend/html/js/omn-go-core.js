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
        function applyPlatformUI() {
            if (typeof IS_ANDROID !== 'undefined' && IS_ANDROID) {
                document.querySelectorAll('.android-only').forEach(el => {
                    el.style.display = '';
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
            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            // Note: ?edit=true is now handled entirely server-side (it serves
            // the standalone editor page), so a rendered view page never
            // carries that query and there is no in-page edit toggle to fire.
            let hash = window.location.hash;
            if (hash) {
                let el = document.getElementById(hash.substring(1));
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

    // Same-document hash navigation (Config sub-screens) must never leave a
    // pending timer armed behind it.
    window.addEventListener('hashchange', () => clearTimeout(timer));
});

// --- Dynamic Metadata Panel Extractor ---
document.addEventListener("DOMContentLoaded", () => {
    const panel = document.getElementById('metadataPanel');
    if (panel) {
        let metaHtml = `<div style="margin-bottom: 8px; color: #0056b3; font-weight: bold; border-bottom: 1px solid #ccc; padding-bottom: 4px;">File: ${typeof PageName !== 'undefined' ? PageName : ''}</div>`;
        // Also update the header name display
        var nameDisplay = document.getElementById('pageNameDisplay');
        if (nameDisplay && typeof PageName !== 'undefined') {
            nameDisplay.textContent = '/' + PageName;
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
                hMeta.innerHTML = ' — ' + parts.join(' · ');
            }
        }
        document.querySelectorAll('meta').forEach(m => {
            const name = m.getAttribute('name');
            const content = m.getAttribute('content');
            if (name && content && !['viewport', 'charset'].includes(name.toLowerCase())) {
                metaHtml += `<div style="margin-bottom: 4px;"><strong>${name.charAt(0).toUpperCase() + name.slice(1)}:</strong> ${content}</div>`;
            }
        });
        panel.innerHTML = metaHtml;
    }
});

window.addEventListener('pageshow', function(event) {
    if (event.persisted) {
        window.location.reload();
    }
});