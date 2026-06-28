import os
import re
import sys

VERSION = "1.5.11"
VERSION_CODE = "10511"

def read_file(filepath):
    if not os.path.exists(filepath):
        return None
    with open(filepath, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(filepath, content):
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write(content)
    print(f"[+] Successfully patched {filepath}")

def bump_versions():
    v_path = os.path.join("backend", "version.go")
    content = read_file(v_path)
    if content:
        content = re.sub(r'APP_VERSION\s*=\s*".*?"', f'APP_VERSION = "{VERSION}"', content)
        write_file(v_path, content)
    
    b_path = os.path.join("android", "app", "build.gradle")
    content = read_file(b_path)
    if content:
        content = re.sub(r'versionCode\s+\d+', f'versionCode {VERSION_CODE}', content)
        content = re.sub(r'versionName\s+".*?"', f'versionName "{VERSION}"', content)
        write_file(b_path, content)

def patch_git_helper():
    path = os.path.join("backend", "git_helper.go")
    content = read_file(path)
    if not content: return

    # Fix assignment mismatch: AddWithOptions only returns 1 value (err)
    content = re.sub(r'[a-zA-Z0-9_]+\s*,\s*err\s*:=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err := \1.AddWithOptions', content)
    content = re.sub(r'_\s*,\s*err\s*:=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err := \1.AddWithOptions', content)
    content = re.sub(r'[a-zA-Z0-9_]+\s*,\s*err\s*=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err = \1.AddWithOptions', content)
    content = re.sub(r'_\s*,\s*err\s*=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err = \1.AddWithOptions', content)

    write_file(path, content)

def patch_css():
    # Dynamically find the CSS file anywhere in the directory tree
    css_path = None
    for root, dirs, files in os.walk("."):
        if "omn-go-core.css" in files:
            css_path = os.path.join(root, "omn-go-core.css")
            break
    
    if css_path:
        content = read_file(css_path)
        if content and ".git-server-card" not in content:
            css_injection = """
/* ====== Git Server UI Styles ====== */
.git-server-card { padding: 1rem; margin-bottom: 1rem; border: 1px solid #ddd; border-radius: 4px; box-shadow: 0 1px 2px rgba(0,0,0,0.05); background-color: #f9fafb; color: #000; }
.git-server-label { font-weight: bold; display: flex; align-items: center; gap: 0.5rem; }
.git-server-grid { display: grid; grid-template-columns: 1fr 1fr; gap: 0.5rem; margin-top: 0.5rem; }
.git-server-input { border: 1px solid #ccc; padding: 0.5rem; width: 100%; box-sizing: border-box; border-radius: 4px; }
@media (max-width: 600px) { .git-server-grid { grid-template-columns: 1fr; } }
"""
            content += "\n" + css_injection
            write_file(css_path, content)
            print(f"[+] Appended Git UI styles to {css_path}")
        else:
            print(f"[*] Git UI styles already present in {css_path}")
    else:
        print("[-] Could not find omn-go-core.css to patch!")

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    if not os.path.exists(path):
        path = os.path.join("backend", "handlers_web.go")
    content = read_file(path)
    if not content: return

    # Clean up the previous inline CSS injection block
    idx = content.find("` + (func() string {")
    if idx != -1:
        end_idx = content.find("})() + `", idx)
        if end_idx != -1:
            content = content[:idx] + content[end_idx+len("})() + `"):]

    # Dynamically resolve the config variable name
    conf_var = "cfg"
    m = re.search(r'SaveConfig\(([a-zA-Z0-9_]+)\)', content)
    if m:
        conf_var = m.group(1)

    if "Use as Active Server" not in content:
        # Using pure CSS classes defined in omn-go-core.css
        inline_ui = f"""` + (func() string {{
\tgitHTML := "<h3>Git Servers</h3>"
\tfor i, gs := range {conf_var}.GitServers {{
\t\tchecked := ""
\t\tif {conf_var}.ActiveGitIndex == i {{
\t\t\tchecked = "checked"
\t\t}}
\t\tgitHTML += fmt.Sprintf(`
\t\t\t<div class="git-server-card">
\t\t\t\t<label class="git-server-label">
\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s> Use as Active Server (Slot %d)
\t\t\t\t</label>
\t\t\t\t<div class="git-server-grid">
\t\t\t\t\t<input type="text" name="git_name_%d" value="%s" placeholder="Server Name" class="git-server-input">
\t\t\t\t\t<input type="text" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" class="git-server-input">
\t\t\t\t\t<input type="text" name="git_ssh_%d" value="%s" placeholder="SSH Key Path" class="git-server-input">
\t\t\t\t\t<input type="password" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" class="git-server-input">
\t\t\t\t</div>
\t\t\t</div>`, i, checked, i+1, i, gs.Name, i, gs.URL, i, gs.SSHKeyPath, i, gs.Password)
\t}}
\treturn gitHTML
}})() + `"""
        
        parts = content.split(">Save Configuration</button>")
        if len(parts) == 2:
            button_start = parts[0].rfind("<button")
            if button_start != -1:
                before_button = parts[0][:button_start]
                button_tag = parts[0][button_start:]
                content = before_button + inline_ui + "\n\t\t" + button_tag + ">Save Configuration</button>" + parts[1]

    if 'strconv.Atoi(r.FormValue("active_git_index"))' not in content:
        post_logic = f"""
\t\t// Parse Git array
\t\t{conf_var}.ActiveGitIndex, _ = strconv.Atoi(r.FormValue("active_git_index"))
\t\tfor i := 0; i < 5; i++ {{
\t\t\t{conf_var}.GitServers[i].Name = r.FormValue(fmt.Sprintf("git_name_%d", i))
\t\t\t{conf_var}.GitServers[i].URL = r.FormValue(fmt.Sprintf("git_url_%d", i))
\t\t\t{conf_var}.GitServers[i].SSHKeyPath = r.FormValue(fmt.Sprintf("git_ssh_%d", i))
\t\t\t{conf_var}.GitServers[i].Password = r.FormValue(fmt.Sprintf("git_pass_%d", i))
\t\t}}
"""
        content = re.sub(rf'(SaveConfig\({conf_var}\))', post_logic + r'\n\t\t\1', content)

    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_css()
    patch_git_helper()
    patch_handlers_ui()
    print("[*] Update complete. Offline CSS classes successfully injected!")

if __name__ == "__main__":
    main()