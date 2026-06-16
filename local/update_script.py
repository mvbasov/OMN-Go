import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.20 Share Intents & Drag/Drop Fixes...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.19"', 'APP_VERSION = "1.2.20"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.19";', 'const APP_VERSION = "1.2.20";'),
        ("backend/frontend/index.html", "let v = '1.2.19';", "let v = '1.2.20';"),
        ("android/app/build.gradle", "versionCode 10219", "versionCode 10220"),
        ("android/app/build.gradle", 'versionName "1.2.19"', 'versionName "1.2.20"')
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

    # 2. Patch AndroidManifest.xml (Add singleTask and ACTION_SEND filter)
    manifest_path = "android/app/src/main/AndroidManifest.xml"
    if os.path.exists(manifest_path):
        with open(manifest_path, "r", encoding="utf-8") as f:
            manifest_content = f.read()
        
        if 'android:launchMode="singleTask"' not in manifest_content:
            manifest_content = manifest_content.replace('android:exported="true"', 'android:exported="true"\n            android:launchMode="singleTask"')
            
        send_filter = '''            <intent-filter>
                <action android:name="android.intent.action.SEND" />
                <category android:name="android.intent.category.DEFAULT" />
                <data android:mimeType="text/plain" />
            </intent-filter>
        </activity>'''
        
        if 'android.intent.action.SEND' not in manifest_content:
            manifest_content = manifest_content.replace('</activity>', send_filter)
            print("  [+] Added singleTask launchMode and ACTION_SEND intent filter to Manifest.")
            
        with open(manifest_path, "w", encoding="utf-8") as f:
            f.write(manifest_content)

    # 3. Patch MainActivity.java (Handle Intent on Boot and onNewIntent)
    main_activity = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(main_activity):
        with open(main_activity, "r", encoding="utf-8") as f:
            java_content = f.read()

        # Fix Boot-up URL loading
        old_run = 'webView.loadUrl("http://127.0.0.1:8080");'
        new_run = '''String startUrl = "http://127.0.0.1:8080/Welcome.html";
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
                webView.loadUrl(startUrl);'''
        
        if old_run in java_content:
            java_content = java_content.replace(old_run, new_run)
            print("  [+] Wired Android Boot Intent to pass URL parameters to WebView.")

        # Add onNewIntent for background resume
        old_back = '    @Override\n    public void onBackPressed() {'
        new_intent_logic = '''    @Override
    protected void onNewIntent(android.content.Intent intent) {
        super.onNewIntent(intent);
        setIntent(intent);
        if (android.content.Intent.ACTION_SEND.equals(intent.getAction()) && "text/plain".equals(intent.getType())) {
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
        }
    }

    @Override
    public void onBackPressed() {'''

        if old_back in java_content and "onNewIntent" not in java_content:
            java_content = java_content.replace(old_back, new_intent_logic)
            print("  [+] Added onNewIntent to dynamically inject shared links via JavaScript.")

        with open(main_activity, "w", encoding="utf-8") as f:
            f.write(java_content)

    # 4. Patch index.html (Handle Query Params and Desktop Drag/Drop)
    index_html = "backend/frontend/index.html"
    if os.path.exists(index_html):
        with open(index_html, "r", encoding="utf-8") as f:
            html_content = f.read()

        # Fix Desktop Drag/Drop
        old_drag = '''        // Image Drag & Drop
        const editor = document.getElementById('editor');
        editor.addEventListener('dragover', e => e.preventDefault());'''
        
        new_drag = '''        // Global Drag & Drop for URLs (Bookmarks)
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
                    const match = html.match(/<a[^>]*>(.*?)<\\/a>/i);
                    if (match && match[1]) {
                        document.getElementById('bmTitle').value = match[1].replace(/<[^>]+>/g, '').trim();
                    }
                }
                document.getElementById('bmPanel').classList.remove('hidden');
            }
        });

        // Image Drag & Drop
        const editor = document.getElementById('editor');
        editor.addEventListener('dragover', e => e.preventDefault());'''
        
        if old_drag in html_content:
            html_content = html_content.replace(old_drag, new_drag)
            print("  [+] Attached global URL Drag & Drop logic for Desktop browsers.")

        # Fix Boot Query Parameter Read
        old_onload = '''        window.onload = () => {
            checkSession();'''
        
        new_onload = '''        window.onload = () => {
            checkSession();
            
            const params = new URLSearchParams(window.location.search);
            if (params.has('share_url')) {
                document.getElementById('bmUrl').value = params.get('share_url');
                document.getElementById('bmTitle').value = params.get('share_title') || '';
                document.getElementById('bmPanel').classList.remove('hidden');
                window.history.replaceState({}, document.title, window.location.pathname + window.location.hash);
            }'''
            
        if old_onload in html_content:
            html_content = html_content.replace(old_onload, new_onload)
            print("  [+] Wired window.onload to parse ?share_url parameters.")

        with open(index_html, "w", encoding="utf-8") as f:
            f.write(html_content)

    commit_msg = """feat(integration): android share intents and desktop url drag-drop

- Appended `android:launchMode="singleTask"` to Manifest, ensuring OMN-Go is reused from the background instead of spawning duplicate server crash instances.
- Exposed Android `ACTION_SEND` intent filter for `text/plain` types allowing native OS "Share To" functionality.
- Upgraded `MainActivity.java` to read `EXTRA_TEXT` and `EXTRA_SUBJECT` on boot via HTTP query parameters.
- Overwrote `onNewIntent` in Java to dynamically execute JavaScript and overlay the Bookmark panel when sharing links to an already-running instance.
- Attached a global `dragover` / `drop` listener to `index.html` allowing Desktop users to drag URLs from other browser tabs directly onto the app to open the Bookmark UI.
- Bumped application to V1.2.20 (Android 10220)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()