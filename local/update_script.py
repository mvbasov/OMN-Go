#!/usr/bin/env python3
"""OMN-Go 1.3.29 → 1.3.30: fix Android external editor path, auto-create missing files."""

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
               'APP_VERSION = "1.3.29"',
               'APP_VERSION = "1.3.30"')
    patch_file("android/app/build.gradle",
               "versionCode 10329",
               "versionCode 10330")
    patch_file("android/app/build.gradle",
               'versionName "1.3.29"',
               'versionName "1.3.30"')

    # ========== FIX 1: Android MainActivity.java - correct file path for non-md ==========
    old_java = '''java.io.File file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/md/" + name + ".md");'''
    new_java = '''// Determine correct subdirectory and extension
                        java.io.File file;
                        if (name.endsWith(".md")) {
                            file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/md/" + name);
                        } else {
                            file = new java.io.File("/storage/emulated/0/Android/media/net.basov.omngo/html/" + name);
                        }'''
    patch_file("android/app/src/main/java/net/basov/omngo/MainActivity.java", old_java, new_java)

    # ========== FIX 2: server.go serveLazyEmbed edit handler - create file if missing ==========
    old_server_fnf = '''			if r.URL.Query().Get("edit") == "true" {
					relPath := strings.TrimPrefix(r.URL.Path, "/")

					// Honour external editor preference
					if !appConfig.UseInternalEd {
						http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
						return
					}

					rawContent, err := os.ReadFile(physPath)
					if err != nil {
						http.Error(w, "File not found", http.StatusNotFound)
						return
					}'''

    new_server_fnf = '''			if r.URL.Query().Get("edit") == "true" {
					relPath := strings.TrimPrefix(r.URL.Path, "/")

					// Honour external editor preference
					if !appConfig.UseInternalEd {
						http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
						return
					}

					rawContent, err := os.ReadFile(physPath)
					if err != nil {
						// File does not exist - create empty one and proceed
						os.MkdirAll(filepath.Dir(physPath), 0755)
						os.WriteFile(physPath, []byte{}, 0644)
						rawContent = []byte{}
					}'''

    patch_file("backend/server.go", old_server_fnf, new_server_fnf)

    # ========== FIX 3: handlers.go serveFrontend edit handler - create file if missing ==========
    old_handler_fnf = '''		if data, err := os.ReadFile(filePath); err == nil {
			rawContent = data
		}'''
    new_handler_fnf = '''		if data, err := os.ReadFile(filePath); err == nil {
			rawContent = data
		} else {
			// File does not exist - create empty one and proceed
			os.MkdirAll(filepath.Dir(filePath), 0755)
			os.WriteFile(filePath, []byte{}, 0644)
			rawContent = []byte{}
		}'''

    patch_file("backend/handlers.go", old_handler_fnf, new_handler_fnf)

    # ========== GIT COMMIT MESSAGE ==========
    commit = (
        "fix(android,editor): external editor now opens correct file; auto-create missing files\n\n"
        "- Android: MainActivity no longer appends .md to non-markdown files and\n"
        "  uses the html/ directory for .js/.css/.json etc.\n"
        "- Both internal edit handlers now create the target file (and parent dirs)\n"
        "  if it does not exist, instead of returning 404.\n\n"
        "Version bumped to 1.3.30"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()