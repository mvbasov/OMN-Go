import os
import re
import sys

VERSION = "1.5.15"
VERSION_CODE = "10515"

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

    # Ensure assignment mismatch is handled cleanly
    content = re.sub(r'[a-zA-Z0-9_]+\s*,\s*err\s*:=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err := \1.AddWithOptions', content)
    content = re.sub(r'_\s*,\s*err\s*:=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err := \1.AddWithOptions', content)
    content = re.sub(r'[a-zA-Z0-9_]+\s*,\s*err\s*=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err = \1.AddWithOptions', content)
    content = re.sub(r'_\s*,\s*err\s*=\s*([a-zA-Z0-9_]+)\.AddWithOptions', r'err = \1.AddWithOptions', content)

    write_file(path, content)

def patch_config_struct():
    path = os.path.join("backend", "config.go")
    content = read_file(path)
    if not content:
        print("[-] Critical Error: backend/config.go not found.")
        return
    
    # Inject GitServerConfig struct safely
    if "type GitServerConfig struct" not in content:
        git_struct = """
type GitServerConfig struct {
\tName       string `json:"name"`
\tURL        string `json:"url"`
\tSSHKeyPath string `json:"ssh_key_path"`
\tPassword   string `json:"password"`
}
"""
        content = re.sub(r'(type Config struct\s*\{)', git_struct + r'\n\1', content)

    # Add Git array fields
    if "GitServers" not in content:
        content = re.sub(r'(type Config struct\s*\{)', 
                         r'\1\n\tActiveGitIndex int                `json:"active_git_index"`\n\tGitServers     []GitServerConfig  `json:"git_servers"`', 
                         content)

    # Ensure empty slots are initialized specifically for func loadConfig 
    if "for len(appConfig.GitServers) < 5" not in content:
        init_logic = """
\t// [OMN-Go 1.5.15] Ensure 5 Git server slots exist dynamically
\tfor len(appConfig.GitServers) < 5 {
\t\tappConfig.GitServers = append(appConfig.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)})
\t}
"""
        # Hook exactly right after json.Unmarshal in the else block
        content = re.sub(r'(json\.Unmarshal\([^)]+\)\n[ \t]*\})', r'\1\n' + init_logic, content)
        
        # Ensure fmt is imported for Sprintf
        if '"fmt"' not in content:
            content = re.sub(r'(import\s*\()', r'\1\n\t"fmt"', content)
                
    write_file(path, content)

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    content = read_file(path)
    if not content: return

    # Clean up broken strconv if any
    content = re.sub(r'\n\s*"strconv"', '', content)
    content = re.sub(r'import\s+"strconv"\s*\n', '', content)

    # Extract the Config HTML generator
    btn_idx = content.find(">Save Configuration</button>")
    if btn_idx == -1: return
    
    func_start = content.rfind("func ", 0, btn_idx)
    func_end = content.find("\n}\n", btn_idx)
    if func_end == -1: func_end = len(content)

    func_body = content[func_start:func_end]

    # Clean any old injections to prevent duplication
    idx = func_body.find("` + (func() string {")
    if idx != -1:
        end_idx = func_body.find("})() + `", idx)
        if end_idx != -1:
            func_body = func_body[:idx] + func_body[end_idx+len("})() + `"):]

    # Resolve config variable (defaults to appConfig in your handlers.go)
    conf_var = "appConfig"
    m_var = re.search(r'\b([a-zA-Z0-9_]+)\.(?:Author|Port|Theme|GitServers|ActiveGitIndex|GitPassword)\b', func_body)
    if m_var:
        conf_var = m_var.group(1)

    # Inject the HTML UI paired with the highly robust JS payload interceptor
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
\t
\t// Automatically pack the Git settings into the JSON fetch payload so the Go backend catches it natively!
\tgitHTML += `<script>
\t(function() {{
\t\tfunction injectGitData(bodyStr) {{
\t\t\ttry {{
\t\t\t\tlet parsed = JSON.parse(bodyStr);
\t\t\t\tif (parsed) {{
\t\t\t\t\tlet activeIndex = document.querySelector('input[name="active_git_index"]:checked');
\t\t\t\t\tparsed.active_git_index = activeIndex ? parseInt(activeIndex.value) : 0;
\t\t\t\t\tparsed.git_servers = [];
\t\t\t\t\tfor(let i=0; i<5; i++) {{
\t\t\t\t\t\tparsed.git_servers.push({{
\t\t\t\t\t\t\tname: document.querySelector('input[name="git_name_'+i+'"]')?.value || '',
\t\t\t\t\t\t\turl: document.querySelector('input[name="git_url_'+i+'"]')?.value || '',
\t\t\t\t\t\t\tssh_key_path: document.querySelector('input[name="git_ssh_'+i+'"]')?.value || '',
\t\t\t\t\t\t\tpassword: document.querySelector('input[name="git_pass_'+i+'"]')?.value || ''
\t\t\t\t\t\t}});
\t\t\t\t\t}}
\t\t\t\t\treturn JSON.stringify(parsed);
\t\t\t\t}}
\t\t\t}} catch(e) {{}}
\t\t\treturn bodyStr;
\t\t}}
\t\t
\t\t// Hook native fetch and XHR to seamlessly append the data
\t\tconst origFetch = window.fetch;
\t\twindow.fetch = function() {{
\t\t\tif (arguments[1] && arguments[1].method && arguments[1].method.toUpperCase() === 'POST' && typeof arguments[1].body === 'string') {{
\t\t\t\targuments[1].body = injectGitData(arguments[1].body);
\t\t\t}}
\t\t\treturn origFetch.apply(this, arguments);
\t\t}};
\t\tconst origSend = XMLHttpRequest.prototype.send;
\t\tXMLHttpRequest.prototype.send = function(body) {{
\t\t\tif (typeof body === 'string') {{ body = injectGitData(body); }}
\t\t\treturn origSend.call(this, body);
\t\t}};
\t}})();
\t</script>`
\treturn gitHTML
}})() + `'''

    parts = func_body.split(">Save Configuration</button>")
    if len(parts) == 2:
        button_start = parts[0].rfind("<button")
        if button_start != -1:
            before_button = parts[0][:button_start]
            button_tag = parts[0][button_start:]
            func_body = before_button + inline_ui + "\n\t\t" + button_tag + ">Save Configuration</button>" + parts[1]

    # Clean old Go POST regex parsing if it accidentally stayed in the code from 1.5.13
    func_body = re.sub(r'// Parse Git array safely.*?\}', '', func_body, flags=re.DOTALL)

    content = content[:func_start] + func_body + content[func_end:]
    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_config_struct()
    patch_git_helper()
    patch_handlers_ui()
    print("[*] Update complete. Array limits forcefully initialized! You may now safely compile!")

if __name__ == "__main__":
    main()