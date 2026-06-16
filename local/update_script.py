import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.21 Smart Share Router Update...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.20"', 'APP_VERSION = "1.2.21"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.20";', 'const APP_VERSION = "1.2.21";'),
        ("backend/frontend/index.html", "let v = '1.2.20';", "let v = '1.2.21';"),
        ("android/app/build.gradle", "versionCode 10220", "versionCode 10221"),
        ("android/app/build.gradle", 'versionName "1.2.20"', 'versionName "1.2.21"')
    ]

    for filepath, old_val, new_val in version_replacements:
        if os.path.exists(filepath):
            with open(filepath, "r", encoding="utf-8") as f:
                content = f.read()
            if old_val in content:
                content = content.replace(old_val, new_val)
                with open(filepath, "w", encoding="utf-8") as f:
                    f.write(content)
                print(f"  [+] Bumped version in {filepath}")

    # 2. Patch index.html (Inject Smart Routing Logic)
    index_html = "backend/frontend/index.html"
    if os.path.exists(index_html):
        with open(index_html, "r", encoding="utf-8") as f:
            html_content = f.read()

        # A. Inject window.handleShare function
        old_onload_start = "window.onload = () => {"
        new_handle_share = """window.handleShare = function(text, subject) {
            text = text || '';
            subject = subject || '';
            
            // Regex to find the first valid URL
            const urlMatch = text.match(/(https?:\\/\\/[^\\s]+)/) || subject.match(/(https?:\\/\\/[^\\s]+)/);
            
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
                if (subject) content += subject + "\\n\\n";
                if (text) content += text;
                
                document.getElementById('quickText').value = content.trim();
                document.getElementById('quickPanel').classList.remove('hidden');
                document.getElementById('bmPanel').classList.add('hidden');
            }
        };

        window.onload = () => {"""
        
        if old_onload_start in html_content and "window.handleShare = function" not in html_content:
            html_content = html_content.replace(old_onload_start, new_handle_share)
            print("  [+] Injected window.handleShare logic into index.html")

        # B. Wire URL Parameters to new logic
        old_url_params = """            const params = new URLSearchParams(window.location.search);
            if (params.has('share_url')) {
                document.getElementById('bmUrl').value = params.get('share_url');
                document.getElementById('bmTitle').value = params.get('share_title') || '';
                document.getElementById('bmPanel').classList.remove('hidden');
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }"""
        
        new_url_params = """            const params = new URLSearchParams(window.location.search);
            if (params.has('share_text') || params.has('share_subject')) {
                window.handleShare(params.get('share_text'), params.get('share_subject'));
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }"""

        if old_url_params in html_content:
            html_content = html_content.replace(old_url_params, new_url_params)
            print("  [+] Rewired boot query parameters to handleShare in index.html")

        with open(index_html, "w", encoding="utf-8") as f:
            f.write(html_content)


    # 3. Patch MainActivity.java to pass raw parameters to JS instead of pre-formatting
    main_activity = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(main_activity):
        with open(main_activity, "r", encoding="utf-8") as f:
            java_content = f.read()

        # Fix Boot-up URL loading
        old_run = """String startUrl = "http://127.0.0.1:8080/Welcome.html";
                android.content.Intent intent = getIntent();
                if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
                    String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
                    String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
                    if (sharedText != null) {
                        startUrl += "?share_url=" + android.net.Uri.encode(sharedText);
                        if (sharedSubject != null) {
                            startUrl += "&share_title=" + android.net.Uri.encode(sharedSubject);
                        }
                    }
                }
                webView.loadUrl(startUrl);"""
        
        new_run = """String startUrl = "http://127.0.0.1:8080/Welcome.html";
                android.content.Intent intent = getIntent();
                if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
                    String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
                    String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
                    startUrl += "?share_text=" + (sharedText != null ? android.net.Uri.encode(sharedText) : "") + 
                                "&share_subject=" + (sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "");
                }
                webView.loadUrl(startUrl);"""

        if old_run in java_content:
            java_content = java_content.replace(old_run, new_run)
            print("  [+] Rewired Java boot intent to pass raw text/subject URL parameters.")

        # Fix onNewIntent execution
        old_intent_logic = """        if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
            String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
            String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
            if (sharedText != null && webView != null) {
                String title = sharedSubject != null ? sharedSubject : "";
                String js = "javascript:(function(){" +
                    "document.getElementById('bmUrl').value = decodeURIComponent('" + android.net.Uri.encode(sharedText) + "');" +
                    "document.getElementById('bmTitle').value = decodeURIComponent('" + android.net.Uri.encode(title) + "');" +
                    "document.getElementById('bmPanel').classList.remove('hidden');" +
                    "})();";
                webView.evaluateJavascript(js, null);
            }
        }"""

        new_intent_logic = """        if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
            String sharedText = intent.getStringExtra(android.content.Intent.EXTRA_TEXT);
            String sharedSubject = intent.getStringExtra(android.content.Intent.EXTRA_SUBJECT);
            if (webView != null) {
                String tText = sharedText != null ? android.net.Uri.encode(sharedText) : "";
                String tSubj = sharedSubject != null ? android.net.Uri.encode(sharedSubject) : "";
                String js = "javascript:(function(){ if(window.handleShare) window.handleShare(decodeURIComponent('" + tText + "'), decodeURIComponent('" + tSubj + "')); })();";
                webView.evaluateJavascript(js, null);
            }
        }"""

        if old_intent_logic in java_content:
            java_content = java_content.replace(old_intent_logic, new_intent_logic)
            print("  [+] Upgraded Java onNewIntent to securely trigger window.handleShare.")

        with open(main_activity, "w", encoding="utf-8") as f:
            f.write(java_content)

    commit_msg = """feat(integration): intelligent share routing for bookmarks and quick notes

- Extracted routing decisions out of Android Java string templates and moved them to a dedicated `window.handleShare` Javascript function in `index.html`.
- Implemented smart Regex URL detection. If a shared intent contains `http://` or `https://` anywhere in its text, it auto-routes to the Bookmark Panel.
- If a shared intent (like highlighted text) lacks a URL, it securely injects the payload directly into the Quick Note UI.
- Rewrote Android's `MainActivity.java` `onCreate` and `onNewIntent` to pass the `EXTRA_TEXT` and `EXTRA_SUBJECT` primitives agnostically to the frontend.
- Bumped application to V1.2.21 (Android 10221)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()