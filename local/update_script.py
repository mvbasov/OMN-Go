#!/usr/bin/env python3
"""OMN-Go 1.3.30 → 1.3.31: refactor embedded inline styles into omn-go-core.css."""

import os

def patch_file(path, old, new):
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    if old not in text:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}...")
    text = text.replace(old, new, 1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)

def update_application():
    # ========== VERSION BUMPS ==========
    patch_file("backend/version.go",
               'APP_VERSION = "1.3.30"',
               'APP_VERSION = "1.3.31"')
    patch_file("android/app/build.gradle",
               "versionCode 10330",
               "versionCode 10331")
    patch_file("android/app/build.gradle",
               'versionName "1.3.30"',
               'versionName "1.3.31"')

    # ===================================================================
    # 1. Refactor getConfigPageBody in handlers.go
    # ===================================================================
    old_config = '''func getConfigPageBody() string {
	return fmt.Sprintf(`
<div style="max-width: 600px; margin: 0 auto; background: #ffffff; padding: 30px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8;">
    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700; border-bottom: 2px solid #eaecef; padding-bottom: 10px;">Configuration Dashboard</h2>
    <form id="configForm" onsubmit="saveConfig(event)" style="margin-top: 20px;">
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Server Port</label>
            <input type="number" id="cfgPort" value="%d" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Admin Password</label>
            <input type="password" id="cfgAdminPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Guest Password</label>
            <input type="password" id="cfgGuestPwd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" required />
        </div>
        <div style="margin-bottom: 20px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Author Name</label>
            <input type="text" id="cfgAuthor" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
        </div>
        <div style="margin-bottom: 20px; display: flex; align-items: center; gap: 10px;">
            <input type="checkbox" id="cfgUseInternal" %s style="width: 20px; height: 20px; cursor: pointer;" />
            <label for="cfgUseInternal" style="font-weight: 600; color: #444; cursor: pointer;">Use HTML Internal Editor</label>
        </div>
        <div style="margin-bottom: 25px;">
            <label style="display: block; font-weight: 600; margin-bottom: 8px; color: #444;">Desktop External Editor Command</label>
            <input type="text" id="cfgExtCmd" value="%s" style="width: 100%%; padding: 10px; border: 1px solid #ccc; border-radius: 4px; box-sizing: border-box;" />
            <small style="color: #666; display: block; margin-top: 5px;">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <button type="submit" style="background: #28a745; color: white; border: none; padding: 12px 20px; border-radius: 4px; font-weight: bold; cursor: pointer; width: 100%%; font-size: 16px; transition: background 0.2s;">Save Configuration</button>
    </form>
</div>
<script>
    async function saveConfig(event) {
        event.preventDefault();
        const params = new URLSearchParams();
        params.append("server_port", document.getElementById("cfgPort").value);
        params.append("admin_password", document.getElementById("cfgAdminPwd").value);
        params.append("guest_password", document.getElementById("cfgGuestPwd").value);
        params.append("author", document.getElementById("cfgAuthor").value);
        params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");
        params.append("desktop_ext_cmd", document.getElementById("cfgExtCmd").value);

        const res = await fetch("/api/config", { method: "POST", body: params });
        if (res.ok) {
            alert("Configuration saved successfully! Server port changes will take effect after restarting the application.");
            window.location.reload();
        } else {
            alert("Failed to save configuration.");
        }
    }
</script>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,
		func() string {
			if appConfig.UseInternalEd {
				return "checked"
			}
			return ""
		}(),
		appConfig.DesktopExtCmd)
}'''

    new_config = '''func getConfigPageBody() string {
	return fmt.Sprintf(`
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
            <input type="checkbox" id="cfgUseInternal" %s class="config-checkbox" />
            <label for="cfgUseInternal" class="config-label config-checkbox-label">Use HTML Internal Editor</label>
        </div>
        <div class="config-field">
            <label class="config-label">Desktop External Editor Command</label>
            <input type="text" id="cfgExtCmd" value="%s" class="config-input" />
            <small class="config-hint">Example: <code>subl</code> or <code>code</code> or <code>nano</code></small>
        </div>
        <button type="submit" class="config-save-btn">Save Configuration</button>
    </form>
</div>
<script>
    async function saveConfig(event) {
        event.preventDefault();
        const params = new URLSearchParams();
        params.append("server_port", document.getElementById("cfgPort").value);
        params.append("admin_password", document.getElementById("cfgAdminPwd").value);
        params.append("guest_password", document.getElementById("cfgGuestPwd").value);
        params.append("author", document.getElementById("cfgAuthor").value);
        params.append("use_internal_editor", document.getElementById("cfgUseInternal").checked ? "true" : "false");
        params.append("desktop_ext_cmd", document.getElementById("cfgExtCmd").value);

        const res = await fetch("/api/config", { method: "POST", body: params });
        if (res.ok) {
            alert("Configuration saved successfully! Server port changes will take effect after restarting the application.");
            window.location.reload();
        } else {
            alert("Failed to save configuration.");
        }
    }
</script>
`, appConfig.ServerPort, appConfig.AdminPassword, appConfig.GuestPassword, appConfig.Author,
		func() string {
			if appConfig.UseInternalEd {
				return "checked"
			}
			return ""
		}(),
		appConfig.DesktopExtCmd)
}'''

    patch_file("backend/handlers.go", old_config, new_config)

    # ===================================================================
    # 2. Refactor getExternalEditPageBody in handlers.go
    # ===================================================================
    old_ext_edit = '''func getExternalEditPageBody(fileName string) string {
	return fmt.Sprintf(`
<div style="max-width: 600px; margin: 40px auto; background: #ffffff; padding: 40px; border-radius: 8px; box-shadow: 0 4px 12px rgba(0,0,0,0.1); border: 1px solid #e1e4e8; text-align: center;">
    <div style="font-size: 48px; margin-bottom: 20px;">📝</div>
    <h2 style="margin-top: 0; color: #1a1a1a; font-size: 24px; font-weight: 700;">Editing Externally</h2>
    <p style="color: #555; font-size: 16px; margin-bottom: 30px; line-height: 1.5;">
        We have launched <strong>%s</strong> to edit <code>%s</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated file.
    </p>
    <button onclick="window.location.replace('/%s')" style="background: #0056b3; color: white; border: none; padding: 15px 30px; border-radius: 6px; font-weight: bold; cursor: pointer; font-size: 18px; transition: background 0.2s; box-shadow: 0 2px 5px rgba(0,0,0,0.2);">
        Press after edit to refresh view
    </button>
</div>
`, appConfig.DesktopExtCmd, fileName, fileName)
}'''

    new_ext_edit = '''func getExternalEditPageBody(fileName string) string {
	return fmt.Sprintf(`
<div class="ext-edit-panel">
    <div class="ext-edit-icon">📝</div>
    <h2 class="ext-edit-title">Editing Externally</h2>
    <p class="ext-edit-msg">
        We have launched <strong>%s</strong> to edit <code>%s</code>. Please complete your changes in your editor, save the file, and click the button below to view the updated file.
    </p>
    <button onclick="window.location.replace('/%s')" class="ext-edit-btn">
        Press after edit to refresh view
    </button>
</div>
`, appConfig.DesktopExtCmd, fileName, fileName)
}'''

    patch_file("backend/handlers.go", old_ext_edit, new_ext_edit)

    # ===================================================================
    # 3. Append new CSS classes to omn-go-core.css
    # ===================================================================
    css_path = "backend/frontend/html/css/omn-go-core.css"
    new_css = r"""
/* Configuration Dashboard */
.config-panel {
    max-width: 600px;
    margin: 0 auto;
    background: #ffffff;
    padding: 30px;
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0,0,0,0.1);
    border: 1px solid #e1e4e8;
}
.config-title {
    margin-top: 0;
    color: #1a1a1a;
    font-size: 24px;
    font-weight: 700;
    border-bottom: 2px solid #eaecef;
    padding-bottom: 10px;
}
.config-form {
    margin-top: 20px;
}
.config-field {
    margin-bottom: 20px;
}
.config-label {
    display: block;
    font-weight: 600;
    margin-bottom: 8px;
    color: #444;
}
.config-input {
    width: 100%;
    padding: 10px;
    border: 1px solid #ccc;
    border-radius: 4px;
    box-sizing: border-box;
}
.config-checkbox-row {
    display: flex;
    align-items: center;
    gap: 10px;
}
.config-checkbox {
    width: 20px;
    height: 20px;
    cursor: pointer;
}
.config-checkbox-label {
    font-weight: 600;
    color: #444;
    cursor: pointer;
}
.config-hint {
    color: #666;
    display: block;
    margin-top: 5px;
}
.config-save-btn {
    background: #28a745;
    color: white;
    border: none;
    padding: 12px 20px;
    border-radius: 4px;
    font-weight: bold;
    cursor: pointer;
    width: 100%;
    font-size: 16px;
    transition: background 0.2s;
}
.config-save-btn:hover {
    background: #218838;
}

/* External Editor Page */
.ext-edit-panel {
    max-width: 600px;
    margin: 40px auto;
    background: #ffffff;
    padding: 40px;
    border-radius: 8px;
    box-shadow: 0 4px 12px rgba(0,0,0,0.1);
    border: 1px solid #e1e4e8;
    text-align: center;
}
.ext-edit-icon {
    font-size: 48px;
    margin-bottom: 20px;
}
.ext-edit-title {
    margin-top: 0;
    color: #1a1a1a;
    font-size: 24px;
    font-weight: 700;
}
.ext-edit-msg {
    color: #555;
    font-size: 16px;
    margin-bottom: 30px;
    line-height: 1.5;
}
.ext-edit-btn {
    background: #0056b3;
    color: white;
    border: none;
    padding: 15px 30px;
    border-radius: 6px;
    font-weight: bold;
    cursor: pointer;
    font-size: 18px;
    transition: background 0.2s;
    box-shadow: 0 2px 5px rgba(0,0,0,0.2);
}
.ext-edit-btn:hover {
    background: #004494;
}
"""
    with open(css_path, "a", encoding="utf-8") as f:
        f.write(new_css)

    # ========== GIT COMMIT MESSAGE ==========
    commit = (
        "refactor(css): extract inline styles from config and external edit pages\n\n"
        "Moved large embedded CSS blocks from Go handlers into dedicated CSS classes\n"
        "inside omn-go-core.css.  The Configuration Dashboard now uses classes\n"
        "`.config-panel`, `.config-title`, `.config-form`, `.config-field`,\n"
        "`.config-label`, `.config-input`, `.config-checkbox-row`, etc.\n"
        "The External Editor page uses `.ext-edit-panel`, `.ext-edit-icon`,\n"
        "`.ext-edit-title`, `.ext-edit-msg`, and `.ext-edit-btn`.\n\n"
        "This keeps the Go code cleaner and improves maintainability.\n\n"
        "Version bumped to 1.3.31"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()