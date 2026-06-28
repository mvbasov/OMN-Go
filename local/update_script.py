#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    """Replace *old* with *new* in *path*.  Raise ValueError if *old* missing."""
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version and bump
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)
    new_code = int(new_ver.replace(".", ""))

    # Update version.go
    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    # Update android/app/build.gradle
    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    old_code = int(cur_ver.replace(".", ""))
    gradle = gradle.replace(f"versionCode {old_code}", f"versionCode {new_code}")
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Apply feature patches (with exact whitespace matching)

    # 2a. backend/handlers.go – handleConfig POST: full git server parsing + active index
    old_sync = (
        "\t\tappConfig.GitServers[appConfig.ActiveGitIndex].URL = r.FormValue(\"sync_remote\")\n"
        "\t\tappConfig.GitServers[appConfig.ActiveGitIndex].SSHKeyPath = r.FormValue(\"sync_ssh_key\")\n"
        "\t\tappConfig.GitServers[appConfig.ActiveGitIndex].Password = r.FormValue(\"sync_ssh_passphrase\")"
    )
    new_sync = (
        "\t\t// Apply active git index from radio selection\n"
        "\t\tif idxStr := r.FormValue(\"active_git_index\"); idxStr != \"\" {\n"
        "\t\t\tvar idx int\n"
        "\t\t\tfmt.Sscanf(idxStr, \"%d\", &idx)\n"
        "\t\t\tif idx >= 0 && idx < len(appConfig.GitServers) {\n"
        "\t\t\t\tappConfig.ActiveGitIndex = idx\n"
        "\t\t\t}\n"
        "\t\t}\n"
        "\t\t// Update all 5 git server slots\n"
        "\t\tfor i := 0; i < 5; i++ {\n"
        "\t\t\tname := r.FormValue(fmt.Sprintf(\"git_name_%d\", i))\n"
        "\t\t\turl := r.FormValue(fmt.Sprintf(\"git_url_%d\", i))\n"
        "\t\t\tssh := r.FormValue(fmt.Sprintf(\"git_ssh_%d\", i))\n"
        "\t\t\tpass := r.FormValue(fmt.Sprintf(\"git_pass_%d\", i))\n"
        "\t\t\t// update fields if any non‑empty value is supplied (allows clearing)\n"
        "\t\t\tif name != \"\" || url != \"\" || ssh != \"\" || pass != \"\" {\n"
        "\t\t\t\tappConfig.GitServers[i].Name = name\n"
        "\t\t\t\tappConfig.GitServers[i].URL = url\n"
        "\t\t\t\tappConfig.GitServers[i].SSHKeyPath = ssh\n"
        "\t\t\t\tappConfig.GitServers[i].Password = pass\n"
        "\t\t\t}\n"
        "\t\t}"
    )
    patch_file("backend/handlers.go", old_sync, new_sync)

    # 2b. Add name attributes to config form fields (with leading spaces)
    patch_file("backend/handlers.go",
        '            <input type="number" id="cfgPort" value="%d" class="config-input" required />',
        '            <input type="number" id="cfgPort" name="server_port" value="%d" class="config-input" required />')
    patch_file("backend/handlers.go",
        '            <input type="password" id="cfgAdminPwd" value="%s" class="config-input" required />',
        '            <input type="password" id="cfgAdminPwd" name="admin_password" value="%s" class="config-input" required />')
    patch_file("backend/handlers.go",
        '            <input type="password" id="cfgGuestPwd" value="%s" class="config-input" required />',
        '            <input type="password" id="cfgGuestPwd" name="guest_password" value="%s" class="config-input" required />')
    patch_file("backend/handlers.go",
        '            <input type="text" id="cfgAuthor" value="%s" class="config-input" />',
        '            <input type="text" id="cfgAuthor" name="author" value="%s" class="config-input" />')
    patch_file("backend/handlers.go",
        '            <input type="checkbox" id="cfgInternalEd" %s />',
        '            <input type="checkbox" id="cfgInternalEd" name="use_internal_editor" value="true" %s />')
    patch_file("backend/handlers.go",
        '            <input type="text" id="cfgDesktopExtCmd" value="%s" class="config-input" />',
        '            <input type="text" id="cfgDesktopExtCmd" name="desktop_ext_cmd" value="%s" class="config-input" />')

    # 2c. Fix git server card inputs – add name attributes and update Sprintf args
    old_git_block = (
        '\t\tgitHTML += fmt.Sprintf(`\n'
        '\t\t\t<div style="border: 1px solid #ccc; padding: 15px; margin-bottom: 15px; border-radius: 6px; background: #ffffff; color: black; box-shadow: 0 2px 4px rgba(0,0,0,0.05);">\n'
        '\t\t\t\t<label style="font-weight: bold; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 16px; color: #2c3e50;">\n'
        '\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s style="transform: scale(1.2);"> Use as Active Server (Slot %d)\n'
        '\t\t\t\t</label>\n'
        '\t\t\t\t<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">\n'
        '\t\t\t\t\t<input type="text" id="git_name_%d" value="%s" placeholder="Server Name" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_ssh_%d" value="%s" placeholder="SSH Key Path" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="password" id="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t</div>\n'
        '\t\t\t</div>`, i, checked, i+1, i, gs.Name, i, gs.URL, i, gs.SSHKeyPath, i, gs.Password)'
    )
    new_git_block = (
        '\t\tgitHTML += fmt.Sprintf(`\n'
        '\t\t\t<div style="border: 1px solid #ccc; padding: 15px; margin-bottom: 15px; border-radius: 6px; background: #ffffff; color: black; box-shadow: 0 2px 4px rgba(0,0,0,0.05);">\n'
        '\t\t\t\t<label style="font-weight: bold; display: flex; align-items: center; gap: 8px; margin-bottom: 10px; font-size: 16px; color: #2c3e50;">\n'
        '\t\t\t\t\t<input type="radio" name="active_git_index" value="%d" %s style="transform: scale(1.2);"> Use as Active Server (Slot %d)\n'
        '\t\t\t\t</label>\n'
        '\t\t\t\t<div style="display: grid; grid-template-columns: 1fr 1fr; gap: 10px;">\n'
        '\t\t\t\t\t<input type="text" id="git_name_%d" name="git_name_%d" value="%s" placeholder="Server Name" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_url_%d" name="git_url_%d" value="%s" placeholder="Git URL (git@...)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="text" id="git_ssh_%d" name="git_ssh_%d" value="%s" placeholder="SSH Key Path" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t\t<input type="password" id="git_pass_%d" name="git_pass_%d" value="%s" placeholder="Key Password (Optional)" style="padding: 8px; border: 1px solid #ccc; border-radius: 4px; font-size: 14px;">\n'
        '\t\t\t\t</div>\n'
        '\t\t\t</div>`, i, checked, i+1, i, i, gs.Name, i, i, gs.URL, i, i, gs.SSHKeyPath, i, i, gs.Password)'
    )
    patch_file("backend/handlers.go", old_git_block, new_git_block)

    # 2d. Replace complex button onclick with simple saveConfig() call
    old_button_onclick = '''<button type="button" class="btn-primary" onclick="
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
            ">Save Configuration</button>'''
    new_button_onclick = '<button type="button" class="btn-primary" onclick="saveConfig()">Save Configuration</button>'
    patch_file("backend/handlers.go", old_button_onclick, new_button_onclick)

    # 2e. Add saveConfig() to omn-go-sse.js
    patch_file("backend/frontend/html/js/omn-go-sse.js",
        "window.syncAction = syncAction;",
        "window.syncAction = syncAction;\n\n"
        "    window.saveConfig = async function() {\n"
        "        const form = document.getElementById('configForm');\n"
        "        if (!form) { alert('Config form not found'); return; }\n"
        "        const fd = new FormData(form);\n"
        "        try {\n"
        "            const res = await fetch('/api/config', { method: 'POST', body: fd });\n"
        "            if (res.ok) {\n"
        "                alert('Configuration saved. Reloading...');\n"
        "                window.location.reload();\n"
        "            } else {\n"
        "                let msg = await res.text();\n"
        "                alert('Failed to save configuration: ' + msg);\n"
        "            }\n"
        "        } catch (e) {\n"
        "            alert('Network error: ' + e);\n"
        "        }\n"
        "    };"
    )

    # 3. Print Git commit message
    commit_msg = (
        "fix(config): repair broken config initialisation and page handling\n\n"
        "- Add name attributes to all config form fields (including git servers)\n"
        "- Implement full 5‑slot git server parsing in POST handler\n"
        "- Replace buggy fetch injection with a dedicated saveConfig() JS function\n"
        "- Ensure config page save button works correctly\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()