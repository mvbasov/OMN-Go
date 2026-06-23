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
                document.querySelector('.toolbar').appendChild(consoleBtn);
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

        // Intercept Markdown links for standard browser-side redirects
        document.getElementById('preview').addEventListener('click', (e) => {
            let target = e.target.closest('a');
            if(target) {
                const href = target.getAttribute('href');
                if (href) {
                    if (href.startsWith('http')) {
                        e.preventDefault();
                        window.open(href, '_blank');
                    } else if (!href.startsWith('javascript:') && !href.startsWith('#')) {
                        e.preventDefault();
                        let cleanHref = href;
                        if (cleanHref.endsWith('.md')) {
                            cleanHref = cleanHref.substring(0, cleanHref.length - 3) + '.html';
                        } else if (!cleanHref.includes('.')) {
                            cleanHref = cleanHref + '.html';
                        }
                        window.location.href = cleanHref;
                    }
                }
            }
        });

        async function loadNoteIntoEditor() {
            const res = await fetch('/api/getnote?name=' + encodeURIComponent(currentNote));
            if (res.ok) {
                document.getElementById('editor').value = await res.text();
            }
        }

        let currentMode = 'view';
        async function toggleMode() {
            if (currentMode === 'view') {
                if (typeof USE_INTERNAL_ED !== 'undefined' && !USE_INTERNAL_ED) {
                    window.location.replace('/api/edit-external?name=' + encodeURIComponent(currentNote));
                    return;
                }
                
                await loadNoteIntoEditor();
                
                const editor = document.getElementById('editor');
                const preview = document.getElementById('preview');
                const btn = document.getElementById('toggleBtn');
                
                editor.style.display = 'block';
                preview.style.display = 'none';
                btn.innerHTML = '<i class="material-icons" title="Switch to View Mode">visibility</i>';
                document.getElementById('saveBtn').style.display = 'block';
                document.getElementById('metaToggleBtn').style.display = 'none';
                document.getElementById('metadataPanel').classList.add('hidden');
                currentMode = 'edit';
            } else {
                const editor = document.getElementById('editor');
                const preview = document.getElementById('preview');
                const btn = document.getElementById('toggleBtn');
                
                editor.style.display = 'none';
                preview.style.display = 'block';
                btn.innerHTML = '<i class="material-icons" title="Switch to Edit Mode">edit</i>';
                document.getElementById('saveBtn').style.display = 'none';
                document.getElementById('metaToggleBtn').style.display = 'block';
                currentMode = 'view';
            }
        }

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

        // Image Drag & Drop
        const editor = document.getElementById('editor');
        editor.addEventListener('dragover', e => e.preventDefault());
        editor.addEventListener('drop', async e => {
            e.preventDefault();
            if(e.dataTransfer.files.length > 0) {
                const fd = new FormData();
                fd.append('image', e.dataTransfer.files[0]);
                const res = await fetch('/api/upload', { method: 'POST', body: fd });
                if(res.ok) {
                    const text = await res.text();
                    const cursor = editor.selectionStart;
                    editor.value = editor.value.substring(0, cursor) + text + editor.value.substring(cursor);
                    editor.dispatchEvent(new Event('input'));
                }
            }
        });

        async function login() {
            const pwd = document.getElementById('pwdInput').value;
            const res = await fetch('/login', {
                method: 'POST',
                headers: {'Content-Type': 'application/x-www-form-urlencoded'},
                body: 'password=' + encodeURIComponent(pwd)
            });
            if(res.ok) {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
                checkRole();
            } else {
                alert('Invalid Password');
            }
        }

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

        async function createNewPage() {
            let title = prompt("Enter New Page Title:");
            if (!title) return;
            let camel = toCamelCase(title);
            let safeName = camel.replace(/[^a-zA-Z0-9-]/g, '-');
            let fileName = prompt("Confirm File Name:", safeName);
            if (!fileName) return;

            let src = typeof currentNote !== 'undefined' ? currentNote : 'Welcome';
            const fd = new URLSearchParams();
            fd.append('source', src);
            fd.append('target', fileName);
            fd.append('title', title);

            const res = await fetch('/api/newpage', { method: 'POST', body: fd });
            if (res.ok) {
                window.location.href = '/' + fileName + '.html?edit=true';
            } else {
                alert("Failed to create new page!");
            }
        }

        async function saveNote() {
            let content = document.getElementById('editor').value;
            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                alert('Note saved!');
                window.location.reload();
            } else {
                alert('Failed to save!');
            }
        }

        async function submitQuickNote() {
            const fd = new URLSearchParams();
            fd.append('note', document.getElementById('quickText').value);
            const res = await fetch('/api/quick', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('quickText').value = '';
                document.getElementById('quickPanel').classList.add('hidden');
                alert('Saved!');
                window.location.reload();
            }
        }

        async function submitBookmark() {
            const fd = new URLSearchParams();
            fd.append('url', document.getElementById('bmUrl').value);
            fd.append('title', document.getElementById('bmTitle').value);
            fd.append('tags', document.getElementById('bmTags').value);
            fd.append('notes', document.getElementById('bmNotes').value);
            const res = await fetch('/api/bookmark', { method: 'POST', body: fd });
            if(res.ok) {
                document.getElementById('bmPanel').classList.add('hidden');
                document.querySelectorAll('#bmPanel input, #bmPanel textarea').forEach(el => el.value = '');
                alert('Saved!');
                window.location.reload();
            }
        }

        async function checkSession() {
            // Unhide UI if role cookies exist
            if (document.cookie.includes('session_role=')) {
                document.getElementById('loginOverlay').style.display = 'none';
                document.getElementById('mainUI').style.display = 'flex';
                checkRole();
            } else {
                // Check if server is configured with public role or check backend
                const test = await fetch('/api/config');
                if (test.status === 401) {
                    document.getElementById('loginOverlay').style.display = 'flex';
                    document.getElementById('mainUI').style.display = 'none';
                } else {
                    document.getElementById('loginOverlay').style.display = 'none';
                    document.getElementById('mainUI').style.display = 'flex';
                }
            }
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
                    if (arrow) arrow.textContent = '−';
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
                arrow.textContent = header.classList.contains('hidden') ? '+' : '−';
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
