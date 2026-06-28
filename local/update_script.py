import os
import re
import sys
import base64

VERSION = "1.5.19"
VERSION_CODE = "10519"

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

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    content = read_file(path)
    if not content: return

    # The robust JS payload interceptor (Base64'd to bypass innerHTML script block rules!)
    js_payload = """
    if (!window.__gitHooked) {
        window.__gitHooked = true;
        const origFetch = window.fetch;
        window.fetch = function() {
            if (arguments[1] && arguments[1].method && arguments[1].method.toUpperCase() === 'POST' && typeof arguments[1].body === 'string') {
                try {
                    let parsed = JSON.parse(arguments[1].body);
                    if (parsed) {
                        let activeIndex = document.querySelector('input[name="active_git_index"]:checked');
                        parsed.active_git_index = activeIndex ? parseInt(activeIndex.value) : 0;
                        parsed.git_servers = [];
                        for(let i=0; i<5; i++) {
                            parsed.git_servers.push({
                                name: document.querySelector('input[name="git_name_'+i+'"]')?.value || '',
                                url: document.querySelector('input[name="git_url_'+i+'"]')?.value || '',
                                ssh_key_path: document.querySelector('input[name="git_ssh_'+i+'"]')?.value || '',
                                password: document.querySelector('input[name="git_pass_'+i+'"]')?.value || ''
                            });
                        }
                        arguments[1].body = JSON.stringify(parsed);
                    }
                } catch(e) {}
            }
            return origFetch.apply(this, arguments);
        };
        const origSend = XMLHttpRequest.prototype.send;
        XMLHttpRequest.prototype.send = function(body) {
            if (typeof body === 'string') {
                try {
                    let parsed = JSON.parse(body);
                    if (parsed) {
                        let activeIndex = document.querySelector('input[name="active_git_index"]:checked');
                        parsed.active_git_index = activeIndex ? parseInt(activeIndex.value) : 0;
                        parsed.git_servers = [];
                        for(let i=0; i<5; i++) {
                            parsed.git_servers.push({
                                name: document.querySelector('input[name="git_name_'+i+'"]')?.value || '',
                                url: document.querySelector('input[name="git_url_'+i+'"]')?.value || '',
                                ssh_key_path: document.querySelector('input[name="git_ssh_'+i+'"]')?.value || '',
                                password: document.querySelector('input[name="git_pass_'+i+'"]')?.value || ''
                            });
                        }
                        body = JSON.stringify(parsed);
                    }
                } catch(e) {}
            }
            return origSend.call(this, body);
        };
    }
    """
    # Base64 encode the JS so it's impervious to Go/HTML formatting quirks
    b64_js = base64.b64encode(js_payload.encode('utf-8')).decode('utf-8')

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

    new_func = f"""func getConfigPageBody() string {{
\t// [OMN-Go 1.5.19] Self-Healing Array Enforcement
\t// Guarantees the UI loop executes even if the JSON payload previously wiped the variable!
\tfor len(appConfig.GitServers) < 5 {{
\t\tappConfig.GitServers = append(appConfig.GitServers, GitServerConfig{{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)}})
\t}}

\tcheckedStr := ""
\tif appConfig.UseInternalEd {{
\t\tcheckedStr = "checked"
\t}}

\tgitHTML := "<h3>Git Servers</h3>"
\tfor i, gs := range appConfig.GitServers {{
\t\tchecked := ""
\t\tif appConfig.ActiveGitIndex == i {{
\t\t\tchecked = "checked"
\t\t}}
\t\tgitHTML += fmt.Sprintf(`
\t\t\t<div style="border: 1px solid #ccc; padding: 10px; margin-bottom: 10px; border-radius: 4px; background: #f9f9f9; color: black;">
\t\t\t\t<label style="font-weight: bold; display: flex; align-items: center; gap: 8px;">
\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s> Use as Active Server (Slot %d)
\t\t\t\t</label>
\t\t\t\t<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px; margin-top: 10px;">
\t\t\t\t\t<input type="text" name="git_name_%d" value="%s" placeholder="Server Name" style="padding: 5px; border: 1px solid #ccc; border-radius: 3px;">
\t\t\t\t\t<input type="text" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 5px; border: 1px solid #ccc; border-radius: 3px;">
\t\t\t\t\t<input type="text" name="git_ssh_%d" value="%s" placeholder="SSH Key Path" style="padding: 5px; border: 1px solid #ccc; border-radius: 3px;">
\t\t\t\t\t<input type="password" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 5px; border: 1px solid #ccc; border-radius: 3px;">
\t\t\t\t</div>
\t\t\t</div>`, i, checked, i+1, i, gs.Name, i, gs.URL, i, gs.SSHKeyPath, i, gs.Password)
\t}}

\t// Use Base64 transparent image trick to flawlessly execute Javascript inside dynamic .innerHTML
\tgitHTML += `<img src="data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7" onload="eval(atob('{b64_js}'))" style="display:none;" />`

\treturn fmt.Sprintf(`
<div class="config-panel">
    <h2 class="config-title">Configuration Dashboard</h2>
    <form id="configForm" onsubmit="saveConfig(event)" class="config-form">
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

        <!-- Legacy Safeguard so frontend JS payload extraction doesn't crash on null! -->
        <input type="hidden" id="cfgSyncRemote" value="" />
        <input type="hidden" id="cfgSyncSSHKey" value="" />
        <input type="hidden" id="cfgSyncSSHPassphrase" value="" />

        %s

        <div class="config-field" style="margin-top: 20px;">
            <button type="submit" class="btn-primary">Save Configuration</button>
        </div>
    </form>
</div>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author, checkedStr, appConfig.DesktopExtCmd, gitHTML)
}}"""
    content = content[:start_idx] + new_func + content[end_idx:]
    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_handlers_ui()
    print("[*] Update complete. The DOM innerHTML security bypass is active and the array is self-healing!")

if __name__ == "__main__":
    main()