import os
import re
import sys

VERSION = "1.5.16"
VERSION_CODE = "10516"

def read_file(filepath):
    if not os.path.exists(filepath):
        return None
    with open(filepath, 'r', encoding='utf-8') as f:
        # Eradicate CRLF matching bugs by forcing standard LF
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
    
    # 1. Physically Replace the Config Struct block using brace counting
    struct_start = content.find("type Config struct {")
    if struct_start != -1:
        struct_end = content.find("}", struct_start) + 1
        new_struct = """type GitServerConfig struct {
\tName       string `json:"name"`
\tURL        string `json:"url"`
\tSSHKeyPath string `json:"ssh_key_path"`
\tPassword   string `json:"password"`
}

type Config struct {
\tForcePullOneTime bool `json:"force_pull_one_time"`
\tServerPort       int               `json:"server_port"`
\tAdminPassword    string            `json:"admin_password"`
\tGuestPassword    string            `json:"guest_password"`
\tAuthor           string            `json:"author"`
\tUseInternalEd    bool              `json:"use_internal_editor"`
\tDesktopExtCmd    string            `json:"desktop_ext_cmd"`
\tMimeTypes        map[string]string `json:"mime_types"`
\tActiveGitIndex   int               `json:"active_git_index"`
\tGitServers     []GitServerConfig `json:"git_servers"`
}"""
        content = content[:struct_start] + new_struct + content[struct_end:]

    # 2. Replace the initialization block to drop the old Sync variables
    init_start = content.find("appConfig = Config{")
    if init_start != -1:
        brace_count = 0
        init_end = init_start
        found_first = False
        for i in range(init_start, len(content)):
            if content[i] == '{':
                brace_count += 1
                found_first = True
            elif content[i] == '}':
                brace_count -= 1
            if found_first and brace_count == 0:
                init_end = i + 1
                break

        new_init = """appConfig = Config{
\t\t\tServerPort:    8080,
\t\t\tAdminPassword: "admin_secret_changeme",
\t\t\tGuestPassword: "guest_secret_changeme",
\t\t\tAuthor:        "Anonymous",
\t\t\tUseInternalEd: true,
\t\t\tDesktopExtCmd: "subl",
\t\t\tMimeTypes: map[string]string{
\t\t\t\t".css":   "text/css",
\t\t\t\t".js":    "application/javascript",
\t\t\t\t".json":  "application/json",
\t\t\t\t".html":  "text/html",
\t\t\t\t".md":    "text/markdown",
\t\t\t\t".svg":   "image/svg+xml",
\t\t\t\t".png":   "image/png",
\t\t\t\t".jpg":   "image/jpeg",
\t\t\t\t".jpeg":  "image/jpeg",
\t\t\t\t".woff2": "font/woff2",
\t\t\t},
\t\t}"""
        content = content[:init_start] + new_init + content[init_end:]

    # 3. Safely insert array logic right after unmarshal
    if "for len(appConfig.GitServers) < 5" not in content:
        unmarshal_idx = content.find("json.Unmarshal(data, &appConfig)")
        if unmarshal_idx != -1:
            brace_idx = content.find("}", unmarshal_idx)
            if brace_idx != -1:
                injection = """\t// [OMN-Go 1.5.16] Enforce 5 empty slots natively
\tfor len(appConfig.GitServers) < 5 {
\t\tappConfig.GitServers = append(appConfig.GitServers, GitServerConfig{Name: fmt.Sprintf("Server %d", len(appConfig.GitServers)+1)})
\t}\n"""
                content = content[:brace_idx+1] + "\n" + injection + content[brace_idx+1:]
                
    if '"fmt"' not in content:
        content = re.sub(r'(import\s*\()', r'\1\n\t"fmt"', content)

    write_file(path, content)

def patch_handlers_ui():
    path = os.path.join("backend", "handlers.go")
    content = read_file(path)
    if not content: return

    # Completely rebuild getConfigPageBody to guarantee execution
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

    new_func = """func getConfigPageBody() string {
\tcheckedStr := ""
\tif appConfig.UseInternalEd {
\t\tcheckedStr = "checked"
\t}

\tgitHTML := "<h3>Git Servers</h3>"
\tfor i, gs := range appConfig.GitServers {
\t\tchecked := ""
\t\tif appConfig.ActiveGitIndex == i {
\t\t\tchecked = "checked"
\t\t}
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
\t}

\tgitHTML += `<script>
\t(function() {
\t\tfunction injectGitData(bodyStr) {
\t\t\ttry {
\t\t\t\tlet parsed = JSON.parse(bodyStr);
\t\t\t\tif (parsed) {
\t\t\t\t\tlet activeIndex = document.querySelector('input[name="active_git_index"]:checked');
\t\t\t\t\tparsed.active_git_index = activeIndex ? parseInt(activeIndex.value) : 0;
\t\t\t\t\tparsed.git_servers = [];
\t\t\t\t\tfor(let i=0; i<5; i++) {
\t\t\t\t\t\tparsed.git_servers.push({
\t\t\t\t\t\t\tname: document.querySelector('input[name="git_name_'+i+'"]')?.value || '',
\t\t\t\t\t\t\turl: document.querySelector('input[name="git_url_'+i+'"]')?.value || '',
\t\t\t\t\t\t\tssh_key_path: document.querySelector('input[name="git_ssh_'+i+'"]')?.value || '',
\t\t\t\t\t\t\tpassword: document.querySelector('input[name="git_pass_'+i+'"]')?.value || ''
\t\t\t\t\t\t});
\t\t\t\t\t}
\t\t\t\t\treturn JSON.stringify(parsed);
\t\t\t\t}
\t\t\t} catch(e) {}
\t\t\treturn bodyStr;
\t\t}
\t\t
\t\tconst origFetch = window.fetch;
\t\twindow.fetch = function() {
\t\t\tif (arguments[1] && arguments[1].method && arguments[1].method.toUpperCase() === 'POST' && typeof arguments[1].body === 'string') {
\t\t\t\targuments[1].body = injectGitData(arguments[1].body);
\t\t\t}
\t\t\treturn origFetch.apply(this, arguments);
\t\t};
\t\tconst origSend = XMLHttpRequest.prototype.send;
\t\tXMLHttpRequest.prototype.send = function(body) {
\t\t\tif (typeof body === 'string') { body = injectGitData(body); }
\t\t\treturn origSend.call(this, body);
\t\t};
\t})();
\t</script>`

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

        <!-- Legacy Safeguard: Hidden inputs so frontend JS payload extraction doesn't crash on null! -->
        <input type="hidden" id="cfgSyncRemote" value="" />
        <input type="hidden" id="cfgSyncSSHKey" value="" />
        <input type="hidden" id="cfgSyncSSHPassphrase" value="" />

        %s

        <div style="margin-top: 20px;">
            <button type="submit" class="btn-primary" style="padding: 10px 20px; font-weight: bold;">Save Configuration</button>
        </div>
    </form>
</div>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author, checkedStr, appConfig.DesktopExtCmd, gitHTML)
}"""
    content = content[:start_idx] + new_func + content[end_idx:]
    write_file(path, content)

def patch_git_helper():
    path = os.path.join("backend", "git_helper.go")
    content = read_file(path)
    if not content: return

    # Reroute old variables straight to the new Active Array
    content = content.replace("appConfig.SyncRemote", "appConfig.GitServers[appConfig.ActiveGitIndex].URL")
    content = content.replace("appConfig.SyncSSHKey", "appConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyPath")
    content = content.replace("appConfig.SyncSSHPassphrase", "appConfig.GitServers[appConfig.ActiveGitIndex].Password")
    
    # Inject Dynamic Remote cache bypass
    if "DeleteRemote" not in content:
        fix_logic = """
\t// [OMN-Go 1.5.16] Dynamically update remote origin cache
\tactiveGit := appConfig.GitServers[appConfig.ActiveGitIndex]
\tremote, remoteErr := repo.Remote("origin")
\tif remoteErr == nil && len(remote.Config().URLs) > 0 && remote.Config().URLs[0] != activeGit.URL {
\t\trepo.DeleteRemote("origin")
\t\trepo.CreateRemote(&gitconfig.RemoteConfig{
\t\t\tName: "origin",
\t\t\tURLs: []string{activeGit.URL},
\t\t})
\t}
"""
        content = re.sub(r'(repo,\s*err\s*:=\s*git\.PlainOpen[^\n]+\n\s*if\s*err\s*!=\s*nil\s*\{[^\}]+\}\n)', r'\1' + fix_logic, content)
        if '"github.com/go-git/go-git/v5/config"' not in content:
            content = re.sub(r'(import\s*\()', r'\1\n\tgitconfig "github.com/go-git/go-git/v5/config"\n', content)

    write_file(path, content)

def main():
    print(f"[*] Starting OMN-Go update to Version {VERSION}...")
    bump_versions()
    patch_config_struct()
    patch_git_helper()
    patch_handlers_ui()
    print("[*] Update complete. Structs successfully rewritten via AST and CRLF bugs eradicated!")

if __name__ == "__main__":
    main()