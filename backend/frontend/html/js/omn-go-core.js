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

        // Global Drag & Drop for URLs (Bookmarks). Registered on
        // DOMContentLoaded: this file now runs in <head>, where
        // document.body is still null - touching it directly here would
        // throw and kill the rest of this script.
        document.addEventListener('DOMContentLoaded', () => {
            document.body.addEventListener('dragover', e => {
                if (!e.target.closest('#editor')) e.preventDefault();
            });
            document.body.addEventListener('drop', e => {
                if (e.target.closest('#editor')) return;
                const url = e.dataTransfer.getData('text/uri-list') || e.dataTransfer.getData('text/plain');
                if (url && (url.startsWith('http://') || url.startsWith('https://'))) {
                    e.preventDefault();
                    document.getElementById('bmUrl').value = url;
                    document.getElementById('bmTitle').value = '';
                    const html = e.dataTransfer.getData('text/html');
                    if (html) {
                        const match = html.match(/<a[^>]*>(.*?)<\/a>/i);
                        if (match && match[1]) {
                            document.getElementById('bmTitle').value = match[1].replace(/<[^>]+>/g, '').trim();
                        }
                    }
                    window.showBookmarkPanel();
                }
            });
        });

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

        // Asks the native shell (MainActivity.shouldOverrideUrlLoading, see
        // the omngo://edit precedent) to pin a home-screen shortcut to the
        // current note. Only reachable via the .android-only button, which
        // applyPlatformUI() only reveals when running inside the Android
        // app - there is no equivalent on desktop.
        window.createNoteShortcut = function() {
            if (typeof currentNote === 'undefined' || !currentNote) return;
            window.location.href = 'omngo://shortcut?name=' + encodeURIComponent(currentNote);
        };

        function checkRole() {
            if(document.cookie.includes('session_role=guest')) {
                document.querySelectorAll('.admin-only').forEach(el => {
                    if(el.tagName === 'BUTTON' || el.tagName === 'TEXTAREA' || el.tagName === 'INPUT') el.disabled = true;
                    if(el.id === 'toggleBtn' || el.id === 'editor' || el.id === 'saveBtn') el.style.display = 'none';
                });
            }
        }

        function toCamelCase(str) {
            let words = str.split(/[-_\s]+/);
            return words.map(w => w ? w.charAt(0).toUpperCase() + w.slice(1) : '').join('');
        }

        window.handleShare = function(text, subject) {
            text = text || '';
            subject = subject || '';

            // Regex to find the first valid URL
            const urlMatch = text.match(/(https?:\/\/[^\s]+)/) || subject.match(/(https?:\/\/[^\s]+)/);

            if (urlMatch) {
                // URL Found -> Route to Bookmark Panel
                const url = urlMatch[0];
                document.getElementById('bmUrl').value = url;

                let title = subject;
                if (!title || title.includes(url)) {
                    title = text.replace(url, '').trim();
                }
                if (!title) title = "Shared Link";

                document.getElementById('bmTitle').value = title;
                window.showBookmarkPanel();
                document.getElementById('quickPanel').classList.add('hidden');
            } else {
                // No URL -> Route to Quick Note Panel
                let content = '';
                if (subject) content += subject + "\n\n";
                if (text) content += text;

                document.getElementById('quickText').value = content.trim();
                document.getElementById('quickPanel').classList.remove('hidden');
                document.getElementById('bmPanel').classList.add('hidden');
            }
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

            const params = new URLSearchParams(window.location.search);
            if (params.has('share_text') || params.has('share_subject')) {
                window.handleShare(params.get('share_text'), params.get('share_subject'));
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

// --- Bookmark "Tags" autocomplete ---
// Suggests existing tags while typing into the Ingest Bookmark modal's
// #bmTags field. Tags are typed comma-separated ("work, recipe, ita|" -
// the "|" marks the caret); suggestions are computed against only the
// fragment after the last comma, and are only shown once that fragment
// reaches #bmTags's minChars attribute (default 2, set in index.html).
// Picking a suggestion completes the fragment and appends ", " so the
// next tag can be typed right away.
//
// This is plain same-origin UI sugar, not a "server extension" - unlike
// the sync/login/etc. calls in omn-go-sse.js it doesn't need a protocol
// guard: a failed fetch (e.g. the page opened offline) is treated as "no
// suggestions" rather than an error, so the field still works as a plain
// comma-separated text input either way.
//
// Both the DOM wiring and the tag-list fetch are deliberately lazy: they
// only run the first time the Ingest Bookmark modal is actually opened,
// not on every page load (most page views never touch this panel).
// window.showBookmarkPanel()/toggleBookmarkPanel() below are the only
// places that reveal #bmPanel - the header's "add bookmark" button, the
// URL drag-and-drop handler, and window.handleShare all go through one of
// them now instead of poking #bmPanel's classList directly - so "the
// modal is opening" is caught in exactly one place.
(function () {
    var tagsCache = null;    // null until prepared; array once loaded (even if empty)
    var tagsPromise = null;  // in-flight fetch, if any
    var wired = false;       // #bmTags/#bmTagsSuggestions listeners attached only once

    // Fetches /json/bookmarker-tags.json at most once per page. Safe to
    // call every time the modal opens: if the list is already prepared
    // (tagsCache set) or already loading (tagsPromise set) this reuses
    // that instead of firing a second request.
    function ensureTagsLoaded() {
        if (tagsCache) return Promise.resolve(tagsCache);
        if (tagsPromise) return tagsPromise;
        tagsPromise = fetch('/json/bookmarker-tags.json', { cache: 'no-store' })
            .then(function (res) { return res.ok ? res.json() : []; })
            .then(function (data) { return (tagsCache = Array.isArray(data) ? data : []); })
            .catch(function () { return (tagsCache = []); });
        return tagsPromise;
    }

    // "foo, bar, ba" -> { done: ["foo", "bar"], fragment: "ba" }
    function splitTags(value) {
        var parts = value.split(',');
        var fragment = parts.pop();
        var done = parts.map(function (s) { return s.trim(); }).filter(Boolean);
        return { done: done, fragment: fragment.replace(/^\s+/, '') };
    }

    // Attaches the input/keydown/click listeners to #bmTags exactly once.
    // Called from showBookmarkPanel()/toggleBookmarkPanel() every time the
    // modal opens; the `wired` guard makes repeat calls no-ops so reopening
    // the panel never double-attaches listeners.
    function wireBookmarkTagAutocomplete() {
        if (wired) return;
        var input = document.getElementById('bmTags');
        var list = document.getElementById('bmTagsSuggestions');
        if (!input || !list) return;
        wired = true;

        var minChars = parseInt(input.getAttribute('minChars'), 10);
        if (!minChars || minChars < 1) minChars = 2;

        var activeIndex = -1;

        function hide() {
            list.innerHTML = '';
            list.classList.add('hidden');
            activeIndex = -1;
        }

        function setActive(idx) {
            var items = list.querySelectorAll('.tag-suggestion-item');
            items.forEach(function (it, i) { it.classList.toggle('active', i === idx); });
            activeIndex = idx;
        }

        function pick(tag) {
            var split = splitTags(input.value);
            var used = split.done.concat([tag]);
            // Rebuilding from scratch (rather than splicing) keeps this
            // correct even if the fragment was picked mid-string; the
            // trailing ", " primes the field for the next tag.
            input.value = used.join(', ') + ', ';
            hide();
            input.focus();
            var end = input.value.length;
            input.setSelectionRange(end, end);
        }

        function render(matches) {
            list.innerHTML = '';
            if (!matches.length) { hide(); return; }
            matches.forEach(function (tag) {
                var li = document.createElement('li');
                li.textContent = tag;
                li.className = 'tag-suggestion-item';
                // mousedown (not click) fires before #bmTags's blur, so
                // the pick survives the input losing focus.
                li.addEventListener('mousedown', function (e) {
                    e.preventDefault();
                    pick(tag);
                });
                list.appendChild(li);
            });
            activeIndex = -1;
            list.classList.remove('hidden');
        }

        function update() {
            var split = splitTags(input.value);
            var fragment = split.fragment;
            if (fragment.length < minChars) { hide(); return; }
            ensureTagsLoaded().then(function (tags) {
                // The field may have moved on while this fetch/cache
                // lookup was pending - drop a stale response.
                if (splitTags(input.value).fragment !== fragment) return;
                var lower = fragment.toLowerCase();
                var used = split.done.map(function (t) { return t.toLowerCase(); });
                var matches = tags.filter(function (t) {
                    return typeof t === 'string' &&
                        t.toLowerCase().indexOf(lower) === 0 &&
                        used.indexOf(t.toLowerCase()) === -1;
                }).slice(0, 20);
                render(matches);
            });
        }

        input.addEventListener('input', update);
        input.addEventListener('focus', update);

        input.addEventListener('keydown', function (e) {
            var items = list.querySelectorAll('.tag-suggestion-item');
            if (list.classList.contains('hidden') || !items.length) return;
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                setActive((activeIndex + 1) % items.length);
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                setActive((activeIndex - 1 + items.length) % items.length);
            } else if (e.key === 'Enter' && activeIndex >= 0) {
                e.preventDefault();
                pick(items[activeIndex].textContent);
            } else if (e.key === 'Escape') {
                hide();
            }
        });

        document.addEventListener('click', function (e) {
            if (e.target !== input && !list.contains(e.target)) hide();
        });
    }

    // Unconditionally shows #bmPanel (used by drag-and-drop and
    // handleShare, which only ever want it open, never toggled).
    window.showBookmarkPanel = function () {
        var panel = document.getElementById('bmPanel');
        if (!panel) return;
        panel.classList.remove('hidden');
        wireBookmarkTagAutocomplete();
        ensureTagsLoaded();
    };

    // Toggles #bmPanel (used by the header's "add bookmark" button, which
    // both opens and closes it). Only prepares the autocomplete on the
    // transition into "visible" - closing the panel does nothing extra.
    window.toggleBookmarkPanel = function () {
        var panel = document.getElementById('bmPanel');
        if (!panel) return;
        var opening = panel.classList.contains('hidden');
        panel.classList.toggle('hidden');
        if (opening) {
            wireBookmarkTagAutocomplete();
            ensureTagsLoaded();
        }
    };
})();

window.addEventListener('pageshow', function(event) {
    if (event.persisted) {
        window.location.reload();
    }
});