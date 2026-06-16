import os

def update_application():
    print("[*] Initiating OMN-Go V1.2.10 KaTeX SSR Initialization Fix...")

    # 1. Version Bumps
    version_replacements = [
        ("backend/server.go", 'APP_VERSION = "1.2.9"', 'APP_VERSION = "1.2.10"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.2.9";', 'const APP_VERSION = "1.2.10";'),
        ("backend/frontend/index.html", "let v = '1.2.9';", "let v = '1.2.10';"),
        ("android/app/build.gradle", "versionCode 10209", "versionCode 10210"),
        ("android/app/build.gradle", 'versionName "1.2.9"', 'versionName "1.2.10"')
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

    # 2. Patch index.html to explicitly render math on window.onload
    index_html = "backend/frontend/index.html"
    if os.path.exists(index_html):
        with open(index_html, "r", encoding="utf-8") as f:
            html_content = f.read()

        old_onload_target = """            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }"""

        # We use raw strings (r"") to ensure the backslashes for KaTeX escape sequences stay intact
        new_onload_target = r"""            if (window.hljs) {
                document.querySelectorAll('#preview pre code').forEach((block) => {
                    hljs.highlightElement(block);
                });
            }
            if (window.renderMathInElement) {
                renderMathInElement(document.getElementById('preview') || document.body, {
                    delimiters: [
                        {left: '$$', right: '$$', display: true},
                        {left: '$', right: '$', display: false},
                        {left: '\\(', right: '\\)', display: false},
                        {left: '\\[', right: '\\]', display: true}
                    ],
                    throwOnError: false
                });
            }"""

        if old_onload_target in html_content:
            html_content = html_content.replace(old_onload_target, new_onload_target)
            with open(index_html, "w", encoding="utf-8") as f:
                f.write(html_content)
            print(f"  [+] Injected explicit KaTeX trigger into window.onload in {index_html}")
        elif "renderMathInElement(document.getElementById('preview')" in html_content:
            print(f"  [=] Explicit KaTeX trigger already exists in {index_html}")
        else:
            print(f"  [-] Could not find target block to patch inside {index_html}")

    commit_msg = """fix(frontend): explicitly render KaTeX on window.onload for SSR

- Added explicit `renderMathInElement` execution during the `window.onload` sequence.
- Resolved issue where Server-Side Rendered (SSR) HTML bypassed the MutationObserver, leaving initial `$$...$$` blocks unparsed.
- Bumped application to V1.2.10 (Android 10210)."""

    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()