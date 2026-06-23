#!/usr/bin/env python3
"""OMN-Go 1.3.27 → 1.3.28: fix ?edit=true — external editor, textarea population, auto-edit."""

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
               'APP_VERSION = "1.3.27"',
               'APP_VERSION = "1.3.28"')
    patch_file("android/app/build.gradle",
               "versionCode 10327",
               "versionCode 10328")
    patch_file("android/app/build.gradle",
               'versionName "1.3.27"',
               'versionName "1.3.28"')

    # ===================================================================
    # FIX 1: serveLazyEmbed in server.go — proper edit handler
    # ===================================================================
    old_lazy_edit = '''			// Check for edit mode before serving static file
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
				}'''

    new_lazy_edit = '''			// Check for edit mode before serving static file
				if r.URL.Query().Get("edit") == "true" {
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
					}
					escapedContent := htmlEscape(string(rawContent))
					customBody := "<pre style=\\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\\">" + escapedContent + "</pre>"
					// Pass raw content as mdContent so the textarea is populated
					compiled := compilePageWithBody(relPath, rawContent, customBody)
					scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode===\\'function\\') toggleMode(); }, 120);</script>"
					compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\\n</head>", 1))
					w.Header().Set("Content-Type", "text/html")
					w.Write(compiled)
					return
				}'''

    patch_file("backend/server.go", old_lazy_edit, new_lazy_edit)

    # Also need "net/url" import in server.go for url.QueryEscape
    old_server_import = '''import (
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)'''
    new_server_import = '''import (
	"embed"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)'''
    patch_file("backend/server.go", old_server_import, new_server_import)

    # ===================================================================
    # FIX 2: serveFrontend edit handler in handlers.go — same fixes
    # ===================================================================
    old_frontend_edit = '''	// Serve editor for any file when ?edit=true
	if r.URL.Query().Get("edit") == "true" {
		relPath := strings.TrimPrefix(r.URL.Path, "/")
		var filePath string
		var rawContent []byte
		if strings.HasSuffix(relPath, ".md") {
			filePath = filepath.Join(storageDir, "md", filepath.Clean(relPath))
		} else {
			filePath = filepath.Join(storageDir, "html", filepath.Clean(relPath))
		}
		if data, err := os.ReadFile(filePath); err == nil {
			rawContent = data
		}
		// Show raw content in preview, leave editor empty – loaded on demand via API
		escapedContent := htmlEscape(string(rawContent))
		customBody := "<pre style=\\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\\">" + escapedContent + "</pre>"
		compiled := compilePageWithBody(relPath, []byte{}, customBody)
		// Tell the frontend this is not a Pelican markdown page
		scriptInjection := "<script>var IS_MARKDOWN = false;</script>"
		compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\\n</head>", 1))
		w.Header().Set("Content-Type", "text/html")
		w.Write(compiled)
		return
	}'''

    new_frontend_edit = '''	// Serve editor for any file when ?edit=true
	if r.URL.Query().Get("edit") == "true" {
		relPath := strings.TrimPrefix(r.URL.Path, "/")

		// Honour external editor preference
		if !appConfig.UseInternalEd {
			http.Redirect(w, r, "/api/edit-external?name="+url.QueryEscape(relPath), http.StatusSeeOther)
			return
		}

		var filePath string
		var rawContent []byte
		if strings.HasSuffix(relPath, ".md") {
			filePath = filepath.Join(storageDir, "md", filepath.Clean(relPath))
		} else {
			filePath = filepath.Join(storageDir, "html", filepath.Clean(relPath))
		}
		if data, err := os.ReadFile(filePath); err == nil {
			rawContent = data
		}
		// Show raw content in preview and populate textarea
		escapedContent := htmlEscape(string(rawContent))
		customBody := "<pre style=\\"white-space: pre-wrap; word-wrap: break-word; background: #f5f5f5; padding: 10px; border-radius: 4px;\\">" + escapedContent + "</pre>"
		compiled := compilePageWithBody(relPath, rawContent, customBody)
		// Tell the frontend this is not a Pelican markdown page + auto-enter edit mode
		scriptInjection := "<script>var IS_MARKDOWN = false; setTimeout(function(){ if(typeof toggleMode===\\'function\\') toggleMode(); }, 120);</script>"
		compiled = []byte(strings.Replace(string(compiled), "</head>", scriptInjection+"\\n</head>", 1))
		w.Header().Set("Content-Type", "text/html")
		w.Write(compiled)
		return
	}'''

    patch_file("backend/handlers.go", old_frontend_edit, new_frontend_edit)

    # handlers.go also needs "net/url" import
    old_handler_import = '''import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)'''
    new_handler_import = '''import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)'''
    patch_file("backend/handlers.go", old_handler_import, new_handler_import)

    # ========== GIT COMMIT MESSAGE ==========
    commit = (
        "fix(core): ?edit=true now honours external editor, populates textarea, auto-enters edit mode\n\n"
        "Three fixes for the edit-via-query-parameter feature:\n"
        "  1. When UseInternalEd is false, ?edit=true now redirects to\n"
        "     /api/edit-external?name=... instead of rendering the internal editor.\n"
        "  2. The textarea is now pre-filled with the actual file content (was empty\n"
        "     because compilePageWithBody received an empty mdContent).\n"
        "  3. A small setTimeout injects a call to toggleMode() so the editor opens\n"
        "     immediately instead of requiring a manual click on the edit button.\n\n"
        "Both serveLazyEmbed (/js/, /css/, /json/ routes) and the serveFrontend\n"
        "fallback handler received the same fixes.  Added \"net/url\" import to\n"
        "server.go and handlers.go for url.QueryEscape.\n\n"
        "Version bumped to 1.3.28"
    )
    print(f"\n[GIT_COMMIT_MESSAGE]\n{commit}\n[/GIT_COMMIT_MESSAGE]")

if __name__ == "__main__":
    update_application()