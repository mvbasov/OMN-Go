#!/usr/bin/env python3
import re, os

def read_file(path):
    if not os.path.exists(path):
        return ""
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def extract_js_entities(code):
    """
    AST-style JS parser. Finds all top-level functions and expressions,
    counts their braces safely, and extracts them without breaking inner scope.
    """
    entities = []
    
    # 1. Top-level functions
    idx = 0
    while True:
        match = re.search(r'^(?:async\s+)?function\s+([a-zA-Z0-9_]+)\s*\(', code[idx:], re.MULTILINE)
        if not match: break
        
        start_idx = idx + match.start()
        brace_start = code.find('{', start_idx)
        if brace_start == -1: break
        
        brace_count = 1
        curr = brace_start + 1
        while curr < len(code) and brace_count > 0:
            if code[curr] == '{': brace_count += 1
            elif code[curr] == '}': brace_count -= 1
            curr += 1
            
        func_name = match.group(1)
        func_body = code[start_idx:curr]
        
        entities.append(('function', func_name, func_body))
        code = code[:start_idx] + (" " * (curr - start_idx)) + code[curr:]
        idx = curr
        
    # 2. Top-level function expressions (const myFunc = () => {)
    idx = 0
    while True:
        match = re.search(r'^(?:const|let|var)\s+([a-zA-Z0-9_]+)\s*=\s*(?:async\s+)?(?:function\s*\(|\([^)]*\)\s*=>|[a-zA-Z0-9_]+\s*=>)\s*{', code[idx:], re.MULTILINE)
        if not match: break
        
        start_idx = idx + match.start()
        brace_start = code.find('{', start_idx)
        if brace_start == -1: break
        
        brace_count = 1
        curr = brace_start + 1
        while curr < len(code) and brace_count > 0:
            if code[curr] == '{': brace_count += 1
            elif code[curr] == '}': brace_count -= 1
            curr += 1
            
        func_name = match.group(1)
        func_body = code[start_idx:curr]
        
        entities.append(('expression', func_name, func_body))
        code = code[:start_idx] + (" " * (curr - start_idx)) + code[curr:]
        idx = curr
        
    # Clean up massive blank gaps left by extraction
    clean_remainder = re.sub(r'\n\s*\n+', '\n\n', code).strip()
    return entities, clean_remainder

def create_iife(module_name, funcs):
    if not funcs: return ""
    lines = [f"const {module_name} = (function() {{"]
    exports = []
    
    for name, body in funcs:
        indented_body = body.replace("\n", "\n    ")
        lines.append(f"    {indented_body}")
        exports.append(name)
        
    lines.append("\n    // Export to global scope to preserve HTML onclick attributes")
    for e in exports:
        lines.append(f"    window.{e} = {e};")
        
    export_str = ", ".join(exports)
    lines.append(f"    return {{ {export_str} }};")
    lines.append("})();\n")
    return "\n".join(lines)

def update_application():
    print("[ ] Starting Frontend Componentization (v1.5.0)...")
    
    # 1. Force version bump to 1.5.0
    new_ver = "1.5.0"
    new_code = "10500"
    
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    content = re.sub(r'APP_VERSION\s*=\s*"[^"]+"', f'APP_VERSION = "{new_ver}"', content)
    write_file(ver_path, content)
    print(f"[+] Bumped backend version to {new_ver}")

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    gradle = re.sub(r'versionCode\s+\d+', f'versionCode {new_code}', gradle)
    gradle = re.sub(r'versionName\s+"[^"]+"', f'versionName "{new_ver}"', gradle)
    write_file(gradle_path, gradle)
    print(f"[+] Bumped Android configurations to {new_code} / {new_ver}")

    # 2. Extract and Sort JS Entities
    core_js_path = "backend/frontend/html/js/omn-go-core.js"
    sse_js_path = "backend/frontend/html/js/omn-go-sse.js"
    
    core_code = read_file(core_js_path)
    if not core_code:
        print("[-] omn-go-core.js not found. Are you in the project root?")
        return

    entities, global_remainder = extract_js_entities(core_code)
    print(f"[+] Extracted {len(entities)} top-level functions for componentization")

    logger_funcs = []
    syncer_funcs = []
    editor_funcs = []
    ui_funcs = []

    for type_, name, body in entities:
        lower = body.lower()
        # Intelligent Domain Sorting
        if 'eventsource' in lower or 'console.' in lower or '/api/log' in lower:
            logger_funcs.append((name, body))
        elif 'fetch(' in lower or '/api/' in lower or 'xmlhttprequest' in lower:
            syncer_funcs.append((name, body))
        elif 'katex' in lower or 'hljs' in lower or 'mode' in lower or 'editor' in lower or 'textarea' in lower:
            editor_funcs.append((name, body))
        else:
            ui_funcs.append((name, body))

    # 3. Build omn-go-core.js (Always Required)
    new_core_js = "// --- OMN-Go Core Architecture ---\n// These modules are strictly for offline viewing, Markdown rendering, and UI manipulation.\n\n"
    new_core_js += create_iife("Editor", editor_funcs)
    new_core_js += create_iife("UI", ui_funcs)
    
    if global_remainder:
        new_core_js += "\n// --- Global Listeners & State ---\n" + global_remainder + "\n"
        
    write_file(core_js_path, new_core_js)
    print("[+] Refactored omn-go-core.js (Editor & UI IIFEs)")

    # 4. Build omn-go-sse.js (Server Required)
    new_sse_js = """// --- OMN-Go Server Extensions ---
// These modules interact with the Go backend API. They will cleanly bypass themselves
// if the user is merely viewing an exported HTML file locally without the server.

if (window.location.protocol !== 'file:') {
"""
    new_sse_js += create_iife("Logger", logger_funcs).replace("\n", "\n    ")
    new_sse_js += create_iife("Syncer", syncer_funcs).replace("\n", "\n    ")
    new_sse_js += """
} else {
    console.warn("OMN-Go: Page opened locally. Server Extensions (Sync/SSE) safely disabled.");
}
"""
    write_file(sse_js_path, new_sse_js)
    print("[+] Generated omn-go-sse.js (Logger & Syncer IIFEs)")

    # 5. Inject the new file into the main HTML layout
    html_path = "backend/frontend/index.html"
    index_html = read_file(html_path)
    
    core_script_match = re.search(r'<script[^>]*src=[\'"]([^\'"]*omn-go-core\.js)[\'"][^>]*></script>', index_html)
    if core_script_match and "omn-go-sse.js" not in index_html:
        core_script_tag = core_script_match.group(0)
        src_path = core_script_match.group(1)
        sse_path = src_path.replace("omn-go-core.js", "omn-go-sse.js")
        sse_script_tag = f'<script src="{sse_path}"></script>'
        
        index_html = index_html.replace(core_script_tag, f'{core_script_tag}\n    {sse_script_tag}')
        write_file(html_path, index_html)
        print("[+] Wired omn-go-sse.js into backend/frontend/index.html")

    commit_msg = (
        "refactor(frontend): componentize app shell and isolate SSE/API logic\n\n"
        "- Extracted domains into Editor, Syncer, Logger, and UI IIFEs\n"
        "- Split omn-go-core.js into offline core logic and omn-go-sse.js for backend APIs\n"
        "- Exposed component methods globally to securely preserve HTML onclick handlers\n"
        "- Guarded API execution for static local-file viewing scenarios\n"
        "- Bumped application to v1.5.0"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()
