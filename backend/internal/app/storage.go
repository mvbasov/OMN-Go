package app

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"net.basov.omngo/backend/frontend"
	"net.basov.omngo/backend/internal/logx"
	"net.basov.omngo/backend/internal/search"
)

// initStorage sets a.StorageDir and makes its layout. A non-empty overrideDir
// wins. The applicationId, and thus the media directory, differs between the
// standard and the fdroid flavor, and this package cannot know it. See
// StartServer.
func (a *App) initStorage(overrideDir string) {
	if overrideDir != "" {
		a.StorageDir = overrideDir
	} else if runtime.GOOS == "android" {
		// This is a fallback for an Android caller that passes no directory.
		// It uses the applicationId of the standard flavor.
		a.StorageDir = "/storage/emulated/0/Android/media/net.basov.omngo"
	} else {
		a.StorageDir = "./data"
	}

	// 1. Make the storage directory.
	if err := os.MkdirAll(a.StorageDir, 0755); err != nil {
		a.log(logx.Storage).Errf("Failed to create storage: %v", err)
	}

	mdDir := a.layout().MD()
	os.MkdirAll(mdDir, 0755)

	htmlDir := a.layout().HTML()
	os.MkdirAll(htmlDir, 0755)

	// Move the .md files at the root of an old storage layout into md/.
	files, _ := filepath.Glob(a.layout().File("*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// Move the static directories of an old layout into html/.
	dirsToMove := []string{"images", "user_json", "css", "js", "json", "fonts"}
	for _, d := range dirsToMove {
		oldPath := a.layout().File(d)
		newPath := filepath.Join(htmlDir, d)
		if stat, err := os.Stat(oldPath); err == nil && stat.IsDir() {
			os.Rename(oldPath, newPath)
		}
	}

	// Bring the version-dependent files up to date with this build. See
	// internal/storage/assets.go. It runs before the first request, and it does
	// nothing while APP_VERSION stays the same.
	a.refreshEmbeddedAssets()

	// 2. Read the configuration.
	setStartStage("Reading the settings")
	a.loadConfig(a.layout().Config())

	// The index struct exists from the start. It stays empty until a person
	// turns global search on.
	a.search = &search.Index{}
	// The hooks of connectGroups need the index, thus they follow it.
	a.connectGroups()

	// 3. Extract each embedded starter note at the top of frontend/md that is
	// absent.
	if entries, err := frontend.Static.ReadDir("md"); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".md") {
				p := filepath.Join(mdDir, entry.Name())
				if _, err := os.Stat(p); os.IsNotExist(err) {
					if data, err := frontend.Static.ReadFile("md/" + entry.Name()); err == nil {
						os.WriteFile(p, data, 0644)
					}
				}
			}
		}
	}

	// 4. Write a fallback note when the embed has no copy.
	initDefaultPage := func(fileName, defaultContent string) {
		p := filepath.Join(mdDir, fileName)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			os.WriteFile(p, []byte(defaultContent), 0644)
		}
	}

	// The two large buttons of the start page are markup in the note, and not
	// page chrome. See .omn-start-buttons in omn-go-core.css. The fallback
	// thus holds them too.
	initDefaultPage("Welcome.md", `Title: Welcome
Date: 2026-06-14 12:00:00
Category: System

<div class="omn-start-buttons">
<a class="omn-start-button" href="QuickNotes">
<i class="material-icons omn-start-icon">insert_comment</i>
<span class="omn-start-text"><span class="omn-start-label">My Quick Notes</span><span class="omn-start-hint">Write it down now. Sort it later.</span></span>
</a>
<a class="omn-start-button omn-start-button-bookmarks" href="Bookmarks">
<i class="material-icons omn-start-icon">bookmark</i>
<span class="omn-start-text"><span class="omn-start-label">My Bookmarks</span><span class="omn-start-hint">Keep a link. Find it again.</span></span>
</a>
</div>

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
	// Copy each plain file beside a note into html/. See
	// internal/storage/note_files.go. This runs before the start ends, because a
	// link tap in the first second must find the copy.
	a.syncNoteFilesToHTML()

	// Make the incoming index when it is absent. See
	// internal/exchange/exchange.go. On the desktop, the receive box on that page
	// is how a note arrives.
	if err := a.ensureIncomingIndex(time.Now()); err != nil {
		a.log(logx.Storage).Errf("initStorage: incoming index: %v", err)
	}

	// Compile each note into html/ in the background.
	go a.precompileAllPages()
}

func (a *App) precompileAllPages() {
	mdDir := a.layout().MD()
	htmlDir := a.layout().HTML()
	os.MkdirAll(htmlDir, 0755)

	// This runs in the background at the start. serveHTMLPage compiles a note
	// that a person opens before this pass ends, and the person waits. The
	// log lines show that wait on /api/logs.
	a.log(logx.Precompile).Debugf("Compiling notes in background")
	started := time.Now()
	compiled := 0

	filepath.Walk(mdDir, func(f string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(f, ".md") {
			content, err := os.ReadFile(f)
			if err == nil {
				relPath, _ := filepath.Rel(mdDir, f)
				name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
				// renderAndCache is the one cache writer. See
				// internal/render/cache.go.
				if _, err := a.renderAndCache(name, content); err != nil {
					a.log(logx.Precompile).Errf("precompileAllPages: %v", err)
				} else {
					compiled++
				}
			}
		}
		return nil
	})

	a.log(logx.Precompile).Infof("Compiled %d notes in %s", compiled,
		time.Since(started).Round(time.Millisecond))

	// After each note, make the Tags page again. html/OMNGoTags.html then
	// exists, and it is current in the offline copy, also when nobody opens
	// it. This runs in the background, thus it never blocks the start.
	if err := a.generateTagsPage(); err != nil {
		a.log(logx.Tags).Errf("precompileAllPages: tags: %v", err)
	}

	// Build the search index last, when the person turned global search on.
	// It renders no markdown, thus it is cheap. The first search after the
	// start then needs no build.
	if a.config.Get().SearchEnabled {
		a.rebuildSearchIndex()
	}
}
