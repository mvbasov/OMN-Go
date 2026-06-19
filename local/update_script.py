import os
import re

def apply_patch(filepath, old_str, new_str, description):
    print(f"\n[PATCH] {description}")
    print(f"  Target: {filepath}")
    
    if not os.path.exists(filepath):
        print(f"  [-] ERROR: File {filepath} not found!")
        return False
        
    with open(filepath, "r", encoding="utf-8") as f:
        content = f.read()
        
    if old_str in content:
        content = content.replace(old_str, new_str)
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content)
        print("  [+] SUCCESS: Exact string match replaced.")
        return True
        
    # Fallback to normalized newlines for cross-platform OS safety
    old_normalized = old_str.replace('\r\n', '\n')
    content_normalized = content.replace('\r\n', '\n')
    
    if old_normalized in content_normalized:
        print("  [~] WARNING: Exact match failed, but normalized newline match succeeded. Applying.")
        content_normalized = content_normalized.replace(old_normalized, new_str.replace('\r\n', '\n'))
        with open(filepath, "w", encoding="utf-8") as f:
            f.write(content_normalized)
        print("  [+] SUCCESS: Normalized match replaced.")
        return True
        
    print("  [-] ERROR: Target string NOT FOUND!")
    print("  --- Expected snippet ---")
    print(old_str[:200].strip() + " ...")
    return False

def bump_versions():
    print("\n[VERSION BUMP] Upgrading to 1.3.12")
    versions = [
        ("backend/server.go", 'APP_VERSION = "1.3.11"', 'APP_VERSION = "1.3.12"'),
        ("backend/frontend/index.html", 'const APP_VERSION = "1.3.11";', 'const APP_VERSION = "1.3.12";'),
        ("android/app/build.gradle", 'versionCode 10311', 'versionCode 10312'),
        ("android/app/build.gradle", 'versionName "1.3.11"', 'versionName "1.3.12"')
    ]
    
    for fp, old, new in versions:
        if os.path.exists(fp):
            with open(fp, "r", encoding="utf-8") as f:
                content = f.read()
            # If we missed the exact 1.3.11 state due to a failed run, dynamically bump via Regex
            if old not in content:
                print(f"  [~] {fp}: Exact old version string not found. Trying dynamic Regex bump...")
                if "build.gradle" in fp:
                    content = re.sub(r'versionCode\s+\d+', 'versionCode 10312', content)
                    content = re.sub(r'versionName\s+"1\.3\.\d+"', 'versionName "1.3.12"', content)
                else:
                    content = re.sub(r'APP_VERSION = "1\.3\.\d+"', 'APP_VERSION = "1.3.12"', content)
                    content = re.sub(r'APP_VERSION = \'1\.3\.\d+\'', 'APP_VERSION = "1.3.12"', content)
            else:
                content = content.replace(old, new)
                
            with open(fp, "w", encoding="utf-8") as f:
                f.write(content)
            print(f"  [+] Bumped version in {fp}")
        else:
            print(f"  [-] Skipped {fp} (File not found)")

def update_application():
    print("==================================================")
    print(" OMN-Go Update Initialized (Target: V1.3.12)")
    print("==================================================")
    
    bump_versions()

    # 1. Update Server Router to force compilation on ?refresh=1
    old_server = r"""		if os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {"""
    new_server = r"""		forceRefresh := r.URL.Query().Get("refresh") == "1" || r.URL.Query().Get("refresh") == "true"
		if forceRefresh || os.IsNotExist(errHtml) || (errHtml == nil && errMd == nil && mdStat.ModTime().After(htmlStat.ModTime())) {
			if os.IsNotExist(errMd) {"""
    apply_patch("backend/server.go", old_server, new_server, "Inject ?refresh=1 query listener into serveFrontend")

    # 2. Add UI Buttons to index.html Header
    old_html = r"""            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>"""
    new_html = r"""            <button onclick="createNewPage()" class="admin-only" style="background: #17a2b8; border-color: #17a2b8;">New Page</button>
            <button onclick="window.location.href = window.location.pathname + '?refresh=1'" class="admin-only" style="background: #6c757d; border-color: #6c757d;">Refresh</button>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only">Quick Note</button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only">Add Bookmark</button>"""
    apply_patch("backend/frontend/index.html", old_html, new_html, "Add New Page and Refresh buttons to header")

    # 3. Add JS Logic for New Page Generation and Backlink Insertion
    old_js = r"""        async function saveNote() {
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
        }"""
        
    new_js = r"""        function toCamelCase(str) {
            let words = str.split(/[-_\s]+/);
            return words.map(w => w ? w.charAt(0).toUpperCase() + w.slice(1) : '').join('');
        }

        function createNewPage() {
            let title = prompt("Enter New Page Title:");
            if (!title) return;
            let camel = toCamelCase(title);
            let safeName = camel.replace(/[^a-zA-Z0-9-]/g, '-');
            let fileName = prompt("Confirm File Name:", safeName);
            if (!fileName) return;

            sessionStorage.setItem('omn_go_new_page_source', typeof currentNote !== 'undefined' ? currentNote : 'Welcome');
            sessionStorage.setItem('omn_go_new_page_title', title);
            sessionStorage.setItem('omn_go_new_page_target', fileName);

            window.location.href = '/' + fileName + '.html?edit=true';
        }

        async function saveNote() {
            let content = document.getElementById('editor').value;
            const fd = new URLSearchParams();
            fd.append('name', currentNote);
            fd.append('content', content);
            const res = await fetch('/api/save', { method: 'POST', body: fd });
            if(res.ok) {
                try {
                    let src = sessionStorage.getItem('omn_go_new_page_source');
                    let tgt = sessionStorage.getItem('omn_go_new_page_target');
                    let pTitle = sessionStorage.getItem('omn_go_new_page_title');

                    if (src && tgt === currentNote) {
                        sessionStorage.removeItem('omn_go_new_page_source');
                        sessionStorage.removeItem('omn_go_new_page_target');
                        sessionStorage.removeItem('omn_go_new_page_title');

                        let noteRes = await fetch('/api/note?name=' + encodeURIComponent(src));
                        if (noteRes.ok) {
                            let srcContent = await noteRes.text();
                            let parts = srcContent.split('\n\n');
                            let isHeader = parts.length > 0 && parts[0].includes(':') && !parts[0].startsWith(' ') && !parts[0].startsWith('#');
                            let linkStr = `* [${pTitle}](${tgt})`;

                            if (isHeader) {
                                if (parts.length > 1) {
                                    parts.splice(1, 0, linkStr);
                                } else {
                                    parts.push(linkStr);
                                }
                            } else {
                                parts.unshift(linkStr);
                            }

                            const srcFd = new URLSearchParams();
                            srcFd.append('name', src);
                            srcFd.append('content', parts.join('\n\n'));
                            await fetch('/api/save', { method: 'POST', body: srcFd });
                        }
                    }
                } catch(e) { console.error("Link injection failed", e); }

                alert('Note saved!');
                window.location.reload();
            } else {
                alert('Failed to save!');
            }
        }"""
    apply_patch("backend/frontend/html/js/omn-go-core.js", old_js, new_js, "Inject createNewPage() and update saveNote() with backlinking API hook")

    # 4. Auto-Flip into Edit Mode on Load
    old_onload = r"""            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            let hash = window.location.hash;"""
    new_onload = r"""            if (typeof currentNote !== 'undefined' && currentNote === 'Config') {
                const tb = document.getElementById('toggleBtn');
                if (tb) tb.style.display = 'none';
            }
            if (window.location.search.includes('edit=true')) {
                setTimeout(() => {
                    if (typeof currentMode !== 'undefined' && currentMode === 'view' && typeof toggleMode === 'function') toggleMode();
                }, 100);
            }
            let hash = window.location.hash;"""
    apply_patch("backend/frontend/html/js/omn-go-core.js", old_onload, new_onload, "Inject ?edit=true listener to toggle UI instantly")

    print("\n==================================================")
    print(" Update Complete! Check the logs above for status.")
    print("==================================================")
    
    commit_msg = "feat(ui): add New Page CamelCase workflow, auto-backlinking, and forced page Refresh\n\nVersion bumped to 1.3.12"
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()