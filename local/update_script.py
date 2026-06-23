#!/usr/bin/env python3
"""OMN-Go 1.3.31 → 1.3.32: extract inline styles from index.html into omn-go-core.css."""

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
               'APP_VERSION = "1.3.31"',
               'APP_VERSION = "1.3.32"')
    patch_file("android/app/build.gradle",
               "versionCode 10331",
               "versionCode 10332")
    patch_file("android/app/build.gradle",
               'versionName "1.3.31"',
               'versionName "1.3.32"')

    # ========== 1. Update index.html ==========
    index_path = "backend/frontend/index.html"

    # Patch the login overlay (remove inline display:none, rely on CSS)
    old_overlay = '<div id="loginOverlay" class="overlay" style="display: none;">'
    new_overlay = '<div id="loginOverlay" class="overlay">'
    patch_file(index_path, old_overlay, new_overlay)

    # Header buttons and links: replace inline styles with classes
    # Create new page button
    old_btn_create = '<button onclick="createNewPage()" class="admin-only" style="background: #17a2b8; border-color: #17a2b8;"><i class="material-icons">note_add</i></button>'
    new_btn_create = '<button onclick="createNewPage()" class="admin-only btn-create-page"><i class="material-icons">note_add</i></button>'
    patch_file(index_path, old_btn_create, new_btn_create)

    # Refresh button
    old_btn_refresh = '<button onclick="window.location.href = window.location.pathname + \'?refresh=1\'" class="admin-only" style="background: #6c757d; border-color: #6c757d;"><i class="material-icons">refresh</i></button>'
    new_btn_refresh = '<button onclick="window.location.href = window.location.pathname + \'?refresh=1\'" class="admin-only btn-refresh"><i class="material-icons">refresh</i></button>'
    patch_file(index_path, old_btn_refresh, new_btn_refresh)

    # Settings link
    old_settings = '<a href="#" onclick="window.location.replace(\'/Config.html\'); return false;" style="background: #444; border-color: #666;"><i class="material-icons">settings</i></a>'
    new_settings = '<a href="#" onclick="window.location.replace(\'/Config.html\'); return false;" class="btn-settings"><i class="material-icons">settings</i></a>'
    patch_file(index_path, old_settings, new_settings)

    # Toolbar metadata toggle button
    old_meta_toggle = '<button id="metaToggleBtn" onclick="document.getElementById(\'metadataPanel\').classList.toggle(\'hidden\')" style="display: block; background: #17a2b8; color: white; border: none;"><i class="material-icons">info</i></button>'
    new_meta_toggle = '<button id="metaToggleBtn" onclick="document.getElementById(\'metadataPanel\').classList.toggle(\'hidden\')" class="btn-metadata-toggle"><i class="material-icons">info</i></button>'
    patch_file(index_path, old_meta_toggle, new_meta_toggle)

    # Toolbar save button
    old_save_btn = '<button id="saveBtn" onclick="saveNote()" class="admin-only" style="display: none; background: #28a745; color: white; border: none;"><i class="material-icons">save</i></button>'
    new_save_btn = '<button id="saveBtn" onclick="saveNote()" class="admin-only btn-save-note"><i class="material-icons">save</i></button>'
    patch_file(index_path, old_save_btn, new_save_btn)

    # Metadata panel
    old_metadata_panel = '<div id="metadataPanel" class="hidden" style="background: #e9ecef; padding: 15px; font-family: monospace; white-space: pre-wrap; border: 1px solid #ccc; margin-bottom: 10px; border-radius: 4px; font-size: 13px;"><!-- OMN_GO_METADATA_PANEL --></div>'
    new_metadata_panel = '<div id="metadataPanel" class="hidden metadata-panel"><!-- OMN_GO_METADATA_PANEL --></div>'
    patch_file(index_path, old_metadata_panel, new_metadata_panel)

    # Quick Note buttons row
    old_qn_buttons = '<div style="display: flex; gap: 10px;">\n            <button onclick="submitQuickNote()">Save</button>\n            <button onclick="document.getElementById(\'quickPanel\').classList.add(\'hidden\')" style="background: #dc3545;">Cancel</button>'
    new_qn_buttons = '<div class="modal-buttons-row">\n            <button onclick="submitQuickNote()">Save</button>\n            <button onclick="document.getElementById(\'quickPanel\').classList.add(\'hidden\')" class="btn-cancel">Cancel</button>'
    patch_file(index_path, old_qn_buttons, new_qn_buttons)

    # Bookmark buttons row
    old_bm_buttons = '<div style="display: flex; gap: 10px;">\n            <button onclick="submitBookmark()">Save</button>\n            <button onclick="document.getElementById(\'bmPanel\').classList.add(\'hidden\')" style="background: #dc3545;">Cancel</button>'
    new_bm_buttons = '<div class="modal-buttons-row">\n            <button onclick="submitBookmark()">Save</button>\n            <button onclick="document.getElementById(\'bmPanel\').classList.add(\'hidden\')" class="btn-cancel">Cancel</button>'
    patch_file(index_path, old_bm_buttons, new_bm_buttons)

    # Version footer
    old_footer = '<div id="omn-go-version-footer" style="position: fixed; bottom: 4px; right: 8px; font-size: 0.75rem; color: #888; z-index: 9999; opacity: 0.7; pointer-events: none;"></div>'
    new_footer = '<div id="omn-go-version-footer" class="version-footer"></div>'
    patch_file(index_path, old_footer, new_footer)

    # ========== 2. Append new CSS classes to omn-go-core.css ==========
    css_path = "backend/frontend/html/css/omn-go-core.css"
    new_css = r"""
/* ---------- Header Buttons (replacing inline styles) ---------- */
.btn-create-page {
    background: #17a2b8 !important;
    border-color: #17a2b8 !important;
}
.btn-refresh {
    background: #6c757d !important;
    border-color: #6c757d !important;
}
.btn-settings {
    background: #444 !important;
    border-color: #666 !important;
}

/* ---------- Toolbar Buttons ---------- */
.btn-metadata-toggle {
    display: block;
    background: #17a2b8;
    color: white;
    border: none;
}
.btn-save-note {
    display: none;                   /* hidden by default, shown via JS */
    background: #28a745;
    color: white;
    border: none;
}

/* ---------- Metadata Panel ---------- */
.metadata-panel {
    background: #e9ecef;
    padding: 15px;
    font-family: monospace;
    white-space: pre-wrap;
    border: 1px solid #ccc;
    margin-bottom: 10px;
    border-radius: 4px;
    font-size: 13px;
}

/* ---------- Modal Button Rows ---------- */
.modal-buttons-row {
    display: flex;
    gap: 10px;
}
.btn-cancel {
    background: #dc3545;
}

/* ---------- Version Footer ---------- */
.version-footer {
    position: fixed;
    bottom: 4px;
    right: 8px;
    font-size: 0.75rem;
    color: #888;
    z-index: 9999;
    opacity: 0.7;
    pointer-events: none;
}

/* ---------- Overlay default state (hidden, toggled by JS) ---------- */
.overlay {
    display: none;   /* overwritten by JS to flex when needed */
}
"""
    with open(css_path, "a", encoding="utf-8") as f:
        f.write(new_css)

    # ========== GIT COMMIT MESSAGE ==========
    commit = (
        "refactor(css): move remaining inline styles from index.html to omn-go-core.css\n\n"
        "Extracted inline styles from header buttons, toolbar buttons, metadata panel,\n"
        "modal button rows, cancel buttons, and version footer.  The overlay now defaults\n"
        "to hidden via CSS instead of an inline style attribute.\n\n"
        "New CSS classes: .btn-create-page, .btn-refresh, .btn-settings,\n"
        ".btn-metadata-toggle, .btn-save-note, .metadata-panel, .modal-buttons-row,\n"
        ".btn-cancel, .version-footer.\n\n"
        "Version bumped to 1.3.32"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()