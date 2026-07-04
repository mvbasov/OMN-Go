// --- OMN-Go Core Architecture ---
// These modules are strictly for offline viewing, Markdown rendering, and UI manipulation.

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
                    };
                }

                consoleBtn = document.createElement('button');
                consoleBtn.id = 'omn-go-console-btn';
                consoleBtn.className = 'btn-console-main';
                consoleBtn.innerHTML = '<i class="material-icons icon-xs">terminal</i><span>0</span>';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };

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

            function appendLog(type, args) {
                logs.push({type, args});
                if (!document.body) {
                    window.addEventListener('DOMContentLoaded', () => appendLog(type, args));
                    return;
                }
                if (!consoleBtn) initConsoleUI();
                consoleBtn.innerHTML = `<i class="material-icons icon-xs">terminal</i><span>${logs.length}</span>`;

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
            window.addEventListener('error', function(e) {
                console.error('Uncaught Error:', e.message, 'at', e.filename, ':', e.lineno);
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

        // Global Drag & Drop for URLs (Bookmarks)
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
                document.getElementById('bmPanel').classList.remove('hidden');
            }
        });

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
                document.getElementById('bmPanel').classList.remove('hidden');
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

window.onload = () => {
            checkSession();

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
                renderMathInElement(document.getElementById('preview') || document.body, {
                    delimiters: [
                        {left: '$$', right: '$$', display: true},
                        {left: '$', right: '$', display: false},
                        {left: '\\(', right: '\\)', display: false},
                        {left: '\\[', right: '\\]', display: true}
                    ],
                    throwOnError: false
                });
            }
            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            if (window.location.search.includes('edit=true')) {
                setTimeout(() => {
                    if (typeof currentMode !== 'undefined' && currentMode === 'view' && typeof toggleMode === 'function') toggleMode();
                }, 100);
            }
            let hash = window.location.hash;
            if (hash) {
                let el = document.getElementById(hash.substring(1));
                if (el) el.scrollIntoView();
            }
        };

document.addEventListener("DOMContentLoaded", () => {
            // Setup Auto-Rendering for KaTeX via MutationObserver
            const previewNode = document.getElementById('preview') || document.body;
            let renderTimeout;
            const observer = new MutationObserver(() => {
                clearTimeout(renderTimeout);
                renderTimeout = setTimeout(() => {
                    if (typeof OMN_GO_KATEX !== 'undefined' && OMN_GO_KATEX && window.renderMathInElement) {
                        renderMathInElement(previewNode, {
                            delimiters: [
                                {left: '$$', right: '$$', display: true},
                                {left: '$', right: '$', display: false},
                                {left: '\(', right: '\)', display: false},
                                {left: '\[', right: '\]', display: true}
                            ],
                            throwOnError: false
                        });
                    }
                }, 50);
            });
            observer.observe(previewNode, { childList: true, subtree: true });
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

window.addEventListener('pageshow', function(event) {
    if (event.persisted) {
        window.location.reload();
    }
});