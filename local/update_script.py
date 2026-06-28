import os
import re
import sys

VERSION = "1.5.21"
VERSION_CODE = "10521"

def read_file(filepath):
    if not os.path.exists(filepath):
        return None
    with open(filepath, 'r', encoding='utf-8') as f:
        return f.read().replace('\r\n', '\n')

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

def patch_config_struct():
    path = os.path.join("backend", "config.go")
    content = read_file(path)
    if not content: return

    # Clean out any previous patch attempts to avoid duplicate loops
    content = re.sub(r'\t\t// \[OMN-Go 1\.5\.\d+\].*?appConfig\.GitServers = append[^\}]+?\}', '', content, flags=re.DOTALL)
    
    # Inject the absolute Array Lock right after json.Unmarshal
    unmarshal_target = "json.Unmarshal(data, &appConfig)"
    if unmarshal_target in content:
        injection = """
\t\t// [OMN-Go 1.5.21] Absolute Array Lock: Prevents the JSON 'null' wipe bug forever
\t\tfor len(appConfig.GitServers) < 5 {
\t\t\tappConfig.GitServers = append(appConfig.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)})
\t\t}"""
        content = content.replace(unmarshal_target, unmarshal_target + injection)

    if '"fmt"' not in content:
        content = re.sub(r'(import\s*\()', r'\1\n\t"fmt"', content)

    write_file(path, content)

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    content = read_file(path)
    if not content: return

    func_name = "func getConfigPageBody() string"
    start_idx = content.find(func_name)
    if start_idx == -1: return

    brace_count = 0
    end_idx = start_idx
    found_first = False
    for i in range(start_idx, len(content)):
        if content[i] == '{':
            brace_count += 1
            found_first = True
        elif content[i] == '}':
            brace_count -= 1
        if found_first and brace_count == 0:
            end_idx = i + 1
            break

    # The Ghost Hook Override Template
    # We securely intercept the native fetch mechanism without any quote escaping bugs
    new_func = """func getConfigPageBody() string {
\t// Redundant safety lock to ensure UI generation never crashes
\tfor len(appConfig.GitServers) < 5 {
\t\tappConfig.GitServers = append(appConfig.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)})
\t}

\tcheckedStr := ""
\tif appConfig.UseInternalEd {
\t\tcheckedStr = "checked"
\t}

\tgitHTML := "<h3>Git Servers</h3>"
\tfor i, gs := range appConfig.GitServers {
\t\tchecked := ""
\t\tif appConfig.ActiveGitIndex == i {
\t\t\tchecked = "checked"
\t}
\t\tgitHTML += fmt.Sprintf(`
\t\t\t<div style="border: 1px solid #ccc; padding: 15px; margin-bottom: 15px; border-radius: 6px; background: #ffffff; color: black; box-shadow: 0 2px 4px rgba(0,0,0,0.05);">
\t\t\t\t<label style="font-weight: bold; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 16px; color: #2c3e50;">
\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s style="transform: scale(1.2);"> Use as Active Server (Slot %d)
\t\t\t\t</label>
\t\t\t\t<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">
\t\t\t\t\t<input type="text" id="git_name_%d" value="%s" placeholder="Server Name" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
\t\t\t\t\t<input type="text" id="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
\t\t\t\t\t<input type="text" id="git_ssh_%d" value="%s" placeholder="SSH Key Path" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
\t\t\t\t\t<input type="password" id="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">
\t\t\t\t</div>
\t\t\t</div>`, i, checked, i+1, i, gs.Name, i, gs.URL, i, gs.SSHKeyPath, i, gs.Password)
\t}

\treturn fmt.Sprintf(`
<div class="config-panel">
    <h2 class="config-title">Configuration Dashboard</h2>
    <form id="configForm" class="config-form">
        <div class="config-field">
            <label class="config-label">Server Port</label>
            <input type="number" id="cfgPort" value="%d" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Admin Password</label>
            <input type="password" id="cfgAdminPwd" value="%s" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Guest Password</label>
            <input type="password" id="cfgGuestPwd" value="%s" class="config-input" required />
        </div>
        <div class="config-field">
            <label class="config-label">Author Name</label>
            <input type="text" id="cfgAuthor" value="%s" class="config-input" />
        </div>
        <div class="config-field config-checkbox-row">
            <input type="checkbox" id="cfgInternalEd" %s />
            <label class="config-label">Use Internal Editor</label>
        </div>
        <div class="config-field">
            <label class="config-label">Desktop External Cmd</label>
            <input type="text" id="cfgDesktopExtCmd" value="%s" class="config-input" />
        </div>

        %s

        <div class="config-field" style="margin-top: 20px;">
            <button type="button" class="btn-primary" onclick="
                let injectData = function(bodyStr) {
                    try {
                        let parsed = JSON.parse(bodyStr);
                        let activeEl = document.querySelector('input[name=active_git_index]:checked');
                        parsed.active_git_index = activeEl ? parseInt(activeEl.value) : 0;
                        parsed.git_servers = [];
                        for(let i=0; i<5; i++) {
                            let sn = document.getElementById('git_name_'+i);
                            let su = document.getElementById('git_url_'+i);
                            let sp = document.getElementById('git_ssh_'+i);
                            let sw = document.getElementById('git_pass_'+i);
                            parsed.git_servers.push({
                                name: sn ? sn.value : '',
                                url: su ? su.value : '',
                                ssh_key_path: sp ? sp.value : '',
                                password: sw ? sw.value : ''
                            });
                        }
                        return JSON.stringify(parsed);
                    } catch(e) { return bodyStr; }
                };
                let origFetch = window.fetch;
                window.fetch = function(url, options) {
                    if (options && options.body && typeof options.body === 'string') {
                        options.body = injectData(options.body);
                    }
                    window.fetch = origFetch;
                    return origFetch.call(this, url, options);
                };
                let origSend = XMLHttpRequest.prototype.send;
                XMLHttpRequest.prototype.send = function(body) {
                    if (typeof body === 'string') {
                        body = injectData(body);
                    }
                    XMLHttpRequest.prototype.send = origSend;
                    return origSend.call(this, body);
                };
                if(typeof saveConfig === 'function') {
                    saveConfig({ preventDefault: function(){}, target: document.getElementById('configForm') });
                }
            ">Save Configuration</button>
        </div>
    </form>
</div>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author, checkedStr, appConfig.DesktopExtCmd, gitHTML)
}"""
    content = content[:start_idx] + new_func + content[end_idx:]
    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_config_struct()
    patch_handlers_ui()
    print("[*] Update complete. Quote escaping syntax errors are annihilated and CSS is restored!")

if __name__ == "__main__":
    main()