import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.47"', 'APP_VERSION = "1.0.48"'),
        ("frontend/index.html", 'APP_VERSION = "1.0.47"', 'APP_VERSION = "1.0.48"')
    ]

    for filepath, old_v, new_v in version_replacements:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_v in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old_v, new_v))
            # Catch scenario where 1.0.47 version bump was skipped or failed
            elif 'APP_VERSION = "1.0.46"' in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace('APP_VERSION = "1.0.46"', new_v))

    # 2. Synchronize Android APK Version in build.gradle
    gradle_path = "android/app/build.gradle" if os.path.exists("android/app/build.gradle") else "backend/android/app/build.gradle"
    
    if os.path.exists(gradle_path):
        with open(gradle_path, 'r', encoding='utf-8') as f:
            content = f.read()

        new_version_code = "10048"
        new_version_name = '"1.0.48"'
        
        content = re.sub(r'versionCode\s+\d+', f'versionCode {new_version_code}', content)
        content = re.sub(r'versionName\s+"[^"]+"', f'versionName {new_version_name}', content)

        with open(gradle_path, 'w', encoding='utf-8') as f:
            f.write(content)
        print(f"Successfully synchronized Android APK version in {gradle_path}")

    # 3. Inject JS Console Interceptor into HTML
    html_path = "frontend/index.html" if os.path.exists("frontend/index.html") else "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, 'r', encoding='utf-8') as f:
            html_content = f.read()
        
        if 'goomn-console-modal' not in html_content:
            console_inject = """
    <!-- JS Console Interceptor & UI -->
    <script>
        (function() {
            const originalLog = console.log;
            const originalError = console.error;
            const originalWarn = console.warn;
            const originalInfo = console.info;

            let logs = [];
            let consoleBtn = null;
            let consoleModal = null;
            let logsContainer = null;

            function initConsoleUI() {
                if (consoleBtn) return;

                // 1. Create Scrollable Modal
                consoleModal = document.createElement('div');
                consoleModal.id = 'goomn-console-modal';
                consoleModal.style.cssText = 'display:none; position:fixed; top:10%; left:10%; width:80%; height:80%; background:#1e1e1e; color:#00ff00; z-index:10000; border:2px solid #555; border-radius:8px; flex-direction:column; font-family:monospace; box-shadow: 0 4px 12px rgba(0,0,0,0.5);';

                const header = document.createElement('div');
                header.style.cssText = 'padding:10px; background:#333; color:#fff; display:flex; justify-content:space-between; align-items:center; border-bottom:1px solid #555; font-weight:bold;';
                header.innerHTML = '<span>JS Console Output</span><button id="goomn-console-close" style="background:#ff5555; color:white; border:none; border-radius:4px; padding:4px 12px; cursor:pointer;">Close</button>';

                logsContainer = document.createElement('div');
                logsContainer.style.cssText = 'flex:1; overflow-y:auto; padding:10px; white-space:pre-wrap; word-break:break-all; font-size:12px; line-height:1.4;';

                consoleModal.appendChild(header);
                consoleModal.appendChild(logsContainer);
                document.body.appendChild(consoleModal);

                document.getElementById('goomn-console-close').onclick = () => {
                    consoleModal.style.display = 'none';
                };

                // 2. Create Activation Button
                consoleBtn = document.createElement('button');
                consoleBtn.id = 'goomn-console-btn';
                consoleBtn.innerText = 'Console (0)';
                consoleBtn.style.cssText = 'margin-left:8px; padding:4px 8px; background:#ff9800; color:#fff; border:none; border-radius:4px; cursor:pointer; font-size:0.8rem; font-weight:bold;';
                consoleBtn.onclick = () => {
                    consoleModal.style.display = 'flex';
                };

                // 3. Intelligently locate the "metadata" element to snap next to it
                let metadataEl = Array.from(document.querySelectorAll('*')).find(el => {
                    if (el.children.length > 0) return false; // Focus on leaf nodes only
                    const text = (el.textContent || '').toLowerCase();
                    const id = (el.id || '').toLowerCase();
                    const cls = (el.className || '').toLowerCase();
                    return text.includes('metadata') || id.includes('metadata') || cls.includes('metadata');
                });

                if (metadataEl && metadataEl.parentNode) {
                    metadataEl.parentNode.insertBefore(consoleBtn, metadataEl.nextSibling);
                } else {
                    // Fallback: Drop it floating in the bottom-left if metadata isn't found
                    consoleBtn.style.position = 'fixed';
                    consoleBtn.style.bottom = '4px';
                    consoleBtn.style.left = '8px';
                    consoleBtn.style.zIndex = '9999';
                    document.body.appendChild(consoleBtn);
                }
            }

            function appendLog(type, args) {
                logs.push({type, args});
                
                // Ensure the DOM is ready before trying to append UI elements
                if (!document.body) {
                    window.addEventListener('DOMContentLoaded', () => appendLog(type, args));
                    return;
                }

                if (!consoleBtn) initConsoleUI();

                consoleBtn.innerText = `Console (${logs.length})`;

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

            // 4. Override Native Console Functions
            console.log = function(...args) {
                originalLog.apply(console, args);
                appendLog('log', args);
            };
            console.error = function(...args) {
                originalError.apply(console, args);
                appendLog('error', args);
            };
            console.warn = function(...args) {
                originalWarn.apply(console, args);
                appendLog('warn', args);
            };
            console.info = function(...args) {
                originalInfo.apply(console, args);
                appendLog('info', args);
            };
            
            // 5. Catch fatal uncaught errors automatically
            window.addEventListener('error', function(e) {
                console.error('Uncaught Error:', e.message, 'at', e.filename, ':', e.lineno);
            });
        })();
    </script>
</body>"""
            # Use regex to safely replace </body> regardless of case or whitespace
            html_content = re.sub(r'(?i)</body>', console_inject, html_content)
            
            with open(html_path, 'w', encoding='utf-8') as f:
                f.write(html_content)
            print(f"Successfully injected JS Console Interceptor into {html_path}")
        else:
            print(f"JS Console Interceptor already present in {html_path}")

    # 4. Output Standardized Git Commit Message
    commit_msg = """feat(frontend): intercept JS console output and display in UI
    
- Inject an IIFE to wrap native `console.log`, `error`, `warn`, and `info`.
- Dynamically generate a UI button labeled "Console" placed near the "Metadata" element (or floating fallback).
- Render a scrollable popup modal to trace output natively within the application.
- Synchronize Version to 1.0.48.

Version bumped to 1.0.48"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()