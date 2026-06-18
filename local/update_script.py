import os
import re

def update_application():
    index_path = "backend/frontend/index.html"
    if not os.path.exists(index_path):
        print(f"WARNING: File not found: {index_path}")
        return

    with open(index_path, "r", encoding="utf-8") as f:
        html = f.read()

    # 1. Extract CSS
    css_match = re.search(r'<style>(.*?)</style>', html, re.DOTALL)
    if not css_match:
        print("WARNING: Could not find <style> block in index.html")
        return
    
    css_content = css_match.group(1).strip()
    html = html.replace(css_match.group(0), '<link rel="stylesheet" href="/css/omn-go-core.css">')

    # 2. Extract JavaScript 
    js_content = ""

    # Block A: Main logic (leaves APP_VERSION and OMN_GO_PAGE_NAME_JS in place)
    main_pattern = r'(<script>\s*/\* OMN_GO_PAGE_NAME_JS \*/\s*const APP_VERSION = "[^"]+";)(.*?)(\s*</script>)'
    match = re.search(main_pattern, html, re.DOTALL)
    if match:
        js_content += match.group(2).strip() + "\n\n"
        # Terminate the dynamic variable script block and inject the external JS link
        new_script_block = match.group(1) + "\n    </script>\n    <script src=\"/js/omn-go-core.js\"></script>"
        html = html.replace(match.group(0), new_script_block)
    else:
        print("WARNING: Could not find main JS block in index.html")

    # Block B: KaTeX Auto-Rendering
    katex_pattern = r'<script>(\s*document\.addEventListener\("DOMContentLoaded", \(\) => {\s*// Setup Auto-Rendering.*?)\s*</script>'
    match = re.search(katex_pattern, html, re.DOTALL)
    if match:
        js_content += match.group(1).strip() + "\n\n"
        html = html.replace(match.group(0), "")

    # Block C: Version Footer Tracker
    footer_pattern = r'<script>(\s*document\.addEventListener\("DOMContentLoaded", \(\) => {\s*const footer = document\.getElementById\(\'omn-go-version-footer\'.*?)\s*</script>'
    match = re.search(footer_pattern, html, re.DOTALL)
    if match:
        js_content += match.group(1).strip() + "\n\n"
        html = html.replace(match.group(0), "")

    # Block D: JS Console Interceptor
    console_pattern = r'<!-- JS Console Interceptor & UI -->\s*<script>(.*?)\s*</script>'
    match = re.search(console_pattern, html, re.DOTALL)
    if match:
        js_content += match.group(1).strip() + "\n"
        html = html.replace(match.group(0), "")

    # Clean up empty lines created by extraction
    html = re.sub(r'\n\s*\n\s*\n', '\n\n', html)

    # 3. Write Extracted Files to Disk
    os.makedirs("backend/frontend/html/css", exist_ok=True)
    os.makedirs("backend/frontend/html/js", exist_ok=True)

    with open("backend/frontend/html/css/omn-go-core.css", "w", encoding="utf-8") as f:
        f.write(css_content)

    # Automatically upgrade the hardcoded JS fallback version before saving
    js_content = re.sub(r"let v = '1\.3\.\d+';", "let v = '1.3.5';", js_content)
    with open("backend/frontend/html/js/omn-go-core.js", "w", encoding="utf-8") as f:
        f.write(js_content)

    # 4. Bump Versions to 1.3.5
    html = re.sub(r'const APP_VERSION = "1\.3\.\d+";', 'const APP_VERSION = "1.3.5";', html)
    with open(index_path, "w", encoding="utf-8") as f:
        f.write(html)

    # Bump server.go
    server_path = "backend/server.go"
    if os.path.exists(server_path):
        with open(server_path, "r", encoding="utf-8") as f:
            server_code = f.read()
        server_code = re.sub(r'const APP_VERSION = "1\.3\.\d+"', 'const APP_VERSION = "1.3.5"', server_code)
        with open(server_path, "w", encoding="utf-8") as f:
            f.write(server_code)

    # Bump build.gradle
    gradle_path = "android/app/build.gradle"
    if os.path.exists(gradle_path):
        with open(gradle_path, "r", encoding="utf-8") as f:
            gradle_code = f.read()
        gradle_code = re.sub(r'versionCode \d+', 'versionCode 10305', gradle_code)
        gradle_code = re.sub(r'versionName "1\.3\.\d+"', 'versionName "1.3.5"', gradle_code)
        with open(gradle_path, "w", encoding="utf-8") as f:
            f.write(gradle_code)

    print("SUCCESS: Core CSS and JS extracted, linked, and versions bumped to 1.3.5.")
    
    commit_msg = """refactor(frontend): extract core css/js into external files\n\nModularized index.html by abstracting layout styles and logic into omn-go-core assets. Version bumped to 1.3.5."""
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()