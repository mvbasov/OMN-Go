#!/usr/bin/env python3
"""OMN-Go 1.3.36 → 1.3.37: fix external editor refresh URL to use .html extension."""

import re, os

def read_file(path):
    with open(path, 'r', encoding='utf-8') as f:
        return f.read()

def write_file(path, content):
    with open(path, 'w', encoding='utf-8') as f:
        f.write(content)

def patch_file(path, old, new):
    content = read_file(path)
    if old not in content:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old[:120]}")
    content = content.replace(old, new, 1)
    write_file(path, content)

def increment_version(ver_str):
    parts = ver_str.strip().split('.')
    if len(parts) == 3 and parts[2].isdigit():
        parts[2] = str(int(parts[2]) + 1)
        return '.'.join(parts)
    raise ValueError(f"Unrecognised version format: {ver_str}")

def update_application():
    # 1. Auto‑detect current version and bump
    ver_path = 'backend/version.go'
    content = read_file(ver_path)
    match = re.search(r'APP_VERSION\s*=\s*"(\d+\.\d+\.\d+)"', content)
    cur_ver = match.group(1)
    new_ver = increment_version(cur_ver)

    content = content.replace(f'APP_VERSION = "{cur_ver}"', f'APP_VERSION = "{new_ver}"')
    write_file(ver_path, content)

    gradle_path = 'android/app/build.gradle'
    gradle = read_file(gradle_path)
    gradle = gradle.replace(f'versionCode {int(cur_ver.replace(".", ""))}',
                            f'versionCode {int(new_ver.replace(".", ""))}')
    gradle = gradle.replace(f'versionName "{cur_ver}"',
                            f'versionName "{new_ver}"')
    write_file(gradle_path, gradle)

    # 2. Patch getExternalEditPageBody to accept separate view URL
    old_func = """func getExternalEditPageBody(fileName string) string {
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
}"""
    new_func = """func getExternalEditPageBody(fileName string, viewURL string) string {
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
`, appConfig.DesktopExtCmd, fileName, viewURL)
}"""
    patch_file('backend/handlers.go', old_func, new_func)

    # 3. Patch handleEditExternal to compute viewURL and pass it
    # Old block: after filePath is determined, before waitBody.
    # We insert a line to compute viewURL, then update the call.
    # Find the exact lines:
    #   	waitBody := getExternalEditPageBody(name)
    #   	compiledWait := compilePageWithBody(name, ...)
    old_call = """	waitBody := getExternalEditPageBody(name)
	compiledWait := compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody)"""
    new_call = """	// Compute the correct view URL (.html for markdown, raw name otherwise)
	viewURL := name
	if strings.HasSuffix(name, ".md") {
		viewURL = strings.TrimSuffix(name, ".md") + ".html"
	}
	waitBody := getExternalEditPageBody(name, viewURL)
	compiledWait := compilePageWithBody(name, fmt.Appendf(nil, "Title: Refresh %s\\nDate: %s\\nCategory: Action\\n\\n", name, time.Now().Format("2006-01-02 15:04:05")), waitBody)"""
    patch_file('backend/handlers.go', old_call, new_call)

    # 4. Commit message
    commit_msg = (
        f"fix(editor): external editor refresh now loads .html page for markdown files\n\n"
        "- The 'Press after edit to refresh view' button previously used the raw\n"
        "  file name with .md extension, which the server would 404 because it\n"
        "  serves compiled .html pages.  Now it redirects to the .html version.\n"
        "- getExternalEditPageBody now accepts a separate viewURL parameter.\n"
        "- Non‑markdown files (e.g., .js, .css, .json) are unaffected because\n"
        "  they are served directly and the viewURL remains the raw file name.\n\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()