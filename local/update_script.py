import os
import re

def update_application():
    print("[*] Initiating OMN-Go V1.2.17 LAN Bind & Editor Timestamp Fixes...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.16"', 'APP_VERSION = "1.2.17"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.16";', 'const APP_VERSION = "1.2.17";'),
        ("backend/frontend/index.html", "let v = '1.2.16';", "let v = '1.2.17';"),
        ("android/app/build.gradle", "versionCode 10216", "versionCode 10217"),
        ("android/app/build.gradle", 'versionName "1.2.16"', 'versionName "1.2.17"')
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

    # 2. Patch server.go to check file ModTime and bind to 0.0.0.0 (LAN)
    server_go = "backend/server.go"
    if os.path.exists(server_go):
        with open(server_go, "r", encoding="utf-8") as f:
            server_code = f.read()

        # A. Bind to 0.0.0.0 to enable LAN access
        old_bind = '''		bindAddr := fmt.Sprintf("127.0.0.1:%d", appConfig.ServerPort)
		if runtime.GOOS != "android" {
			bindAddr = fmt.Sprintf(":%d", appConfig.ServerPort)
		}'''
        
        new_bind = '''		bindAddr := fmt.Sprintf("0.0.0.0:%d", appConfig.ServerPort)'''
        
        if old_bind in server_code:
            server_code = server_code.replace(old_bind, new_bind)
            print("  [+] Configured Go backend to bind to 0.0.0.0 (LAN Access Enabled).")

        # B. Compare .md vs .html timestamps to force recompile on external edits
        old_html_check = '''		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))

		if _, err := os.Stat(htmlPath); os.IsNotExist(err) {
			mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))
			if _, err := os.Stat(mdPath); os.IsNotExist(err) {'''
        
        new_html_check = '''		htmlPath := filepath.Join(storageDir, "html", filepath.Clean(name+".html"))
		mdPath := filepath.Join(storageDir, "md", filepath.Clean(name+".md"))

		htmlStat, errHtml := os.Stat(htmlPath)
		mdStat, errMd := os.Stat(mdPath)

		// Recompile if HTML is missing, OR if Markdown was modified more recently than HTML
		if os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {'''
        
        if old_html_check in server_code:
            server_code = server_code.replace(old_html_check, new_html_check)
            print("  [+] Upgraded compile engine to track Markdown vs HTML timestamps.")

        with open(server_go, "w", encoding="utf-8") as f:
            f.write(server_code)

    # 3. Patch MainActivity.java to wait for Intent result and reload WebView
    main_activity = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(main_activity):
        with open(main_activity, "r", encoding="utf-8") as f:
            java_content = f.read()

        # Change standard startActivity to startActivityForResult
        old_start_activity = 'view.getContext().startActivity(android.content.Intent.createChooser(intent, "Edit Markdown File"));'
        new_start_activity = 'MainActivity.this.startActivityForResult(android.content.Intent.createChooser(intent, "Edit Markdown File"), 1001);'
        
        if old_start_activity in java_content:
            java_content = java_content.replace(old_start_activity, new_start_activity)
            print("  [+] Converted ACTION_EDIT to startActivityForResult.")

        # Inject onActivityResult listener before onBackPressed
        old_on_back = '    @Override\n    public void onBackPressed() {'
        new_on_back = '''    @Override
    protected void onActivityResult(int requestCode, int resultCode, android.content.Intent data) {
        super.onActivityResult(requestCode, resultCode, data);
        if (requestCode == 1001 && webView != null) {
            webView.reload(); // Refresh view when returning from external editor
        }
    }

    @Override
    public void onBackPressed() {'''
        
        if old_on_back in java_content and "onActivityResult" not in java_content:
            java_content = java_content.replace(old_on_back, new_on_back)
            print("  [+] Injected onActivityResult to automatically reload WebView.")

        with open(main_activity, "w", encoding="utf-8") as f:
            f.write(java_content)

    commit_msg = """fix(engine): enable LAN access and auto-refresh external edits

- Modified `server.go` to explicitly bind to `0.0.0.0`, safely exposing the Go WebServer to the LAN (WiFi) without interrupting Android's internal `127.0.0.1` routing.
- Upgraded the static file router to compare `.md` and `.html` file modification timestamps (`ModTime`). If an external editor saves changes to the Markdown, the server automatically recompiles the HTML.
- Converted Android's `startActivity` to `startActivityForResult` in `MainActivity.java`.
- Added an `onActivityResult` listener to the Android lifecycle that automatically triggers `webView.reload()` the moment the user returns from an external editor.
- Bumped application to V1.2.17 (Android 10217)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()