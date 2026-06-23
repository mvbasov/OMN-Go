#!/usr/bin/env python3
"""OMN-Go 1.3.38 → 1.3.39: hide save button in view mode, remove redundant header metadata line."""

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
    # 1. Bump version
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

    # 2. Remove the .header-info line (metadata Author/Date/Modified) from index.html
    idx_path = 'backend/frontend/index.html'
    old_info_block = '''                <!-- Header metadata (Author, Date, Modified) displayed inline after icons -->
                <div class="header-info">
                    <span id="headerMetadata"><!-- OMN_GO_METADATA_INFO --></span>
                </div>
'''
    new_info_block = ''  # remove entirely
    # Check if the block exists before patching
    idx_content = read_file(idx_path)
    if old_info_block in idx_content:
        idx_content = idx_content.replace(old_info_block, new_info_block)
        write_file(idx_path, idx_content)

    # 3. Fix save button visibility: increase specificity to beat .header-actions button
    css_path = 'backend/frontend/html/css/omn-go-core.css'
    # Replace the existing .btn-save-note rule
    old_save_btn = '''.btn-save-note {
    display: none;                   /* hidden by default, shown via JS */
    background: #28a745;
    color: white;
    border: none;
}'''
    new_save_btn = '''.btn-save-note {
    display: none !important;        /* hidden in view mode, JS sets inline style in edit mode */
    background: #28a745;
    color: white;
    border: none;
}'''
    patch_file(css_path, old_save_btn, new_save_btn)

    # 4. Clean up Go: skip generating OMN_GO_METADATA_INFO since the placeholder is gone
    go_path = 'backend/markdown.go'
    # The block that builds metaInfo and injects it — we just remove the injection line
    old_go_block = '''	// Build metadata info line for collapsible header
	metaInfoParts := []string{}
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 {
			key := strings.ToLower(strings.TrimSpace(parts[0]))
			val := htmlEscape(strings.TrimSpace(parts[1]))
			if key == "author" || key == "date" || key == "modified" {
				metaInfoParts = append(metaInfoParts, fmt.Sprintf("%s: %s", strings.Title(key), val))
			}
		}
	}
	metaInfo := strings.Join(metaInfoParts, " · ")

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_INFO -->", metaInfo)'''
    new_go_block = '''	// Metadata info now shown only in the metadata panel (via meta tags);
	// the inline header line has been removed from the template.
	metaInfo := ""'''
    if old_go_block in read_file(go_path):
        patch_file(go_path, old_go_block, new_go_block)

    # 5. Commit message
    commit_msg = (
        f"fix(ui): hide save button in view mode; remove redundant header metadata line\n\n"
        "- Save button now uses !important to beat the .header-actions button\n"
        "  flex display, so it stays hidden in view mode and only appears\n"
        "  when toggleMode() sets display:block inline.\n"
        "- Removed the Author / Date / Modified line from the collapsible\n"
        "  header; that information is already visible in the metadata\n"
        "  panel toggled by the info (i) button.\n"
        f"Version bumped to {new_ver}"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit_msg.strip()}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()