import os
import re

def update_application():
    # 1. Update MainActivity.java with programmatic Native Layout & Spinner
    java_path = "android/app/src/main/java/net/basov/omngo/MainActivity.java"
    if os.path.exists(java_path):
        with open(java_path, "r", encoding="utf-8") as f:
            java_code = f.read()

        # Inject FrameLayout and ProgressBar before WebView initialization
        pattern1 = r'\s*// Initialize WebView\s*webView = new WebView\(this\);\s*WebSettings webSettings = webView\.getSettings\(\);\s*webSettings\.setJavaScriptEnabled\(true\);\s*webSettings\.setDomStorageEnabled\(true\);\s*webView\.setWebViewClient\(new WebViewClient\(\) \{'
        
        replacement1 = """
        // Create Native Loading Layout
        android.widget.FrameLayout rootLayout = new android.widget.FrameLayout(this);
        rootLayout.setBackgroundColor(android.graphics.Color.parseColor("#f9f9f9"));
        
        final android.widget.ProgressBar progressBar = new android.widget.ProgressBar(this);
        android.widget.FrameLayout.LayoutParams pbParams = new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT,
            android.view.ViewGroup.LayoutParams.WRAP_CONTENT);
        pbParams.gravity = android.view.Gravity.CENTER;
        progressBar.setLayoutParams(pbParams);

        // Initialize WebView
        webView = new WebView(this);
        webView.setLayoutParams(new android.widget.FrameLayout.LayoutParams(
            android.view.ViewGroup.LayoutParams.MATCH_PARENT,
            android.view.ViewGroup.LayoutParams.MATCH_PARENT));

        WebSettings webSettings = webView.getSettings();
        webSettings.setJavaScriptEnabled(true);
        webSettings.setDomStorageEnabled(true);
        webView.setWebViewClient(new WebViewClient() {
            @Override
            public void onPageFinished(WebView view, String url) {
                progressBar.setVisibility(android.view.View.GONE);
                super.onPageFinished(view, url);
            }"""
        
        java_code = re.sub(pattern1, lambda _: replacement1, java_code, flags=re.DOTALL)

        # Wire the new layout into setContentView
        pattern2 = r'\s*setContentView\(webView\);'
        replacement2 = """
        rootLayout.addView(webView);
        rootLayout.addView(progressBar);
        setContentView(rootLayout);"""
        
        java_code = re.sub(pattern2, lambda _: replacement2, java_code)

        with open(java_path, "w", encoding="utf-8") as f:
            f.write(java_code)
    else:
        print(f"WARNING: File not found: {java_path}")


    # 2. Optimize server.go to unblock HTTP initialization
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            server_code = f.read()

        server_code = re.sub(
            r'\s*// Precompile all notes to data/html/ at startup\s*precompileAllPages\(\)',
            lambda _: "\n\t// Precompile all notes to data/html/ at startup in the background\n\tgo precompileAllPages()",
            server_code
        )
        
        # Bump version string
        server_code = re.sub(r'const APP_VERSION = "1\.3\.\d+"', 'const APP_VERSION = "1.3.7"', server_code)
        
        with open(server_path, "w", encoding="utf-8") as f:
            f.write(server_code)
            
            
    # 3. Update index.html version
    index_path = "backend/frontend/index.html"
    if os.path.exists(index_path):
        with open(index_path, "r", encoding="utf-8") as f:
            html = f.read()
        html = re.sub(r'const APP_VERSION = "1\.3\.\d+";', 'const APP_VERSION = "1.3.7";', html)
        with open(index_path, "w", encoding="utf-8") as f:
            f.write(html)
            
            
    # 4. Update core JS version
    js_path = "backend/frontend/html/js/omn-go-core.js"
    if os.path.exists(js_path):
        with open(js_path, "r", encoding="utf-8") as f:
            js = f.read()
        js = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.7';", js)
        with open(js_path, "w", encoding="utf-8") as f:
            f.write(js)


    # 5. Bump Android build.gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            gradle_code = f.read()
        gradle_code = re.sub(r'versionCode \d+', 'versionCode 10307', gradle_code)
        gradle_code = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.7"', gradle_code)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(gradle_code)

    print("SUCCESS: Native loading screen implemented and Go boot bottleneck removed.")
    
    commit_msg = """feat(android): programmatic loading screen & boot optimization\n\nImplemented native FrameLayout and ProgressBar in Java to mask the WebView loading phase. Pushed precompileAllPages() into a background goroutine to allow instant HTTP port binding. Version bumped to 1.3.7."""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()