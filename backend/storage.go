package backend

import (
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// initStorage computes a.StorageDir and prepares its layout. overrideDir,
// when non-empty, is used as-is (see StartServer's doc comment for why
// Android needs this instead of the runtime.GOOS branch below - its
// applicationId, and therefore its external media directory, differs
// between the standard and fdroid product flavors, which this package
// cannot know on its own).
func (a *App) initStorage(overrideDir string) {
	if overrideDir != "" {
		a.StorageDir = overrideDir
	} else if runtime.GOOS == "android" {
		// Fallback only: reached if a future Android caller ever starts
		// the server without passing its own directory. Matches the
		// standard flavor's applicationId, not fdroid's.
		a.StorageDir = "/storage/emulated/0/Android/media/net.basov.omngo"
	} else {
		a.StorageDir = "./data"
	}

	// 1. Create Isolated Storage
	if err := os.MkdirAll(a.StorageDir, 0755); err != nil {
		log.Printf("Failed to create storage: %v", err)
	}

	mdDir := filepath.Join(a.StorageDir, "md")
	os.MkdirAll(mdDir, 0755)

	htmlDir := filepath.Join(a.StorageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	// Migrate legacy root md files recursively
	files, _ := filepath.Glob(filepath.Join(a.StorageDir, "*.md"))
	for _, f := range files {
		os.Rename(f, filepath.Join(mdDir, filepath.Base(f)))
	}

	// Migrate static directories inside html/
	dirsToMove := []string{"images", "user_json", "css", "js", "json", "fonts"}
	for _, d := range dirsToMove {
		oldPath := filepath.Join(a.StorageDir, d)
		newPath := filepath.Join(htmlDir, d)
		if stat, err := os.Stat(oldPath); err == nil && stat.IsDir() {
			os.Rename(oldPath, newPath)
		}
	}

	// Bring previously-extracted embedded assets (html/js, html/css, ...)
	// up to date with this build. Runs synchronously so no request is ever
	// served a stale asset; a no-op unless APP_VERSION changed since the
	// last start (see assets.go).
	a.refreshEmbeddedAssets()

	// 2. Init Config
	a.loadConfig(a.StorageDir)

	// The index struct exists from the start; it stays empty (and free) until
	// global search is switched on.
	a.search = &searchIndex{}

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

	// The start page's two large buttons are markup in the note, not page
	// chrome (see the .omn-start-buttons block in omn-go-core.css), so this
	// fallback carries them too - an install that lands here must still get
	// the same two entry points as the embedded Welcome.md.
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
	// A plain file kept beside a note (md/log.txt) is copied into html/,
	// which is where its URL resolves - see note_files.go. Synchronous, and
	// before the precompile below: the walk reads nothing until it finds a
	// file to copy, and a link tapped in the first second after start must
	// not race the copy that makes it work.
	a.syncNoteFilesToHTML()

	// The incoming index (note_exchange.go), created when it is absent, the
	// same way the four starter notes above are. It has to exist before the
	// first note arrives: on the desktop the receive box ON that page is how
	// a note arrives at all.
	if err := a.ensureIncomingIndex(time.Now()); err != nil {
		log.Printf("initStorage: incoming index: %v", err)
	}

	// Precompile all notes to data/html/ at startup in the background
	go a.precompileAllPages()
}

func (a *App) precompileAllPages() {
	mdDir := filepath.Join(a.StorageDir, "md")
	htmlDir := filepath.Join(a.StorageDir, "html")
	os.MkdirAll(htmlDir, 0755)

	// This runs in a background goroutine at startup, so a note opened before
	// it finishes is compiled on demand by serveHTMLPage instead - the user
	// waits, with no way to tell why. Logging the run makes that visible on
	// the /api/logs stream rather than leaving it invisible.
	log.Printf("[precompile] Compiling notes in background")
	started := time.Now()
	compiled := 0

	filepath.Walk(mdDir, func(f string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(f, ".md") {
			content, err := os.ReadFile(f)
			if err == nil {
				relPath, _ := filepath.Rel(mdDir, f)
				name := strings.TrimSuffix(filepath.ToSlash(relPath), ".md")
				// renderAndCache is the single cache writer (render_cache.go).
				if _, err := a.renderAndCache(name, content); err != nil {
					log.Printf("precompileAllPages: %v", err)
				} else {
					compiled++
				}
			}
		}
		return nil
	})

	log.Printf("[precompile] Compiled %d notes in %s", compiled,
		time.Since(started).Round(time.Millisecond))

	// After every note is compiled, (re)generate the Tags index so
	// html/OMNGoTags.html exists and is current in the offline artifact even if
	// it is never viewed (it is reachable only via tag pills). Runs here, at the
	// end of the background startup precompile, so it never blocks server start.
	// See tags.go.
	if err := a.generateTagsPage(); err != nil {
		log.Printf("precompileAllPages: tags: %v", err)
	}

	// Warm the search index, if the user asked for one. Deliberately last and
	// on this same background goroutine: it is the cheapest of the three
	// startup passes (masks and trigrams, no markdown rendering), and running
	// it here means the first search after launch is instant instead of
	// paying for the build inside a request.
	if a.GetConfig().SearchEnabled {
		a.rebuildSearchIndex()
	}
}
