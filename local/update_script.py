#!/usr/bin/env python3
"""OMN-Go 1.3.33 → 1.3.34: fix external editor .md extension, header metadata, updateArrow error, move toolbar into collapsible header."""

import os

def patch_file(path, old, new):
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    if old not in text:
        # Find best match for debugging
        for i, line in enumerate(text.split("\n")):
            if old[:40].strip() in line:
                raise ValueError(f"❌ Patch target not found in {path} near line {i+1}.\nExpected:\n{old[:200]}\nFound similar:\n{line[:200]}")
        raise ValueError(f"❌ Patch target not found in {path}.\nFirst 120 chars:\n{old[:120]}")
    text = text.replace(old, new, 1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)

def update_application():
    # 1. VERSION BUMPS
    patch_file("backend/version.go",
               'APP_VERSION = "1.3.33"',
               'APP_VERSION = "1.3.34"')
    patch_file("android/app/build.gradle",
               "versionCode 10333",
               "versionCode 10334")
    patch_file("android/app/build.gradle",
               'versionName "1.3.33"',
               'versionName "1.3.34"')

    # ===================================================================
    # 2. INDEX.HTML — Move toolbar actions into collapsible header,
    #    add metadata info line, improve arrow safety.
    # ===================================================================
    idx = "backend/frontend/index.html"

    # --- Patch 2a: Replace the collapsible header block to include all buttons and metadata ---
    old_header = '''        <div id="ptitle" class="page-header">
            <div id="hidable_header" class="collapsible-header hidden">
                <div class="header-actions">
                    <a href="/Welcome.html"><i class="material-icons">home</i></a>
                    <a href="/Welcome.html#help"><i class="material-icons">help</i></a>
                    <button onclick="createNewPage()" class="admin-only btn-create-page"><i class="material-icons">note_add</i></button>
                    <button onclick="window.location.href = window.location.pathname + '?refresh=1'" class="admin-only btn-refresh"><i class="material-icons">refresh</i></button>
                    <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bolt</i></button>
                    <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bookmark_add</i></button>
                    <a href="/Bookmarks.html"><i class="material-icons">bookmarks</i></a>
                    <a href="#" onclick="window.location.replace('/Config.html'); return false;" class="btn-settings"><i class="material-icons">settings</i></a>
                </div>
                <div class="header-info">
                    Name: <span id="pageNameDisplay">/</span>
                </div>
                <div id="headerTags"><!-- OMN_GO_TAGS --></div>
            </div>
            <h4 id="pageTitle" class="page-title" onclick="toggleHeader()">
                <!-- OMN_GO_PAGE_TITLE -->
                <span id="title_arrow" class="title-arrow">+</span>
            </h4>
        </div>'''

    new_header = '''        <div id="ptitle" class="page-header">
            <div id="hidable_header" class="collapsible-header hidden">
                <!-- Actions row: navigation + editing + admin -->
                <div class="header-actions">
                    <a href="/Welcome.html"><i class="material-icons">home</i></a>
                    <a href="/Welcome.html#help"><i class="material-icons">help</i></a>
                    <button onclick="createNewPage()" class="admin-only btn-create-page"><i class="material-icons">note_add</i></button>
                    <button onclick="window.location.href = window.location.pathname + '?refresh=1'" class="admin-only btn-refresh"><i class="material-icons">refresh</i></button>
                    <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bolt</i></button>
                    <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bookmark_add</i></button>
                    <a href="/Bookmarks.html"><i class="material-icons">bookmarks</i></a>
                    <a href="#" onclick="window.location.replace('/Config.html'); return false;" class="btn-settings"><i class="material-icons">settings</i></a>
                    <!-- Editing controls -->
                    <button id="metaToggleBtn" onclick="var h=document.getElementById('hidable_header');h.classList.toggle('hidden');if(typeof updateArrow==='function')updateArrow();" class="btn-metadata-toggle" title="Toggle header"><i class="material-icons">info</i></button>
                    <button id="saveBtn" onclick="saveNote()" class="admin-only btn-save-note"><i class="material-icons">save</i></button>
                    <button id="toggleBtn" onclick="toggleMode()" class="admin-only"><i class="material-icons">edit</i></button>
                </div>
                <!-- Metadata line: name + pelican headers -->
                <div class="header-info">
                    Name: <span id="pageNameDisplay">/</span>
                    <span id="headerMetadata"><!-- OMN_GO_METADATA_INFO --></span>
                </div>
                <!-- Tag pills -->
                <div id="headerTags"><!-- OMN_GO_TAGS --></div>
            </div>
            <h4 id="pageTitle" class="page-title" onclick="toggleHeader()">
                <!-- OMN_GO_PAGE_TITLE -->
                <span id="title_arrow" class="title-arrow">+</span>
            </h4>
        </div>'''
    patch_file(idx, old_header, new_header)

    # --- Patch 2b: Remove the old toolbar from the content area (now inside header) ---
    old_toolbar_content = '''        <div id="content" class="page-content">
            <div class="toolbar">
                <button id="metaToggleBtn" onclick="var h=document.getElementById('hidable_header');h.classList.toggle('hidden');updateArrow();" class="btn-metadata-toggle"><i class="material-icons">info</i></button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only btn-save-note"><i class="material-icons">save</i></button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only"><i class="material-icons">edit</i></button>
            </div>
            <div id="metadataPanel" class="hidden metadata-panel"><!-- OMN_GO_METADATA_PANEL --></div>'''
    new_toolbar_content = '''        <div id="content" class="page-content">
            <div id="metadataPanel" class="hidden metadata-panel"><!-- OMN_GO_METADATA_PANEL --></div>'''
    patch_file(idx, old_toolbar_content, new_toolbar_content)

    # ===================================================================
    # 3. JAVASCRIPT — Fix toggleMode external editor extension,
    #    update updateArrow safety, populate header metadata.
    # ===================================================================
    js_path = "backend/frontend/html/js/omn-go-core.js"

    # 3a. Fix toggleMode to append .md extension when no explicit FILE_EXT is set
    old_toggle = '''        async function toggleMode() {
            if (currentMode === 'view') {
                if (typeof USE_INTERNAL_ED !== 'undefined' && !USE_INTERNAL_ED) {
                    window.location.replace('/api/edit-external?name=' + encodeURIComponent(currentNote));
                    return;
                }'''
    new_toggle = '''        async function toggleMode() {
            if (currentMode === 'view') {
                if (typeof USE_INTERNAL_ED !== 'undefined' && !USE_INTERNAL_ED) {
                    var ext = (typeof PAGE_EXT !== 'undefined' && PAGE_EXT) ? PAGE_EXT : '.md';
                    window.location.replace('/api/edit-external?name=' + encodeURIComponent(currentNote + ext));
                    return;
                }'''
    patch_file(js_path, old_toggle, new_toggle)

    # 3b. Add populateHeaderMetadata function and call it on DOMContentLoaded
    #     Insert after metadata panel builder that updates pageNameDisplay
    old_meta_display = '''var nameDisplay = document.getElementById('pageNameDisplay');
        if (nameDisplay && typeof PageName !== 'undefined') {
            nameDisplay.textContent = '/' + PageName;
        }'''
    new_meta_display = '''var nameDisplay = document.getElementById('pageNameDisplay');
        if (nameDisplay && typeof PageName !== 'undefined') {
            nameDisplay.textContent = '/' + PageName;
        }
        // Populate header metadata line (Author, Date, Modified) from meta tags
        var hMeta = document.getElementById('headerMetadata');
        if (hMeta) {
            var parts = [];
            document.querySelectorAll('meta[name]').forEach(function(m) {
                var n = m.getAttribute('name').toLowerCase();
                if (n === 'author' || n === 'date' || n === 'modified') {
                    parts.push(m.getAttribute('name') + ': ' + m.getAttribute('content'));
                }
            });
            if (parts.length) {
                hMeta.innerHTML = ' — ' + parts.join(' · ');
            }
        }'''
    patch_file(js_path, old_meta_display, new_meta_display)

    # 3c. Ensure updateArrow is safe in toolbar onClick by checking typeof (already done in HTML)

    # ===================================================================
    # 4. GO — markdown.go: inject metadata info placeholder, and PAGE_EXT script
    # ===================================================================
    go_path = "backend/markdown.go"

    # 4a. Add metadata info string for the header (Author, Date, Modified) to layout
    old_meta_panel_injection = '''	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_TAGS -->", tagsHTML)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")'''
    new_meta_panel_injection = '''	// Build metadata info line for collapsible header
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

	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_INFO -->", metaInfo)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_TAGS -->", tagsHTML)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")'''
    patch_file(go_path, old_meta_panel_injection, new_meta_panel_injection)

    # 4b. Inject PAGE_EXT script variable for the frontend to know the file extension
    old_meta_script_inject = '''	metaScript := fmt.Sprintf(`    <script>
      var PackageName = 'net.basov.omngo';
      var PageName = '%s';
      var Title = '%s';
    </script>`, name, title)'''
    new_meta_script_inject = '''	// Determine file extension for editor use
	pageExt := ""
	if strings.HasSuffix(name, ".md") {
		pageExt = ".md"
	} else if strings.Contains(name, ".") {
		// non-markdown file — keep its extension (e.g. .js, .css, .json)
		pageExt = filepath.Ext(name)
	}
	metaScript := fmt.Sprintf(`    <script>
      var PackageName = 'net.basov.omngo';
      var PageName = '%s';
      var Title = '%s';
      var PAGE_EXT = '%s';
    </script>`, name, title, pageExt)'''
    patch_file(go_path, old_meta_script_inject, new_meta_script_inject)

    # 4c. Also inject IS_MARKDOWN for markdown pages (currently only set to false for non-md)
    # We'll set IS_MARKDOWN = true for markdown pages (explicitly) to avoid undefined issues.
    old_markdown_flag_injection = '''	layout = strings.ReplaceAll(layout, "</head>", metaBlock+"\\n</head>")'''
    new_markdown_flag_injection = '''	// Explicitly set IS_MARKDOWN = true for markdown pages (overrides any previous false)
	if pageExt == ".md" || pageExt == "" {
		metaBlock += "\n    <script>var IS_MARKDOWN = true;</script>"
	}
	layout = strings.ReplaceAll(layout, "</head>", metaBlock+"\\n</head>")'''
    patch_file(go_path, old_markdown_flag_injection, new_markdown_flag_injection)

    # ===================================================================
    # 5. CSS — Minor adjustments for toolbar-in-header
    # ===================================================================
    css_path = "backend/frontend/html/css/omn-go-core.css"
    new_css = r"""
/* Toolbar no longer a standalone element, but integrated in .header-actions */
.toolbar {
    display: none;
}
.btn-metadata-toggle,
.btn-save-note {
    /* inherit from .header-actions styling */
}
"""
    with open(css_path, "a", encoding="utf-8") as f:
        f.write(new_css)

    # ===================================================================
    # 6. GIT COMMIT MESSAGE
    # ===================================================================
    commit = (
        "fix(ui,editor): external editor .md extension, header metadata, updateArrow safety, toolbar merge\n\n"
        "- External editor (toggleMode) now appends .md to page name when no explicit\n"
        "  PAGE_EXT is set, preventing empty files on desktop/Android.\n"
        "- Collapsible header now displays Author, Date, Modified metadata in the\n"
        "  info line alongside the page name.\n"
        "- updateArrow calls are guarded with typeof check to avoid ReferenceError.\n"
        "- Toolbar buttons (info, save, edit) moved into the collapsible header,\n"
        "  eliminating the separate toolbar row.  The content area is now cleaner.\n"
        "- Markdown pages now explicitly set IS_MARKDOWN = true for frontend logic.\n"
        "- Added PAGE_EXT variable injected by the Go compiler for accurate editing.\n\n"
        "Version bumped to 1.3.34"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()