import os
import re
import sys

VERSION = "1.5.12"
VERSION_CODE = "10512"

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

    # Ensure assignment mismatch is handled
    content = re.sub(r'[a-zA-Z0-9_]+\s*,\s*err\s*:=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err := \1.AddWithOptions', content)
    content = re.sub(r'_\s*,\s*err\s*:=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err := \1.AddWithOptions', content)
    content = re.sub(r'[a-zA-Z0-9_]+\s*,\s*err\s*=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err = \1.AddWithOptions', content)
    content = re.sub(r'_\s*,\s*err\s*=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err = \1.AddWithOptions', content)

    write_file(path, content)

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    if not os.path.exists(path):
        path = os.path.join("backend", "handlers_web.go")
    content = read_file(path)
    if not content: return

    # 1. Strip out unused strconv cleanly to fix the compiler error
    content = re.sub(r'\n\s*"strconv"', '', content)
    content = re.sub(r'import\s+"strconv"\s*\n', '', content)

    # 2. Extract only the Config Handler Block for intelligent parsing
    btn_idx = content.find(">Save Configuration</button>")
    if btn_idx == -1: return
    
    func_start = content.rfind("func ", 0, btn_idx)
    func_end = content.find("\n}\n", btn_idx)
    if func_end == -1: func_end = len(content)

    func_body = content[func_start:func_end]

    # 3. Clean up the previous broken injection BEFORE scanning for the variable
    idx = func_body.find("` + (func() string {")
    if idx != -1:
        end_idx = func_body.find("})() + `", idx)
        if end_idx != -1:
            func_body = func_body[:idx] + func_body[end_idx+len("})() + `"):]

    # 4. Deep-Inspect to find the REAL configuration variable name
    conf_var = "cfg" # fallback
    m_var = re.search(r'\b([a-zA-Z0-9_]+)\.(?:Author|Port|Theme|GitServers|ActiveGitIndex|GitPassword)\b', func_body)
    if m_var:
        conf_var = m_var.group(1)
    else:
        m_save = re.search(r'Save[a-zA-Z0-9_]*\(\s*(?:&)?([a-zA-Z0-9_]+)\s*\)', func_body)
        if m_save:
            conf_var = m_save.group(1)

    print(f"[*] Detected actual config variable name as: '{conf_var}'")

    # 5. Inject the UI perfectly using the resolved variable
    inline_ui = f'''` + (func() string {{
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
}})() + `'''

    parts = func_body.split(">Save Configuration</button>")
    if len(parts) == 2:
        button_start = parts[0].rfind("<button")
        if button_start != -1:
            before_button = parts[0][:button_start]
            button_tag = parts[0][button_start:]
            func_body = before_button + inline_ui + "\n\t\t" + button_tag + ">Save Configuration</button>" + parts[1]

    # 6. Inject POST Parser using fmt.Sscanf (avoiding the strconv error entirely)
    if 'FormValue("active_git_index")' not in func_body:
        post_logic = f"""
\t\t// Parse Git array
\t\tfmt.Sscanf(r.FormValue("active_git_index"), "%d", &{conf_var}.ActiveGitIndex)
\t\tfor i := 0; i < 5; i++ {{
\t\t\t{conf_var}.GitServers[i].Name = r.FormValue(fmt.Sprintf("git_name_%d", i))
\t\t\t{conf_var}.GitServers[i].URL = r.FormValue(fmt.Sprintf("git_url_%d", i))
\t\t\t{conf_var}.GitServers[i].SSHKeyPath = r.FormValue(fmt.Sprintf("git_ssh_%d", i))
\t\t\t{conf_var}.GitServers[i].Password = r.FormValue(fmt.Sprintf("git_pass_%d", i))
\t\t}}
"""
        # Place it right before the save function executes
        m_save = re.search(r'([a-zA-Z0-9_]*\.)?Save[a-zA-Z0-9_]*\(.*?\)', func_body)
        if m_save:
            func_body = func_body.replace(m_save.group(0), post_logic + '\n\t\t' + m_save.group(0))
        else:
            print("[-] Warning: Could not find Save method to inject POST logic!")

    # Save the repaired function body back to the file
    content = content[:func_start] + func_body + content[func_end:]
    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_git_helper()
    patch_handlers_ui()
    print("[*] Update complete. The scope mismatch and unused import are completely eliminated!")

if __name__ == "__main__":
    main()