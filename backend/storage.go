package backend

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

var storageDir string

func initStorage() {
	if runtime.GOOS == "android" {
		storageDir = "/storage/emulated/0/Android/media/net.basov.omngo"
	} else {
		storageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		log.Printf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(storageDir, "md")
	os.MkdirAll(mdDir, 0755)

	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	// Migrate legacy root md files recursively
	files, _ := filepath.Glob(filepath.Join(storageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// Migrate static directories inside html/
	dirsToMove := []string{"images", "user_json", "css", "js", "json", "fonts"}
	for _, d := range dirsToMove {
		oldPath := filepath.Join(storageDir, d)
		newPath := filepath.Join(htmlDir, d)
		if stat, err := os.Stat(oldPath); err == nil && stat.IsDir() {
			os.Rename(oldPath, newPath)
		}
	}

	// 2. Init Config
	loadConfig(storageDir)

	// 3. Extract all embedded MD files first
	if entries, err := staticFS.ReadDir("frontend/md"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				p := filepath.Join(mdDir, entry.Name())
				if _, err := os.Stat(p); os.IsNotExist(err) {
					if data, err := staticFS.ReadFile("frontend/md/" + entry.Name()); err == nil {
						os.WriteFile(p, data, 0644)
					}
				}
			}
		}
	}

	// 4. Init Default Notes fallback (if embedFS fails)
	initDefaultPage := func(fileName, defaultContent string) {
		p := filepath.Join(mdDir, fileName)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			os.WriteFile(p, []byte(defaultContent), 0644)
		}
	}

	initDefaultPage("Welcome.md", `Title: Welcome
Date: 2026-06-14 12:00:00
Category: System

Yo! Welcome to OMN-Go! Start editing.

- [Help](Welcome)
- [Scripting Rules](ScriptRules.md)
- [Bookmarks](Bookmarks)
- [Quick Notes](QuickNotes)`)

	initDefaultPage("ScriptRules.md", `Title: JS Scripting Rules
Date: 2026-06-15
Category: System

# JavaScript Guidelines for OMN-Go

Because OMN-Go is rendered server-side, keep scripts wrapped in block scopes.`)

	initDefaultPage("QuickNotes.md", `Title: Quick Notes
Date: 2026-06-14 12:00:00
Category: Log

`)

	initDefaultPage("Bookmarks.md", `Title: Incoming bookmarks
Date: 2026-06-15 20:00:00
Author: 
Tags: Bookmarks

<script>bookmarks = [
<!-- Don't edit body below this line -->
];
</script>`)
	// Precompile all notes to data/html/ at startup in the background
	go precompileAllPages()
}

func precompileAllPages() {
	mdDir := filepath.Join(storageDir, "md")
	htmlDir := filepath.Join(storageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	filepath.Walk(mdDir, func(f string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(f, ".md") {
			content, err := os.ReadFile(f)
			if err == nil {
				relPath, _ := filepath.Rel(mdDir, f)
				name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
				compiled := compilePage(name, content)
				htmlPath := filepath.Join(htmlDir, filepath.Clean(name+".html"))
				os.MkdirAll(filepath.Dir(htmlPath), 0755)
				os.WriteFile(htmlPath, compiled, 0644)
			}
		}
		return nil
	})
}

