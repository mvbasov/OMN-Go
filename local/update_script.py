#!/usr/bin/env python3
"""OMN-Go 1.3.32 → 1.3.33: restructure UI to match legacy OMN design — collapsible header, borderless content, tag display, utility classes."""

import os

def patch_file(path, old, new):
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    if old not in text:
        # Debug: find what's close
        for i, line in enumerate(text.split("\n")):
            if old[:40] in line:
                raise ValueError(f"❌ Patch target not found in {path} around line {i+1}.\nExpected:\n{old[:200]}\n\nFound similar at line {i+1}:\n{line[:200]}")
        raise ValueError(f"❌ Patch target not found in {path}.\nFirst 200 chars of target:\n{old[:200]}")
    text = text.replace(old, new, 1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)

def update_application():
    # ===================================================================
    # 1. VERSION BUMPS
    # ===================================================================
    patch_file("backend/version.go",
               'APP_VERSION = "1.3.32"',
               'APP_VERSION = "1.3.33"')
    patch_file("android/app/build.gradle",
               "versionCode 10332",
               "versionCode 10333")
    patch_file("android/app/build.gradle",
               'versionName "1.3.32"',
               'versionName "1.3.33"')

    # ===================================================================
    # 2. INDEX.HTML — Structural Refactor
    # ===================================================================
    idx = "backend/frontend/index.html"

    # --- Patch 2a: Add page_container class to #mainUI ---
    patch_file(idx, '<div id="mainUI">', '<div id="mainUI" class="page_container">')

    # --- Patch 2b: Replace the .header nav bar with collapsible #ptitle ---
    old_header_block = '''        <div class="header">
            <strong><!-- OMN_GO_PAGE_TITLE --></strong>
            <a href="/Welcome.html"><i class="material-icons">home</i></a>
            <a href="/Welcome.html#help"><i class="material-icons">help</i></a>
            <button onclick="createNewPage()" class="admin-only btn-create-page"><i class="material-icons">note_add</i></button>
            <button onclick="window.location.href = window.location.pathname + '?refresh=1'" class="admin-only btn-refresh"><i class="material-icons">refresh</i></button>
            <button onclick="document.getElementById('quickPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bolt</i></button>
            <button onclick="document.getElementById('bmPanel').classList.toggle('hidden')" class="admin-only"><i class="material-icons">bookmark_add</i></button>
            <a href="/Bookmarks.html"><i class="material-icons">bookmarks</i></a>
            <a href="#" onclick="window.location.replace('/Config.html'); return false;" class="btn-settings"><i class="material-icons">settings</i></a>
        </div>'''

    new_header_block = '''        <div id="ptitle" class="page-header">
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

    patch_file(idx, old_header_block, new_header_block)

    # --- Patch 2c: Replace .content-area with #content ---
    old_content_area = '''        <div class="content-area">
            <div class="toolbar">
                <button id="metaToggleBtn" onclick="document.getElementById('metadataPanel').classList.toggle('hidden')" class="btn-metadata-toggle"><i class="material-icons">info</i></button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only btn-save-note"><i class="material-icons">save</i></button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only"><i class="material-icons">edit</i></button>
            </div>
            <div id="metadataPanel" class="hidden metadata-panel"><!-- OMN_GO_METADATA_PANEL --></div>
            <textarea id="editor" class="admin-only" placeholder="Markdown/Code content... Drag images here to upload."><!-- OMN_GO_RAW_MD --></textarea>
            <div id="preview"><!-- OMN_GO_PREVIEW_BODY --></div>
        </div>'''

    new_content_area = '''        <div id="content" class="page-content">
            <div class="toolbar">
                <button id="metaToggleBtn" onclick="var h=document.getElementById('hidable_header');h.classList.toggle('hidden');updateArrow();" class="btn-metadata-toggle"><i class="material-icons">info</i></button>
                <button id="saveBtn" onclick="saveNote()" class="admin-only btn-save-note"><i class="material-icons">save</i></button>
                <button id="toggleBtn" onclick="toggleMode()" class="admin-only"><i class="material-icons">edit</i></button>
            </div>
            <div id="metadataPanel" class="hidden metadata-panel"><!-- OMN_GO_METADATA_PANEL --></div>
            <textarea id="editor" class="admin-only" placeholder="Markdown/Code content... Drag images here to upload."><!-- OMN_GO_RAW_MD --></textarea>
            <div id="preview"><!-- OMN_GO_PREVIEW_BODY --></div>
        </div>'''

    patch_file(idx, old_content_area, new_content_area)

    # --- Patch 2d: Add status footer after content div, before Quick Note Modal ---
    old_qn_modal = '''    <!-- Quick Note Modal -->'''
    new_status_footer = '''    <div id="status" class="page-footer">
        <span class="version-footer-inline" id="omn-go-version-footer"></span>
    </div>

    <!-- Quick Note Modal -->'''
    patch_file(idx, old_qn_modal, new_status_footer)

    # --- Patch 2e: Remove standalone version footer div at bottom ---
    old_standalone_footer = '''    <div id="omn-go-version-footer" class="version-footer"></div>'''
    new_standalone_footer = '''    <!-- version footer now inline in #status -->'''
    patch_file(idx, old_standalone_footer, new_standalone_footer)

    # ===================================================================
    # 3. CSS — Major additions to omn-go-core.css
    # ===================================================================
    css_path = "backend/frontend/html/css/omn-go-core.css"
    new_css = r"""
/* ====== Legacy OMN Design: Page Container Flex Layout ====== */
.page_container {
    height: 100%;
    display: flex;
    flex-direction: column;
    padding: 0;
    margin: 0;
}

/* ====== Collapsible Page Header ====== */
.page-header {
    overflow: hidden;
    background-color: #e8e8e8;
    width: 100%;
    margin: 0;
    flex-shrink: 0;
    user-select: none;
}
.collapsible-header {
    background-color: #e8e8e8;
    width: 100%;
    padding: 0.5em;
    box-sizing: border-box;
}
.collapsible-header.hidden {
    display: none;
}
.header-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-bottom: 6px;
}
.header-actions a,
.header-actions button {
    color: #333;
    text-decoration: none;
    cursor: pointer;
    background: transparent;
    border: 1px solid #999;
    padding: 4px 8px;
    border-radius: 4px;
    font-size: 14px;
    display: flex;
    align-items: center;
}
.header-actions a:hover,
.header-actions button:hover {
    background: #d0d0d0;
}
.header-info {
    font-size: 0.9em;
    color: #555;
    margin-bottom: 4px;
}
.header-info span {
    font-weight: bold;
    color: #222;
}
.page-title {
    margin: 0.4em 0 0.5em 0;
    width: 100%;
    text-align: center;
    cursor: pointer;
    font-size: 1.1em;
    font-weight: bold;
    color: #1a1a1a;
}
.page-title:hover {
    color: #0056b3;
}
.title-arrow {
    display: inline-block;
    border: 1px solid #666;
    border-radius: 6px;
    padding: 0.1em 0.4em;
    margin-left: 6px;
    font-size: 0.9em;
    color: #444;
    background: #f5f5f5;
    vertical-align: middle;
}

/* ====== Page Content (flex:1 scrollable, no borders) ====== */
.page-content {
    padding: 0.5em;
    overflow: auto;
    flex: 1;
}
.page-content #preview {
    border: none;                /* Remove the old border */
    background: transparent;
    padding: 10px 0;
}
.page-content #editor {
    border: 1px solid #ddd;      /* Subtle border for textarea only */
}

/* ====== Page Footer (status bar) ====== */
.page-footer {
    overflow-x: hidden;
    flex-shrink: 0;
    background-color: #e8e8e8;
    padding: 2px 8px;
    font-size: 0.75rem;
    color: #666;
    display: flex;
    justify-content: space-between;
    align-items: center;
}
.version-footer-inline {
    font-size: 0.7rem;
    opacity: 0.7;
}

/* ====== Tag Display (from legacy common.css) ====== */
#headerTags {
    margin-top: 5px;
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
}
a.taglink {
    text-decoration: none;
    color: black;
}
.tagmark {
    background-color: #E7E7E7;
    padding: 2px 8px;
    border: 1px solid #505050;
    border-radius: 8px;
    font-size: 0.85em;
    display: inline-block;
}
.tagmark:hover {
    background-color: #d0d0d0;
}
.tagmarkselected {
    color: white;
    background-color: #888;
}

/* ====== TOC (Table of Contents) ====== */
#TOC {
    border: solid black 1px;
    margin: 10px;
    padding: 10px;
    background: #fafafa;
}
.TOCEntry {
    font-family: sans-serif;
}
.TOCEntry a {
    text-decoration: none;
}
.TOCLevel1 { font-weight: bold; }
.TOCLevel2 { font-weight: bold; }
.TOCLevel3 { font-weight: bold; }
.TOCLevel4 { font-weight: bold; margin-left: 0.5em; }
.TOCLevel5 { font-weight: bold; margin-left: 1em; }
.TOCLevel6 { font-weight: bold; margin-left: 1.5em; }

/* ====== Table Styling (from legacy common.css) ====== */
#preview table,
#content table {
    border-collapse: collapse;
}
#preview table,
#preview th,
#preview td,
#content table,
#content th,
#content td {
    border: 2px solid #4d6bfe;
}
#preview th,
#content th {
    background-color: #CCCCCC;
    color: black;
}
#preview tr:nth-child(odd),
#content tr:nth-child(odd) {
    background-color: #EEEEEE;
    color: black;
}
#preview tr:nth-child(even),
#content tr:nth-child(even) {
    background-color: #FFFFFF;
    color: black;
}

/* ====== Utility Color Classes (from legacy common.css) ====== */
.bg-yellow { background-color: yellow; }
.bg-aqua   { background-color: aqua; }
.fg-red    { color: red; }
.fg-green  { color: green; }

/* ====== Paragraph Indent (legacy style) ====== */
#preview p {
    text-indent: 1em;
    margin: 0 3px;
}

/* ====== Remove old .header (replaced by .page-header) ====== */
.header {
    display: none;  /* deprecated, kept for compatibility */
}
.content-area {
    /* deprecated, kept for compatibility */
}

/* ====== Adjust old version-footer (now inline) ====== */
.version-footer {
    position: static;  /* override fixed positioning */
    font-size: inherit;
    color: inherit;
    z-index: auto;
    opacity: 1;
    pointer-events: auto;
}
"""
    with open(css_path, "a", encoding="utf-8") as f:
        f.write(new_css)

    # ===================================================================
    # 4. JAVASCRIPT — Add toggleHeader + helpers to omn-go-core.js
    # ===================================================================
    js_path = "backend/frontend/html/js/omn-go-core.js"

    # Insert toggleHeader() before the window.onload section
    old_js_anchor = "        window.onload = () => {"
    new_js_insert = """        window.toggleHeader = function() {
            var header = document.getElementById('hidable_header');
            var arrow = document.getElementById('title_arrow');
            if (header) {
                if (header.classList.contains('hidden')) {
                    header.classList.remove('hidden');
                    if (arrow) arrow.textContent = '−';
                } else {
                    header.classList.add('hidden');
                    if (arrow) arrow.textContent = '+';
                }
            }
        };
        window.updateArrow = function() {
            var header = document.getElementById('hidable_header');
            var arrow = document.getElementById('title_arrow');
            if (header && arrow) {
                arrow.textContent = header.classList.contains('hidden') ? '+' : '−';
            }
        };

        window.onload = () => {"""
    patch_file(js_path, old_js_anchor, new_js_insert)

    # Update the page name display in the DOMContentLoaded metadata panel builder
    old_panel_name = """let metaHtml = `<div style="margin-bottom: 8px; color: #0056b3; font-weight: bold; border-bottom: 1px solid #ccc; padding-bottom: 4px;">File: ${typeof PageName !== 'undefined' ? PageName + '.md' : ''}</div>`;"""
    new_panel_name = """let metaHtml = `<div style="margin-bottom: 8px; color: #0056b3; font-weight: bold; border-bottom: 1px solid #ccc; padding-bottom: 4px;">File: ${typeof PageName !== 'undefined' ? PageName : ''}</div>`;
        // Also update the header name display
        var nameDisplay = document.getElementById('pageNameDisplay');
        if (nameDisplay && typeof PageName !== 'undefined') {
            nameDisplay.textContent = '/' + PageName;
        }"""
    patch_file(js_path, old_panel_name, new_panel_name)

    # ===================================================================
    # 5. GO — markdown.go: inject tags into <!-- OMN_GO_TAGS -->
    # ===================================================================
    go_path = "backend/markdown.go"

    # Add tag HTML generation before the metaBlock assembly
    old_meta_block = """	metaBlock := strings.Join(metaTags, "\\n") + "\\n" + metaScript"""
    new_meta_block = """	// Build tag links for the header
	var tagLinks []string
	for _, h := range headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "tags") {
			for _, tag := range strings.Split(parts[1], ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tagLinks = append(tagLinks, fmt.Sprintf(`<a href="Tags.html#%s" class="taglink"><span class="tagmark">%s</span></a>`, htmlEscape(tag), htmlEscape(tag)))
				}
			}
		}
	}
	tagsHTML := strings.Join(tagLinks, "\\n")

	metaBlock := strings.Join(metaTags, "\\n") + "\\n" + metaScript"""
    patch_file(go_path, old_meta_block, new_meta_block)

    # Inject tagsHTML into the layout
    old_layout_tags = """	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")"""
    new_layout_tags = """	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_TAGS -->", tagsHTML)
	layout = strings.ReplaceAll(layout, "<!-- OMN_GO_METADATA_PANEL -->", "")"""
    patch_file(go_path, old_layout_tags, new_layout_tags)

    # ===================================================================
    # 6. GIT COMMIT MESSAGE
    # ===================================================================
    commit = (
        "feat(ui): restructure layout to match legacy OMN design\n\n"
        "Complete visual overhaul to bring the classic Open Markdown Notes aesthetic\n"
        "into OMN-Go:\n\n"
        "- New .page_container flex layout: header stays at top, content fills\n"
        "  remaining space with natural scroll, status footer at bottom.\n"
        "- Collapsible header (#ptitle / #hidable_header): page title is always\n"
        "  visible; click to expand/collapse the action buttons, page name, and\n"
        "  tag badges.  Arrow indicator toggles between + and \u2212.\n"
        "- Content area (#preview) now has no border \u2014 clean, distraction-free\n"
        "  reading area matching the legacy look.\n"
        "- Tag badges extracted from Pelican \u2018Tags:\u2019 header and rendered as\n"
        "  .tagmark pills (links to Tags.html#TagName) inside the header.\n"
        "- Legacy table styling (blue borders, striped rows, gray headers).\n"
        "- TOC styling (#TOC box with nested bold levels).\n"
        "- Utility color classes: .bg-yellow, .bg-aqua, .fg-red, .fg-green.\n"
        "- Paragraph text-indent for the preview area.\n"
        "- Status footer bar with inline version display.\n\n"
        "Version bumped to 1.3.33"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()
