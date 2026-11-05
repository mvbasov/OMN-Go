// --- The console of the page ---
//
// This file puts a hook on each console method and on the two error events
// of the window. It keeps each line, and the console panel of the page
// footer shows them. A line of a note script or of a user file can open the
// editor at its place.
//
// IT LOADS BEFORE EACH OTHER SCRIPT OF THE APPLICATION, except
// omn-go-compat.js. index.html names it in the head, before the body. The
// hooks thus exist when a script of the application or a plain script of a
// note runs. A line that a script writes before this file loads is lost.
//
// It reads no name of another script. Nothing at the top level touches
// document.body, because the body does not exist yet.
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
                consoleModal.className = 'omn-console-modal';

                const header = document.createElement('div');
                header.className = 'omn-console-header';
                header.innerHTML = '<span>JS Console Output</span><div class="omn-console-actions"><button id="omn-go-console-clear" class="omn-btn-console omn-btn-console-clear" title="Clear Console"><i class="material-icons omn-icon-sm">delete_sweep</i></button><button id="omn-go-console-close" class="omn-btn-console omn-btn-console-close" title="Close Console"><i class="material-icons omn-icon-sm">close</i></button></div>';

                logsContainer = document.createElement('div');
                logsContainer.className = 'omn-console-logs';

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
                        if (consoleBtn) consoleBtn.innerHTML = '<i class="material-icons omn-icon-xs">terminal</i><span>0</span>';
                        updateConsoleFooterDot();
                    };
                }

                consoleBtn = document.createElement('button');
                consoleBtn.id = 'omn-go-console-btn';
                consoleBtn.className = 'omn-btn-console-main';
                consoleBtn.innerHTML = '<i class="material-icons omn-icon-xs">terminal</i><span>0</span>';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };
                // Footer dot tap-to-open is wired in updateConsoleFooterDot,
                // because the footer (#omn-page-footer) is parsed after #omn-preview and so
                // may not exist yet when initConsoleUI first runs.
                updateConsoleFooterDot();

                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {
                    if (el.children.length > 0) return false;
                    const text = (el.textContent || '').toLowerCase();
                    const id = (el.id || '').toLowerCase();
                    const cls = (el.className || '').toLowerCase();
                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');
                });

                var target = document.querySelector('.omn-header-actions'); if (target) { target.appendChild(consoleBtn); } else if (document.body) { consoleBtn.classList.add('omn-btn-console-main-fixed'); document.body.appendChild(consoleBtn); }
            }

            // computeJump decides whether an uncaught error can be opened in
            // the editor, and how. Only same-origin editable sources qualify:
            //   - the current note itself. The reported line is a line in
            //     the COMPILED html, so we later map it back to the
            //     markdown by content (kind 'note').
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
            // For a note it fetches the served page, and it reads the exact
            // source line text at the error line. It then hands that text to
            // the editor, which locates the line by CONTENT. The markdown to
            // HTML line-number arithmetic is not necessary.
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
                } catch (e) { /* fall back to the plain editor address */ }
                window.location.href = url;
            }

            // The header console button is hidden while the header is folded
            // (the default). This footer dot is always visible, so it tells
            // the user that console messages exist without unfolding. It has
            // the color of the console button and lives in the page footer
            // (#omn-page-footer), added by the template.
            function updateConsoleFooterDot() {
                var fd = document.getElementById('omn-go-console-footer-dot');
                if (!fd) return;
                // Wire tap-to-open once the footer exists (it is parsed after
                // #omn-preview, so a note's parse-time log can run before it).
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
                consoleBtn.innerHTML = `<i class="material-icons omn-icon-xs">terminal</i><span>${logs.length}</span>`;
                updateConsoleFooterDot();

                if (logsContainer) {
                    const msg = document.createElement('div');
                    // The colors are tokens of omn-go-core.css.
                    msg.className = 'omn-console-line' +
                        (type === 'error' ? ' omn-console-line-error' : type === 'warn' ? ' omn-console-line-warn' : '');

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
            // Installed at <head> time, before the body parses. This catches
            // errors from EVERY note script, and a syntax error in a classic
            // inline <script> block counts. The browser reports that error
            // while it parses the body, long before DOMContentLoaded.
            window.addEventListener('error', function(e) {
                var where = e.filename ? ' at ' + e.filename + ':' + e.lineno + (e.colno ? ':' + e.colno : '') : '';
                var msg = 'Uncaught Error: ' + e.message + where;
                // Print to the real console and add a clickable entry to the
                // in-app console. The entry carries a jump when the error
                // points at an editable source on this page. We call
                // originalError and appendLog directly rather than the
                // wrapped console.error, so the jump metadata survives.
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
