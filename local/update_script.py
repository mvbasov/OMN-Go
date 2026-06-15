import os
import re

def update_application():
    # 1. Bump Global Application Version
    version_replacements = [
        ("server.go", 'APP_VERSION = "1.0.43"', 'APP_VERSION = "1.0.44"'),
        ("frontend/index.html", 'APP_VERSION = "1.0.43"', 'APP_VERSION = "1.0.44"')
    ]
    
    # 2. Define File Patches (Target exact string mapping)
    patches = {
        "server.go": [
            # Patch 1: Add Font MIME types
            (
                r'''	mime.AddExtensionType(".json", "application/json")

	go func() {''',
                r'''	mime.AddExtensionType(".json", "application/json")
	mime.AddExtensionType(".woff", "font/woff")
	mime.AddExtensionType(".woff2", "font/woff2")
	mime.AddExtensionType(".ttf", "font/ttf")

	go func() {'''
            ),
            
            # Patch 2: Add specific route for /css/fonts/ to bypass the .css lock
            (
                r'''		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))''',
                r'''		mux.Handle("/js/", serveStrict(".js", "application/javascript"))
		mux.Handle("/css/fonts/", serveStrict(".woff2", "font/woff2"))
		mux.Handle("/css/", serveStrict(".css", "text/css"))'''
            )
        ]
    }

    # Execute Version Bump
    for filepath, old_v, new_v in version_replacements:
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if os.path.exists(actual_path):
            with open(actual_path, 'r', encoding='utf-8') as f:
                content = f.read()
            if old_v in content:
                with open(actual_path, 'w', encoding='utf-8') as f:
                    f.write(content.replace(old_v, new_v))

    # Execute Backend Patches Sequentially with Newline Normalization
    for filepath, file_patches in patches.items():
        actual_path = filepath if os.path.exists(filepath) else f"backend/{filepath}"
        if not os.path.exists(actual_path):
            continue

        with open(actual_path, 'r', encoding='utf-8') as f:
            content = f.read()

        original_newlines = "\r\n" if "\r\n" in content else "\n"
        normalized_content = content.replace("\r\n", "\n")

        for old_str, new_str in file_patches:
            old_norm = old_str.replace("\r\n", "\n")
            new_norm = new_str.replace("\r\n", "\n")
            
            if old_norm in normalized_content:
                normalized_content = normalized_content.replace(old_norm, new_norm)
            elif new_norm in normalized_content:
                print(f"Patch already applied in {actual_path}")
            else:
                raise ValueError(f"Target string not found in {actual_path}:\n{old_norm}")

        final_content = normalized_content.replace("\n", original_newlines)

        with open(actual_path, 'w', encoding='utf-8') as f:
            f.write(final_content)
        print(f"Successfully patched {actual_path}")

    # 3. HTML Injection Logic (Smart DOM Mutation approach)
    html_path = "frontend/index.html" if os.path.exists("frontend/index.html") else "backend/frontend/index.html"
    if os.path.exists(html_path):
        with open(html_path, 'r', encoding='utf-8') as f:
            html_content = f.read()
        
        # Inject CSS before </head>
        if 'katex.min.css' not in html_content:
            css_inject = '    <link rel="stylesheet" href="/css/highlight.default.min.css">\n    <link rel="stylesheet" href="/css/katex.min.css">\n</head>'
            html_content = re.sub(r'(?i)</head>', css_inject, html_content)
        
        # Inject JS before </body>
        if 'katex.min.js' not in html_content:
            js_inject = """
    <!-- Code & Math Formatting Assets -->
    <script src="/js/highlight.min.js"></script>
    <script src="/js/katex.min.js"></script>
    <script src="/js/auto-render.min.js"></script>
    <script>
        document.addEventListener("DOMContentLoaded", () => {
            // 1. Hook Highlight.js into Marked parser globally
            if (window.marked && window.hljs) {
                window.marked.setOptions({
                    highlight: function(code, lang) {
                        const language = window.hljs.getLanguage(lang) ? lang : 'plaintext';
                        return window.hljs.highlight(code, { language }).value;
                    },
                    langPrefix: 'hljs language-'
                });
            }
            
            // 2. Setup Auto-Rendering for KaTeX via MutationObserver
            // This guarantees Math renders regardless of how you inject the Markdown!
            const previewNode = document.getElementById('preview') || document.body;
            let renderTimeout;
            const observer = new MutationObserver(() => {
                clearTimeout(renderTimeout);
                renderTimeout = setTimeout(() => {
                    if (window.renderMathInElement) {
                        renderMathInElement(previewNode, {
                            delimiters: [
                                {left: '$$', right: '$$', display: true},
                                {left: '$', right: '$', display: false},
                                {left: '\\\\(', right: '\\\\)', display: false},
                                {left: '\\\\[', right: '\\\\]', display: true}
                            ],
                            throwOnError: false
                        });
                    }
                }, 50);
            });
            observer.observe(previewNode, { childList: true, subtree: true });
        });
    </script>
</body>"""
            html_content = re.sub(r'(?i)</body>', js_inject, html_content)
        
        with open(html_path, 'w', encoding='utf-8') as f:
            f.write(html_content)
        print(f"Successfully injected KaTeX & Highlight.js into {html_path}")

    # 4. Output Standardized Git Commit Message
    commit_msg = """feat(frontend): implement KaTeX and Highlight.js rendering offline
    
- Add global MIME types for `.woff` and `.woff2` font files.
- Create specific `serveStrict` route for `/css/fonts/` to bypass the `.css` extension lock.
- Inject highlight.js and katex resources into `index.html`.
- Implement DOM MutationObserver to automatically render math without altering core markdown logic.

Version bumped to 1.0.44"""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()