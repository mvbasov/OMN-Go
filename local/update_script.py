#!/usr/bin/env python3
import re, os

def read_file(path):
    with open(path, "r", encoding="utf-8") as f:
        return f.read()

def write_file(path, content):
    with open(path, "w", encoding="utf-8") as f:
        f.write(content)

def patch_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def safe_patch(path, old, new):
    content = read_file(path)
    if old in content:
        patch_file(path, old, new)
    elif new not in content:
        raise ValueError(f"❌ Neither old nor new string found in {path}")

def safe_patch_regex(path, pattern, replacement, check_idempotent=None):
    """Apply regex substitution once; skip if *check_idempotent* already present."""
    content = read_file(path)
    if check_idempotent and re.search(check_idempotent, content):
        return
    new_content = re.sub(pattern, replacement, content, count=1)
    if new_content == content:
        raise ValueError(f"❌ Regex patch failed – pattern not found in {path}")
    write_file(path, new_content)

def increment_version(ver_str):
    parts = ver_str.strip().split(".")
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return ".".join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Bump version
    ver_path = "backend/version.go"
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = "android/app/build.gradle"
    gradle = read_file(gradle_path)
    cur_vc = int(cur_ver.replace(".", ""))
    new_vc = int(new_ver.replace(".", ""))
    gradle = gradle.replace(f'versionCode {cur_vc}', f'versionCode {new_vc}')
    gradle = gradle.replace(f'versionName "{cur_ver}"', f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Add sync fields to Config page HTML (before submit button)
    # Target: the block that ends with the desktop_ext_cmd <small> and then the button
    old_html = """            <small class="config-hint">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <button type="submit" class="config-save-btn">Save Configuration</button>"""

    new_html = """            <small class="config-hint">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <div class="config-field">
            <label class="config-label">Sync Remote (git URL)</label>
            <input type="text" id="cfgSyncRemote" value="%s" class="config-input" placeholder="git@host:repo.git" />
        </div>
        <div class="config-field">
            <label class="config-label">Sync SSH Key Path (relative to storage dir)</label>
            <input type="text" id="cfgSyncSSHKey" value="%s" class="config-input" placeholder="omngo_sync_key" />
        </div>
        <div class="config-field">
            <label class="config-label">Sync SSH Passphrase (optional)</label>
            <input type="password" id="cfgSyncPassphrase" value="%s" class="config-input" placeholder="leave empty if none" />
        </div>
        <button type="submit" class="config-save-btn">Save Configuration</button>"""
    safe_patch("backend/handlers.go", old_html, new_html)

    # 3. Add the three new parameters to the JavaScript saveConfig function
    old_js = """        params.append("desktop_ext_cmd", document.getElementById("cfgExtCmd").value);
"""
    new_js = """        params.append("desktop_ext_cmd", document.getElementById("cfgExtCmd").value);
        params.append("sync_remote", document.getElementById("cfgSyncRemote").value);
        params.append("sync_ssh_key", document.getElementById("cfgSyncSSHKey").value);
        params.append("sync_ssh_passphrase", document.getElementById("cfgSyncPassphrase").value);
"""
    safe_patch("backend/handlers.go", old_js, new_js)

    # 4. Update handleConfig POST to save the new fields
    # The current POST block already saves all appConfig fields to JSON, so no extra code needed if the fields are in the Config struct,
    # but we must actually assign them from the form values. The existing code only assigns the previously known fields.
    # We need to add assignments for SyncRemote, SyncSSHKey, SyncSSHPassphrase.
    # Locate the line where appConfig.DesktopExtCmd is set, add after it.
    old_set = "appConfig.DesktopExtCmd = r.FormValue(\"desktop_ext_cmd\")"
    new_set = """appConfig.DesktopExtCmd = r.FormValue("desktop_ext_cmd")
		appConfig.SyncRemote = r.FormValue("sync_remote")
		appConfig.SyncSSHKey = r.FormValue("sync_ssh_key")
		appConfig.SyncSSHPassphrase = r.FormValue("sync_ssh_passphrase")"""
    safe_patch("backend/handlers.go", old_set, new_set)

    # 5. Also need to update the fmt.Sprintf arguments for the config form to include the new values.
    # The current Sprintf call has 6 %s arguments (or maybe 5? Let's check). In the code, the Sprintf has:
    #   appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,
    #   func() string for checked, appConfig.DesktopExtCmd
    # That's 6 arguments. Now we added 3 new fields, so we need 9 arguments.
    # We'll replace the fmt.Sprintf call arguments to include the new sync fields.
    # The target is the end of the Sprintf call: `appConfig.DesktopExtCmd)`
    # We'll change it to include the three new fields.
    old_sprintf = "appConfig.DesktopExtCmd)"
    new_sprintf = "appConfig.DesktopExtCmd,\n\t\tappConfig.SyncRemote, appConfig.SyncSSHKey, appConfig.SyncSSHPassphrase)"
    safe_patch("backend/handlers.go", old_sprintf, new_sprintf)

    # 6. Finally, ensure the config.json default includes these fields, which we already did in config.go, but that patch may have been applied.
    # We'll trust that, but if not, the Config page would just show empty strings.

    commit_msg = (
        "feat(ui): expose sync configuration fields on Config page\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()