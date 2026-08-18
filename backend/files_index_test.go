package backend

// Tests for the file index page.
//
// Four things here are worth more than the rest, because each is a claim the
// page makes that nothing else in the app would catch if it stopped being
// true:
//
//   - it shows ONE directory of ONE tree. The whole design turns on that, and
//     "flat list of everything" is what it drifted into twice while being
//     planned.
//   - one NAME has one ROW. Until 26.08.53 a name that both shipped and sat on
//     the device was printed twice, in two sections, with nothing to pair them.
//   - it never WRITES. The obvious way to resolve an embedded path is
//     materializeAsset, which extracts the file as a side effect - a listing
//     built that way would silently defeat lazy extraction for all 70 of them.
//   - templates never appear. That is currently guaranteed by the embeds being
//     separate rather than by any code here, which is exactly the kind of
//     guarantee that evaporates without a test.

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func getFilesPage(t *testing.T, a *App, query string) *httptest.ResponseRecorder {
	t.Helper()
	target := "/OMNGoFiles.html"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.RemoteAddr = "127.0.0.1:54321" // local: always the owner
	rec := httptest.NewRecorder()
	a.serveFilesPage(rec, req)
	return rec
}

// served asks for one directory of the Served tree, which is the tree most of
// these tests are about.
func served(t *testing.T, a *App, dir string) string {
	t.Helper()
	q := "tree=served"
	if dir != "" {
		q += "&dir=" + dir
	}
	return getFilesPage(t, a, q).Body.String()
}

// writeDiskFile drops a file under StorageDir/html.
func writeDiskFile(t *testing.T, a *App, rel, body string) {
	t.Helper()
	writeStorageFile(t, a, "html", rel, body)
}

// writeNoteFile drops a file under StorageDir/md.
func writeNoteFile(t *testing.T, a *App, rel, body string) {
	t.Helper()
	writeStorageFile(t, a, "md", rel, body)
}

func writeStorageFile(t *testing.T, a *App, sub, rel, body string) {
	t.Helper()
	full := filepath.Join(a.StorageDir, sub, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

// ----------------------------------------------------------------------
// The first screen
// ----------------------------------------------------------------------

// Three buttons and nothing else. A listing under them was the shape that made
// the old page unreadable: two kinds of file on one screen with no way to tell
// which question each answered.
func TestFilesPage_FirstScreenIsThreeTrees(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/mine.js", "// mine")

	body := getFilesPage(t, a, "").Body.String()
	for _, want := range []string{"Bundled", "Served", "Source",
		"tree=bundled", "tree=served", "tree=source"} {
		if !strings.Contains(body, want) {
			t.Errorf("the first screen is missing %q", want)
		}
	}
	if strings.Contains(body, "mine.js") {
		t.Error("the first screen listed a file; it carries three buttons only")
	}
	if strings.Contains(body, `class="files-row"`) {
		t.Error("the first screen carries a listing row")
	}
}

// A link written before 26.08.54 has a dir and no tree. It must still land on
// the tree it meant.
func TestNormalizeFilesTree(t *testing.T) {
	cases := []struct{ raw, dir, want string }{
		{"", "", ""},
		{"", "js/", filesTreeServed},
		{"served", "", filesTreeServed},
		{"bundled", "", filesTreeBundled},
		{"source", "", filesTreeSource},
		{"nonsense", "", ""},
		{"nonsense", "js/", filesTreeServed},
	}
	for _, c := range cases {
		if got := normalizeFilesTree(c.raw, c.dir); got != c.want {
			t.Errorf("normalizeFilesTree(%q, %q) = %q, want %q", c.raw, c.dir, got, c.want)
		}
	}
}

// ----------------------------------------------------------------------
// One directory at a time
// ----------------------------------------------------------------------

// THE test for this page. A listing that shows every file at every depth is a
// different feature, and it is the one this drifted into while being designed.
func TestFilesPage_ShowsOneDirectoryAtATime(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/mine.js", "// mine")
	writeDiskFile(t, a, "css/mine.css", "body{}")
	writeDiskFile(t, a, "Welcome.html", "<html></html>")

	root := served(t, a, "")
	if !strings.Contains(root, ">js/<") {
		t.Error("the root does not list js/ as a directory")
	}
	if strings.Contains(root, "mine.js") {
		t.Error("the root listed a file from INSIDE js/; this page shows one level")
	}
	if !strings.Contains(root, "Welcome.html") {
		t.Error("a file at the root level is missing")
	}

	js := served(t, a, "js%2F")
	if !strings.Contains(js, "mine.js") {
		t.Error("?dir=js/ does not list the file in js/")
	}
	if strings.Contains(js, "mine.css") {
		t.Error("?dir=js/ leaked a file from css/")
	}
}

// Following two directory links has to work, and the breadcrumb has to offer
// every ancestor - otherwise a nested note tree is reachable but not leavable.
func TestFilesPage_NavigatesAndOffersAWayBack(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "Test/OMN-Go/Fetch.html", "<html></html>")

	root := served(t, a, "")
	if !strings.Contains(root, "dir=Test%2F") {
		t.Fatalf("no link into Test/ from the root")
	}

	mid := served(t, a, "Test%2F")
	if !strings.Contains(mid, "dir=Test%2FOMN-Go%2F") {
		t.Fatalf("no link into Test/OMN-Go/ from Test/")
	}

	leaf := served(t, a, "Test%2FOMN-Go%2F")
	if !strings.Contains(leaf, "Fetch.html") {
		t.Error("the leaf directory does not list its file")
	}
	// The breadcrumb must offer BOTH ancestors, not just the immediate one.
	for _, want := range []string{`href="/OMNGoFiles.html?tree=served"`, "dir=Test%2F"} {
		if !strings.Contains(leaf, want) {
			t.Errorf("breadcrumb is missing %q; a leaf you cannot leave is a dead end", want)
		}
	}
}

// The crumb reads as the path it is. Until 26.08.53 each label carried a slash
// AND a separator span went between two labels, so html/js/ read "html/ / js/".
func TestFilesCrumbs_HaveNoDoubledSlash(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/mine.js", "// mine")

	body := served(t, a, "js%2F")
	if strings.Contains(body, "files-crumb-sep") {
		t.Error("the crumb still writes a separator between two labels")
	}
	if strings.Contains(body, "html/ / js/") || strings.Contains(body, "html//js/") {
		t.Error("the crumb shows a doubled slash")
	}
	if !strings.Contains(body, `<span class="files-crumb-here">js/</span>`) {
		t.Error("the last crumb is not the directory in view")
	}

	crumbs := filesCrumbs(filesTreeSource, "Test/OMN-Go/")
	if len(crumbs) != 3 || crumbs[0].Label != "md/" || crumbs[2].Label != "OMN-Go/" {
		t.Errorf("filesCrumbs gave %+v", crumbs)
	}
}

// A directory row answers "how big is this subtree", not "how many entries are
// immediately inside" - which is rarely the question a file index is opened to
// answer.
func TestFilesPage_DirectoryTotalsAreRecursive(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "Test/a/one.html", "12345")
	writeDiskFile(t, a, "Test/b/two.html", "12345")

	dirs, _, _, _ := foldToDir(a.treeEntries(filesTreeServed), "")
	var test *filesDirRow
	for i := range dirs {
		if dirs[i].Name == "Test" {
			test = &dirs[i]
		}
	}
	if test == nil {
		t.Fatal("Test/ missing")
	}
	if test.Files != 2 {
		t.Errorf("Test/ reports %d files; both live one level further down", test.Files)
	}
	if test.Bytes != 10 {
		t.Errorf("Test/ reports %d bytes, want 10", test.Bytes)
	}
}

// ----------------------------------------------------------------------
// One name, one row
// ----------------------------------------------------------------------

// The reason the page was rebuilt. A file that ships AND sits on the device is
// one row that states the relation, not two rows in two sections.
func TestFilesPage_OneNameOneRow(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/omn-go-core.js", "// changed by hand")

	body := served(t, a, "js%2F")
	if n := strings.Count(body, ">omn-go-core.js<"); n != 1 {
		t.Errorf("omn-go-core.js has %d rows, want 1", n)
	}
	for _, gone := range []string{"Embedded in the application", "On this device"} {
		if strings.Contains(body, gone) {
			t.Errorf("the two-section layout is still rendered: %q", gone)
		}
	}
}

// The words that survive, and the colour that says what happens to the file.
//
// The rule since 26.08.55: a row speaks only when the application is
// involved. On a real installation nearly every file is the user's, and a
// word on each of those rows buried the words that matter.
func TestFilesPage_StatesAndColours(t *testing.T) {
	a := newTestApp(t)
	// Shipped, extracted, and edited: the size differs, so no byte read is
	// needed to know. It is app-owned, thus the change is the one that is lost.
	writeDiskFile(t, a, "js/omn-go-core.js", "// a hand edit")
	// Shipped, extracted, untouched: written from the embed itself. This row
	// must say NOTHING.
	sameBody, err := staticFS.ReadFile("frontend/html/js/local_counter.js")
	if err != nil {
		t.Fatal(err)
	}
	writeDiskFile(t, a, "js/local_counter.js", string(sameBody))
	// Never shipped: also silent.
	writeDiskFile(t, a, "js/mine.js", "// mine")

	body := served(t, a, "js%2F")
	for _, want := range []string{
		"changed here", "not extracted",
		filesColorAlert, filesColorApp, filesColorPlain,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the listing never says %q", want)
		}
	}
	// The word is the fact; the colour is a hint. app-owned is spelled out.
	if !strings.Contains(body, ">app-owned<") {
		t.Error("no row spells out app-owned")
	}
	// The words of 26.08.54 that said "ordinary" are gone.
	for _, gone := range []string{"as shipped", "yours", "compiled"} {
		if strings.Contains(body, gone) {
			t.Errorf("the listing still says %q; the ordinary case must be silent", gone)
		}
	}

	// Read the two silent rows directly, so the assertion is about the row and
	// not about a substring of the page.
	for _, name := range []string{"local_counter.js", "mine.js"} {
		if row := rowOfName(t, a, filesTreeServed, "js/", name); row.State != "" {
			t.Errorf("%s says %q; nothing is at stake, so it must say nothing", name, row.State)
		}
	}
}

// A page that OMN-Go compiled from a note, a note the user wrote, and an
// upload are the ordinary case of their tree. Each one says nothing at all.
func TestFilesPage_OrdinaryFilesAreSilent(t *testing.T) {
	a := newTestApp(t)
	writeNoteFile(t, a, "Log.md", "Title: Log\n\nbody\n")
	writeDiskFile(t, a, "Log.html", "<html></html>")
	writeDiskFile(t, a, "images/photo.png", "\x89PNG")

	for _, name := range []string{"Log.html"} {
		if row := rowOfName(t, a, filesTreeServed, "", name); row.State != "" {
			t.Errorf("%s says %q; a compiled page is the ordinary case", name, row.State)
		}
	}
	if row := rowOfName(t, a, filesTreeSource, "", "Log.md"); row.State != "" {
		t.Errorf("a note you wrote says %q", row.State)
	}
	if row := rowOfName(t, a, filesTreeServed, "images/", "photo.png"); row.State != "" {
		t.Errorf("an upload says %q", row.State)
	}
}

// The .txt pair of note_files.go: md/x.txt is the file, html/x.txt is a copy.
// A pair that agrees says NOTHING - that is the ordinary state of every text
// file beside a note. Only a pair that disagrees speaks, and the two
// directions do not read the same, because the remedies differ.
func TestFilesPage_TxtMirror(t *testing.T) {
	a := newTestApp(t)
	old := time.Now().Add(-2 * time.Hour)

	// In step: same size, same modification time - which is what
	// copyFileWithTime produces on purpose.
	writeNoteFile(t, a, "log.txt", "one")
	writeDiskFile(t, a, "log.txt", "one")
	touch(t, filepath.Join(a.StorageDir, "md", "log.txt"), old)
	touch(t, filepath.Join(a.StorageDir, "html", "log.txt"), old)

	// The copy is newer than the file: an editor outside OMN-Go wrote html/
	// and gave OMN-Go no signal. Nothing repairs this by itself.
	writeNoteFile(t, a, "ahead.txt", "one")
	writeDiskFile(t, a, "ahead.txt", "one and more")
	touch(t, filepath.Join(a.StorageDir, "md", "ahead.txt"), old)

	// The copy is older: the next start repairs it.
	writeNoteFile(t, a, "behind.txt", "two and more")
	writeDiskFile(t, a, "behind.txt", "two")
	touch(t, filepath.Join(a.StorageDir, "html", "behind.txt"), old)

	body := served(t, a, "")
	if strings.Contains(body, "copy of md/log.txt") || strings.Contains(body, "log.txt</a></span><span class=\"files-state") {
		t.Error("a .txt pair that agrees still carries a word")
	}
	if row := rowOfName(t, a, filesTreeServed, "", "log.txt"); row.State != "" {
		t.Errorf("a .txt in step says %q", row.State)
	}
	if !strings.Contains(body, "edited outside") {
		t.Error("a copy newer than its file is not reported")
	}
	if !strings.Contains(body, "save it once in the editor") {
		t.Error("the one case that no start repairs does not say what to do")
	}
	if !strings.Contains(body, "waits for restart") {
		t.Error("a copy older than its file is not reported")
	}

	// The Source tree sees the same pair from the other side.
	source := getFilesPage(t, a, "tree=source").Body.String()
	if !strings.Contains(source, "edited outside") {
		t.Error("the Source tree does not report a copy that was edited outside")
	}
	if row := rowOfName(t, a, filesTreeSource, "", "log.txt"); row.State != "" {
		t.Errorf("the Source tree says %q for a pair in step", row.State)
	}
}

func touch(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

// rowOfName renders one tree and returns the row of one name, so a test can
// assert on the fields rather than on a substring of HTML.
func rowOfName(t *testing.T, a *App, tree, dir, name string) filesFileRow {
	t.Helper()
	_, here, _, _ := foldToDir(a.treeEntries(tree), dir)
	for _, e := range here {
		if filepath.Base(e.path) == name {
			return a.filesRowFor(tree, e)
		}
	}
	t.Fatalf("no row for %q in %s/%s", name, tree, dir)
	return filesFileRow{}
}

// A same-size edit is the case that sizes alone cannot answer, and a note is
// exactly where it happens: one word swapped for another of equal length.
func TestFilesPage_SameSizeEditIsFound(t *testing.T) {
	a := newTestApp(t)
	shipped, err := staticFS.ReadFile("frontend/md/Editor.md")
	if err != nil {
		t.Fatal(err)
	}
	edited := []byte(string(shipped))
	edited[len(edited)/2] = '#' // same length, different bytes
	writeNoteFile(t, a, "Editor.md", string(edited))

	row := rowOfName(t, a, filesTreeSource, "", "Editor.md")
	if row.State != "changed here" {
		t.Errorf("a same-size edit reads %q; sizes alone cannot see it, so the "+
			"bytes have to be compared", row.State)
	}
	if !row.AppOwned {
		t.Error("Editor.md is in versionDependentAssets, thus the change is the one that is lost")
	}
	if row.StateColor != filesColorAlert {
		t.Errorf("an app-owned file that was changed reads %q, want the alert colour", row.StateColor)
	}
}

// ----------------------------------------------------------------------
// What is listed
// ----------------------------------------------------------------------

// Templates are excluded by the embeds being separate, not by a filter here.
// That is a guarantee about a structure someone could merge away in a minute,
// so it is asserted rather than trusted.
func TestFilesPage_NeverListsTemplates(t *testing.T) {
	for _, f := range embeddedFiles() {
		if strings.Contains(f.path, "templates") {
			t.Errorf("the embedded walk yielded %q", f.path)
		}
	}
	for _, name := range []string{"index.html", "config_page.html", "search_page.html", "files_page.html"} {
		for _, f := range embeddedFiles() {
			if f.path == name {
				t.Errorf("%q reached the listing; templatesFS must stay a separate embed", name)
			}
		}
	}
	a := newTestApp(t)
	body := getFilesPage(t, a, "tree=bundled").Body.String()
	if strings.Contains(body, "files_page.html") {
		t.Error("a template reached the Bundled tree")
	}
}

func TestFilesPage_ExcludesDatabaseBackups(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "db_backup/omn-go-host-2026-08-01.jsonl", "{}")
	writeDiskFile(t, a, "js/mine.js", "// mine")

	for _, f := range a.walkStorage("html") {
		if strings.HasPrefix(f.path, "db_backup") {
			t.Fatalf("the walk yielded %q", f.path)
		}
	}
	root := served(t, a, "")
	if strings.Contains(root, "db_backup") {
		t.Error("db_backup/ appears as a directory row")
	}
	// ... and it cannot be reached by asking for it directly either.
	if strings.Contains(served(t, a, "db_backup%2F"), "omn-go-host") {
		t.Error("?dir=db_backup/ listed a backup")
	}
}

// The Bundled tree carries the starter notes as md/. The Source tree carries
// the notes on the device. The Served tree carries neither: a note is not
// served from html/, its compiled page is.
func TestFilesPage_TreesHoldTheRightThings(t *testing.T) {
	a := newTestApp(t)
	writeNoteFile(t, a, "Log.md", "Title: Log\n\nbody\n")

	bundled := getFilesPage(t, a, "tree=bundled").Body.String()
	if !strings.Contains(bundled, ">md/<") {
		t.Error("md/ is not offered as a directory of the Bundled tree")
	}
	if strings.Contains(bundled, "Log.md") {
		t.Error("a note of the device reached the Bundled tree")
	}
	if strings.Contains(bundled, `class="files-edit"`) {
		t.Error("the Bundled tree offers an edit link; an edit operates on the copy " +
			"on the device, and that copy has a row in another tree")
	}

	source := getFilesPage(t, a, "tree=source").Body.String()
	if !strings.Contains(source, "Log.md") {
		t.Error("the Source tree does not list a note of the device")
	}

	servedRoot := served(t, a, "")
	if strings.Contains(servedRoot, ">md/<") {
		t.Error("md/ still appears inside the Served tree, which is html/ only")
	}
}

// The Bundled tree says app-owned as a word on the second line, like the other
// two trees. Colour alone would leave the fact unreadable for a reader who does
// not see it.
func TestFilesPage_BundledSpellsOutAppOwned(t *testing.T) {
	a := newTestApp(t)
	body := getFilesPage(t, a, "tree=bundled&dir=js%2F").Body.String()
	if !strings.Contains(body, `<span class="files-facts">`) {
		t.Fatal("no facts line in the Bundled tree")
	}
	if !strings.Contains(body, ">app-owned<") {
		t.Error("the Bundled tree never spells out app-owned")
	}
	if !strings.Contains(body, filesColorApp) {
		t.Error("the Bundled tree carries no app colour")
	}
	// A shipped file that the application does not own carries no such word,
	// and there is at least one in js/.
	row := rowOfName(t, a, filesTreeBundled, "js/", "local_counter.js")
	if row.AppOwned {
		t.Error("local_counter.js is not in versionDependentAssets")
	}
}

// A path under md/local/, or any path with a local- segment, stays on this
// device. That is the one fact the Source tree adds beyond the states.
func TestFilesPage_SourceMarksLocalOnly(t *testing.T) {
	a := newTestApp(t)
	writeNoteFile(t, a, "local/Draft.md", "Title: Draft\n\nx\n")
	writeNoteFile(t, a, "local-notes/Second.md", "Title: Second\n\nx\n")

	first := rowOfName(t, a, filesTreeSource, "local/", "Draft.md")
	if len(first.Extra) == 0 || first.Extra[0] != "local only" {
		t.Errorf("md/local/Draft.md is not marked local only: %+v", first.Extra)
	}
	second := rowOfName(t, a, filesTreeSource, "local-notes/", "Second.md")
	if len(second.Extra) == 0 {
		t.Error("a local- directory is not marked local only")
	}
}

// A directory speaks only when the application delivered files into it. Until
// 26.08.55 the rule was inverted and nearly every directory of a real
// installation carried a word that said "ordinary".
func TestFilesDirNote_MarksOnlyAppFiles(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "Journal/2026-08-01.html", "<html></html>")
	writeDiskFile(t, a, "images/photo.png", "x")
	writeDiskFile(t, a, "js/omn-go-core.js", "// extracted")

	dirs, _, _, _ := foldToDir(a.treeEntries(filesTreeServed), "")
	for _, d := range dirs {
		word, color := filesDirNote(filesTreeServed, d)
		switch d.Name {
		case "Journal", "images":
			if word != "" {
				t.Errorf("%s/ says %q; the application delivered nothing into it", d.Name, word)
			}
		case "js":
			if !strings.Contains(word, "from the app") || color != filesColorApp {
				t.Errorf("js/ says %q in %q", word, color)
			}
		case "css", "json":
			// Shipped, and nothing extracted yet.
			if !strings.Contains(word, "none extracted") {
				t.Errorf("%s/ says %q, want the none-extracted form", d.Name, word)
			}
		}
	}
	if word, _ := filesDirNote(filesTreeBundled, filesDirRow{anyShips: true, shipCount: 3}); word != "" {
		t.Errorf("the Bundled tree marks a directory: %q", word)
	}
}

// ----------------------------------------------------------------------
// Edit links
// ----------------------------------------------------------------------

func TestFilesEditable(t *testing.T) {
	a := newTestApp(t)

	yes := []string{"js/mine.js", "css/mine.css", "json/x.json", "user_json/x.json", "notes.txt"}
	for _, p := range yes {
		if !a.filesEditable(p) {
			t.Errorf("%s should offer an edit link", p)
		}
	}

	// The two that were asked for by name get their own reason.
	if a.filesEditable("Welcome.html") {
		t.Error("a compiled page must not offer an edit link: the rendered page " +
			"already carries an Edit button that opens the same editor on the same source")
	}
	if a.filesEditable("images/photo.png") {
		t.Error("an image must not offer an edit link: there is nothing to type into it")
	}

	for _, p := range []string{
		"css/fonts/material-icons.woff2",
		"favicon.ico",
		"images/drawing.svg", // text, but an image - the rule is about images
		"weird.qqq",          // unknown extension: not assumed to be text
	} {
		if a.filesEditable(p) {
			t.Errorf("%s should not offer an edit link", p)
		}
	}
}

func TestFilesPage_EditLinksAppearOnlyWhereTheyShould(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/mine.js", "// mine")
	writeDiskFile(t, a, "Welcome.html", "<html></html>")
	writeDiskFile(t, a, "images/photo.png", "\x89PNG")

	body := served(t, a, "js%2F")
	if !strings.Contains(body, `href="/js/mine.js?edit=true"`) {
		t.Error("no edit link on a .js file")
	}
	// A file that ships and is not extracted yet keeps its edit link: pressing
	// it goes through handleGetNote, which extracts the file and then opens the
	// editor on it.
	if !strings.Contains(body, `href="/js/katex.min.js?edit=true"`) {
		t.Error("a file that is not extracted yet lost its edit link")
	}

	root := served(t, a, "")
	if strings.Contains(root, `Welcome.html?edit=true`) {
		t.Error("edit link on a compiled page")
	}
	imgs := served(t, a, "images%2F")
	if !strings.Contains(imgs, "photo.png") {
		t.Error("images/ content must still be LISTED")
	}
	// Scoped to the row's own href: the page SHELL carries an Edit button of
	// its own (index.html), so a bare search for "?edit=true" would always hit.
	if strings.Contains(imgs, "/images/photo.png?edit=true") {
		t.Error("edit link on an image")
	}
}

// A note of the Source tree opens the editor through its PAGE address, which is
// what resolvePageName expects and what the Edit button of the page does.
func TestFilesPage_SourceNoteEditsThroughItsPage(t *testing.T) {
	a := newTestApp(t)
	writeNoteFile(t, a, "Test/Deep.md", "Title: Deep\n\nx\n")

	row := rowOfName(t, a, filesTreeSource, "Test/", "Deep.md")
	if row.URL != "/Test/Deep.html" {
		t.Errorf("the row links to %q, want the page", row.URL)
	}
	if row.EditURL != "/Test/Deep.html?edit=true" {
		t.Errorf("the edit link is %q", row.EditURL)
	}
}

// ----------------------------------------------------------------------
// Ownership
// ----------------------------------------------------------------------

// isVersionDependent reads a path as versionDependentAssets writes it: relative
// to the storage directory. The mapping from a logical path of one tree is
// filesStoragePath, and getting it wrong would mark every file safe.
func TestFilesStoragePathAndOwnership(t *testing.T) {
	cases := []struct {
		tree, logical, want string
	}{
		{filesTreeServed, "js/omn-go-core.js", "html/js/omn-go-core.js"},
		{filesTreeSource, "UserManual.md", "md/UserManual.md"},
		{filesTreeBundled, "js/omn-go-core.js", "html/js/omn-go-core.js"},
		{filesTreeBundled, "md/UserManual.md", "md/UserManual.md"},
	}
	for _, c := range cases {
		got := filesStoragePath(c.tree, c.logical)
		if got != c.want {
			t.Errorf("filesStoragePath(%q, %q) = %q, want %q", c.tree, c.logical, got, c.want)
		}
		if !isVersionDependent(got) {
			t.Errorf("%q is not app-owned; the test proves nothing if the list changed", got)
		}
	}
	if isVersionDependent("html/js/mine.js") {
		t.Error("a file that the application does not ship was reported as app-owned")
	}
	if isVersionDependent("js/omn-go-core.js") {
		t.Error("a logical path must not match; the list holds storage paths")
	}
}

func TestFilesEmbeddedPath(t *testing.T) {
	cases := []struct{ tree, logical, want string }{
		{filesTreeServed, "js/x.js", "frontend/html/js/x.js"},
		{filesTreeSource, "Editor.md", "frontend/md/Editor.md"},
		{filesTreeBundled, "md/Editor.md", "frontend/md/Editor.md"},
		{filesTreeBundled, "css/x.css", "frontend/html/css/x.css"},
	}
	for _, c := range cases {
		if got := filesEmbeddedPath(c.tree, c.logical); got != c.want {
			t.Errorf("filesEmbeddedPath(%q, %q) = %q, want %q", c.tree, c.logical, got, c.want)
		}
	}
	// The path has to be readable, or every comparison silently falls back to
	// "same size" and the page stops answering its main question.
	if _, err := staticFS.ReadFile(filesEmbeddedPath(filesTreeServed, "js/omn-go-core.js")); err != nil {
		t.Errorf("the embedded path does not resolve: %v", err)
	}
}

// ----------------------------------------------------------------------
// The row, and the page around it
// ----------------------------------------------------------------------

// The name owns the first line and shares it with one word. The facts are on
// the second. A one-line row squeezed the name into a column of two characters
// on a phone.
func TestFilesPage_RowIsTwoLines(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/omn-go-core.js", "// extracted")

	body := served(t, a, "js%2F")
	if !strings.Contains(body, `<span class="files-name">`) {
		t.Error("a row has no name element")
	}
	if !strings.Contains(body, `<span class="files-facts">`) {
		t.Error("a row has no facts line")
	}
	if !strings.Contains(body, `class="material-icons files-kind"`) {
		t.Error("a row carries no type mark")
	}
	// The row shows the date and keeps the full time in the title.
	if strings.Count(body, `class="files-meta" title="`) == 0 {
		t.Error("a row on the device does not carry the full time in a title")
	}
}

func TestFilesKindIcon(t *testing.T) {
	cases := map[string]string{
		"x.html": "html", "x.md": "article", "x.js": "javascript",
		"x.css": "css", "x.json": "data_object", "x.txt": "subject",
		"x.png": "image", "x.woff2": "text_fields", "x.qqq": "insert_drive_file",
	}
	for name, want := range cases {
		if got := filesKindIcon(name, false); got != want {
			t.Errorf("filesKindIcon(%q) = %q, want %q", name, got, want)
		}
	}
	if got := filesKindIcon("js", true); got != "folder" {
		t.Errorf("a directory got %q", got)
	}
}

// The key names only the words that this directory uses, it is folded, and a
// directory that uses no word gets no key at all.
func TestFilesLegend_NamesOnlyWhatIsUsed(t *testing.T) {
	rows := []filesFileRow{{State: "changed here", StateColor: filesColorKeep}}
	legend := filesLegend(filesTreeServed, rows, nil)
	if len(legend) != 1 || legend[0].Color != filesColorKeep || legend[0].Word != "changed here" {
		t.Errorf("legend = %+v, want the green changed-here line only", legend)
	}
	// The same word in the other colour is a different line, because the two
	// outcomes differ.
	red := filesLegend(filesTreeServed, []filesFileRow{
		{State: "changed here", StateColor: filesColorAlert, AppOwned: true, OwnerColor: filesColorAlert},
	}, nil)
	if len(red) != 2 {
		t.Errorf("legend = %+v, want the red changed-here line and app-owned", red)
	}
	if len(filesLegend(filesTreeServed, nil, nil)) != 0 {
		t.Error("an empty directory still printed a key")
	}
}

func TestFilesPage_LegendIsFoldedAndScoped(t *testing.T) {
	a := newTestApp(t)
	writeNoteFile(t, a, "Log.md", "Title: Log\n\nx\n")
	writeDiskFile(t, a, "Log.html", "<html></html>")

	// A directory of nothing but the user's own files needs no key.
	quiet := served(t, a, "")
	if strings.Contains(quiet, "What the words mean") {
		t.Error("a directory that uses no word still printed a key")
	}

	// One that does uses a folded <details>, and it must not be open.
	writeDiskFile(t, a, "js/omn-go-core.js", "// edited")
	loud := served(t, a, "js%2F")
	if !strings.Contains(loud, `<details class="files-legend"><summary>What the words mean</summary>`) {
		t.Error("the key is not a folded details element")
	}
	if strings.Contains(loud, "<details open") {
		t.Error("the key is open by default")
	}
}

// The point of the whole page: describing a file must not create it.
// materializeAsset is the obvious function to reach for when resolving an
// embedded path, and calling it here would extract every shipped file on the
// first page view.
func TestFilesPage_WritesNothing(t *testing.T) {
	a := newTestApp(t)
	before := countFiles(t, a.StorageDir)

	getFilesPage(t, a, "")
	getFilesPage(t, a, "tree=bundled")
	getFilesPage(t, a, "tree=bundled&dir=js%2F")
	getFilesPage(t, a, "tree=served&dir=js%2F")
	getFilesPage(t, a, "tree=source")

	if after := countFiles(t, a.StorageDir); after != before {
		t.Errorf("storage went from %d files to %d; listing a file must not create it", before, after)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "OMNGoFiles.md")); err == nil {
		t.Error("the page synthesized an md/ source for itself")
	}
	if _, err := os.Stat(a.pageHTMLPath("OMNGoFiles")); err == nil {
		t.Error("the page wrote an html/ cache; it must stay dynamic")
	}
}

func countFiles(t *testing.T, root string) int {
	t.Helper()
	n := 0
	filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			n++
		}
		return nil
	})
	return n
}

// ----------------------------------------------------------------------
// Authorization
// ----------------------------------------------------------------------

func remoteFilesRequest(t *testing.T, a *App, cookie string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/OMNGoFiles.html?tree=served&dir=js%2F", nil)
	req.RemoteAddr = "192.168.1.50:41234"
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: "session_role", Value: cookie})
	}
	rec := httptest.NewRecorder()
	a.serveFilesPage(rec, req)
	return rec
}

func TestFilesPage_Authorization(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/secret-name.js", "// x")

	// A connection from the device itself is always the owner - that is how the
	// Android WebView and the desktop browser both arrive.
	if body := served(t, a, "js%2F"); !strings.Contains(body, "secret-name.js") {
		t.Error("a local connection was refused")
	}

	if body := remoteFilesRequest(t, a, "admin").Body.String(); strings.Contains(body, "Administrator only") {
		t.Error("an admin cookie was refused")
	}

	// A guest is not an admin here, and neither is an anonymous request.
	for _, role := range []string{"", "guest"} {
		rec := remoteFilesRequest(t, a, role)
		body := rec.Body.String()
		if !strings.Contains(body, "Administrator only") {
			t.Errorf("role %q was not refused", role)
		}
		if !strings.Contains(body, "Log in") {
			t.Errorf("role %q got a refusal with no way to act on it", role)
		}
		// A refusal must not be a listing with the rows removed.
		if strings.Contains(body, "secret-name.js") {
			t.Errorf("role %q saw a filename in the refusal", role)
		}
		if strings.Contains(body, "%%") {
			t.Errorf("role %q: unfilled template placeholder", role)
		}
	}
}

// hasRole is the single definition of the rule, so authMiddleware must still
// behave exactly as it did when it carried the condition inline.
func TestAuthMiddlewareStillRefusesAfterExtraction(t *testing.T) {
	a := newTestApp(t)
	h := a.authMiddleware(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("through"))
	}, true)

	cases := []struct {
		remote, cookie string
		want           int
	}{
		{"127.0.0.1:1", "", http.StatusOK},        // local bypass
		{"192.168.1.9:1", "admin", http.StatusOK}, // admin cookie
		{"192.168.1.9:1", "guest", http.StatusUnauthorized},
		{"192.168.1.9:1", "", http.StatusUnauthorized},
	}
	for _, c := range cases {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = c.remote
		if c.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "session_role", Value: c.cookie})
		}
		rec := httptest.NewRecorder()
		h(rec, req)
		if rec.Code != c.want {
			t.Errorf("%s/%q: status %d, want %d", c.remote, c.cookie, rec.Code, c.want)
		}
	}
}

// ----------------------------------------------------------------------
// dir, all, escaping
// ----------------------------------------------------------------------

func TestNormalizeFilesDir(t *testing.T) {
	cases := map[string]string{
		"":                 "",
		"/":                "",
		"js":               "js/",
		"js/":              "js/",
		"/js/":             "js/",
		"Test/OMN-Go":      "Test/OMN-Go/",
		"js/../css":        "css/",
		"../../etc":        "",
		"/etc/passwd":      "etc/passwd/", // no escape: still a relative prefix
		"..":               "",
		"db_backup":        "",
		"db_backup/nested": "",
		"./js":             "js/",
	}
	for in, want := range cases {
		if got := normalizeFilesDir(in); got != want {
			t.Errorf("normalizeFilesDir(%q) = %q, want %q", in, got, want)
		}
	}
}

// A hostile or nonsense dir must never reach outside the tree. The two cases
// end up in different places, and both are correct:
//
//   - one that cannot be a relative path at all ("../../etc") collapses to the
//     root, so you land on the top of the listing;
//   - one that is a legal path naming nothing ("/etc/passwd" -> "etc/passwd/",
//     "nope/") renders an empty directory with a breadcrumb back.
//
// Neither ever lists a file, and neither is ever joined onto a filesystem path.
//
// What "never escapes" means here is worth stating, because the obvious
// assertion is the wrong one. It is NOT that the requested string is absent
// from the page: the breadcrumb echoes the directory you asked for, escaped,
// exactly as a browser shows the address you typed. The listing is built by
// filtering paths already collected from the two roots, so a dir that names
// nothing simply matches nothing - there is no filesystem walk to escape from.
// So what is asserted is that nothing was listed and no link leads out.
func TestFilesPage_HostileDirNeverEscapes(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "js/mine.js", "// mine")

	for _, q := range []string{"..%2F..%2Fetc", "%2Fetc%2Fpasswd", "nope%2F", "..%2F.."} {
		body := served(t, a, q)
		if strings.Contains(body, "mine.js") {
			t.Errorf("%s leaked a file from a real directory", q)
		}
		if !strings.Contains(body, "html/") {
			t.Errorf("%s produced no breadcrumb at all", q)
		}
		// Every link on the page is an in-app one: a crumb, or the app's own
		// chrome. Nothing addresses the host filesystem.
		for _, out := range []string{`href="file:`, `href="/etc`, `href="../`, `="/root/`} {
			if strings.Contains(body, out) {
				t.Errorf("%s produced a link out of the app: %s", q, out)
			}
		}
	}

	// The two that name nothing list nothing at all - not a file row, not a
	// directory row. "files-row" is the class every row of either kind carries.
	for _, q := range []string{"%2Fetc%2Fpasswd", "nope%2F"} {
		if body := served(t, a, q); strings.Contains(body, "files-row") {
			t.Errorf("%s listed something; it names no directory in the tree", q)
		}
	}

	// The ones that name nothing say so, rather than rendering a bare heading.
	empty := served(t, a, "nope%2F")
	if !strings.Contains(empty, "holds nothing") {
		t.Error("an empty directory does not say it is empty")
	}
	if !strings.Contains(empty, `href="/OMNGoFiles.html?tree=served"`) {
		t.Error("an empty directory offers no way back to the root of its tree")
	}
}

func TestFilesPage_CapIsHonest(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < 250; i++ {
		writeDiskFile(t, a, filepath.ToSlash(filepath.Join("many", itoa(i)+".txt")), "x")
	}

	body := served(t, a, "many%2F")
	if !strings.Contains(body, "50 not shown") {
		t.Error("the cap did not say how many files it withheld; a listing that " +
			"quietly stops lies about how many files exist")
	}
	if !strings.Contains(body, "show all 250") {
		t.Error("the show-all link does not carry the true total")
	}
	if !strings.Contains(body, "all=1") {
		t.Error("no way to expand the directory")
	}

	all := getFilesPage(t, a, "tree=served&dir=many%2F&all=1").Body.String()
	if strings.Contains(all, "not shown") {
		t.Error("all=1 did not expand the directory")
	}
}

// Directory rows are never capped: hiding a subdirectory hides a branch of the
// tree, not some leaves.
func TestFilesPage_DirectoryRowsAreNeverCapped(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < 250; i++ {
		writeDiskFile(t, a, "d"+itoa(i)+"/f.txt", "x")
	}
	if !strings.Contains(served(t, a, ""), "dir=d249%2F") {
		t.Error("the 250th directory is missing; directory rows must not be capped")
	}
}

// File names come from uploads and note titles, and this page is assembled by
// hand like every other in templates.go.
func TestFilesPage_EscapesFileNames(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, `js/<img src=x onerror=alert(1)>.js`, "// x")

	body := served(t, a, "js%2F")
	if strings.Contains(body, "<img src=x onerror=alert(1)>") {
		t.Error("a file name reached the page as markup")
	}
	if !strings.Contains(body, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Error("the file name is missing entirely; it should be listed, escaped")
	}
}

// The cap is what keeps a real storage directory from turning into a
// multi-megabyte page, and the number that matters is the response size, not
// the row count - so it is the response size that is asserted.
//
// 3 000 files in ONE directory is the shape that hurts: the note tree spreads
// pages across subdirectories, but every note at the top level is a file
// directly in html/, so the root page is the one that can grow without bound.
//
// The budget is generous on purpose. It is not a golden size to be nudged
// whenever a column is added; it is the line between "a page" and "a download",
// and only a change that removes the cap can cross it.
func TestFilesPage_ScalesToABigDirectory(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < 3000; i++ {
		writeDiskFile(t, a, "big"+itoa(i)+".txt", "x")
	}

	body := served(t, a, "")
	if !strings.Contains(body, "not shown") {
		t.Errorf("3 000 files did not render capped at %d", filesDirLimit)
	}
	// One list now, not two, so the page carries one capped list of rows plus
	// the directory rows of the tree.
	if n := strings.Count(body, `class="files-row"`); n > filesDirLimit+40 {
		t.Errorf("%d file rows, want at most %d", n, filesDirLimit+40)
	}
	if len(body) > 256*1024 {
		t.Errorf("the page is %d KB; the cap exists so that it cannot be", len(body)/1024)
	}
}
