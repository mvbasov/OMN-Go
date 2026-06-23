#!/usr/bin/env python3
"""OMN-Go 1.3.26 → 1.3.27: fix ?edit=true for /js/, /css/, /json/ routes."""

import os

def patch_file(path, old, new):
    with open(path, "r", encoding="utf-8") as f:
        text = f.read()
    if old not in text:
        raise ValueError(f"❌ Patch target not found in {path}:\n{old}")
    text = text.replace(old, new, 1)
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)

def update_application():
    # ========== VERSION BUMPS ==========
    patch_file("backend/version.go",
               'APP_VERSION = "1.3.26"',
               'APP_VERSION = "1.3.27"')
    patch_file("android/app/build.gradle",
               "versionCode 10326",
               "versionCode 10327")
    patch_file("android/app/build.gradle",
               'versionName "1.3.26"',
               'versionName "1.3.27"')

    # ========== SERVER.GO: add "strings" import ==========
    old_imports = '''import (
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)'''
    new_imports = '''import (
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)'''
    patch_file("backend/server.go", old_imports, new_imports)

    # ========== SERVER.GO: replace serveLazyEmbed to support ?edit=true ==========
    old_lazy = '''		serveLazyEmbed := func() http.Handler {
			physicalDir := filepath.Join(storageDir, "html")
			fsHandler := http.FileServer(http.Dir(physicalDir))

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Calculate physical path
				physPath := filepath.Join(physicalDir, filepath.Clean(r.URL.Path))

				// Lazy Extraction: Check if file exists on disk, if not, pull from embedFS
				if _, err := os.Stat(physPath); os.IsNotExist(err) {
					embedPath := "frontend/html" + filepath.ToSlash(filepath.Clean(r.URL.Path))
					if data, err := staticFS.ReadFile(embedPath); err == nil {
						os.MkdirAll(filepath.Dir(physPath), 0755)
						os.WriteFile(physPath, data, 0644)
					}
				}

				// Serve the file dynamically from the physical directory
				fsHandler.ServeHTTP(w, r)
			})
		}'''

    new_lazy = '''		serveLazyEmbed := func() http.Handler {
			physicalDir := filepath.Join(storageDir, "html")
			fsHandler := http.FileServer(http.Dir(physicalDir))

			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Calculate physical path
				physPath := filepath.Join(physicalDir, filepath.Clean(r.URL.Path))

				// Lazy Extraction: Check if file exists on disk, if not, pull from embedFS
				if _, err := os.Stat(physPath); os.IsNotExist(err) {
					embedPath := "frontend/html" + filepath.ToSlash(filepath.Clean(r.URL.Path))
					if data, err := staticFS.ReadFile(embedPath); err == nil {
						os.MkdirAll(filepath.Dir(physPath), 0755)
						os.WriteFile(physPath, data, 0644)
					}
				}

				// Check for edit mode before serving static file
				if r.URL.Query().Get("edit") == "true" {
					rawContent, err := os.ReadFile(physPath)
					if err != nil {
						http.Error(w, "File not found", http.StatusNotFound)
						return
					}
					relPath := strings.TrimPrefix(r.URL.Path, "/")
					escapedContent := htmlEscape(string(rawContent))
					customBody := "<pre style=\\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\\">" + escapedContent + "</pre>"
					compiled := compilePageWithBody(relPath, []byte{}, customBody)
					scriptInjection := "<script>var IS_MARKDOWN = false;</script>"
					compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\\n</head>", 1))
					w.Header().Set("Content-Type", "text/html")
					w.Write(compiled)
					return
				}

				// Serve the file dynamically from the physical directory
				fsHandler.ServeHTTP(w, r)
			})
		}'''

    patch_file("backend/server.go", old_lazy, new_lazy)

    # ========== GIT COMMIT MESSAGE ==========
    commit = (
        "fix(core): ?edit=true now triggers editor for /js/, /css/, /json/ routes\n\n"
        "Static files served via serveLazyEmbed (which handles /js/, /css/, /json/\n"
        "prefixes) ignored the ?edit=true query parameter because those routes never\n"
        "reached the edit handler in serveFrontend.  The serveLazyEmbed handler now\n"
        "checks for ?edit=true before serving the raw file and, if present, renders\n"
        "the internal editor with the raw file content.\n\n"
        "Also added the missing \"strings\" import to server.go.\n\n"
        "Version bumped to 1.3.27"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()