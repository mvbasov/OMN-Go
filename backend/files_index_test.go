package backend

// Tests for the file index page.
//
// Three things here are worth more than the rest, because each is a claim the
// page makes that nothing else in the app would catch if it stopped being
// true:
//
//   - it shows ONE directory. The whole design turns on that, and "flat list
//     of everything" is what it drifted into twice while being planned.
//   - it never WRITES. The obvious way to resolve an embedded path is
//     materializeAsset, which extracts the file as a side effect - a listing
//     built that way would silently defeat lazy extraction for all 42 of them.
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

// writeDiskFile drops a file under StorageDir/html.
func writeDiskFile(t *testing.T, a *App, rel, body string) {
	t.Helper()
	full := filepath.Join(a.StorageDir, "html", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
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

	root := getFilesPage(t, a, "").Body.String()
	if !strings.Contains(root, ">js/<") {
		t.Error("the root does not list js/ as a directory")
	}
	if strings.Contains(root, "mine.js") {
		t.Error("the root listed a file from INSIDE js/; this page shows one level")
	}
	if !strings.Contains(root, "Welcome.html") {
		t.Error("a file at the root level is missing")
	}

	js := getFilesPage(t, a, "dir=js%2F").Body.String()
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

	root := getFilesPage(t, a, "").Body.String()
	if !strings.Contains(root, "dir=Test%2F") {
		t.Fatalf("no link into Test/ from the root")
	}

	mid := getFilesPage(t, a, "dir=Test%2F").Body.String()
	if !strings.Contains(mid, "dir=Test%2FOMN-Go%2F") {
		t.Fatalf("no link into Test/OMN-Go/ from Test/")
	}

	leaf := getFilesPage(t, a, "dir=Test%2FOMN-Go%2F").Body.String()
	if !strings.Contains(leaf, "Fetch.html") {
		t.Error("the leaf directory does not list its file")
	}
	// The breadcrumb must offer BOTH ancestors, not just the immediate one.
	for _, want := range []string{`href="/OMNGoFiles.html"`, "dir=Test%2F"} {
		if !strings.Contains(leaf, want) {
			t.Errorf("breadcrumb is missing %q; a leaf you cannot leave is a dead end", want)
		}
	}
}

// A directory row answers "how big is this subtree", not "how many entries are
// immediately inside" - which is rarely the question a file index is opened to
// answer.
func TestFilesPage_DirectoryTotalsAreRecursive(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "Test/a/one.html", "12345")
	writeDiskFile(t, a, "Test/b/two.html", "12345")

	files, _, _, _ := foldToDir(a.diskFiles(), "")
	var test *filesDirRow
	for i := range files {
		if files[i].Name == "Test" {
			test = &files[i]
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
}

func TestFilesPage_ExcludesDatabaseBackups(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, "db_backup/omn-go-host-2026-08-01.jsonl", "{}")
	writeDiskFile(t, a, "js/mine.js", "// mine")

	for _, f := range a.diskFiles() {
		if strings.HasPrefix(f.path, "db_backup") {
			t.Fatalf("the walk yielded %q", f.path)
		}
	}
	root := getFilesPage(t, a, "").Body.String()
	if strings.Contains(root, "db_backup") {
		t.Error("db_backup/ appears as a directory row")
	}
	// ... and it cannot be reached by asking for it directly either.
	deep := getFilesPage(t, a, "dir=db_backup%2F").Body.String()
	if strings.Contains(deep, "omn-go-host") {
		t.Error("?dir=db_backup/ listed a backup")
	}
}

// The embedded starter notes appear as a directory named md/, which is a
// different question from "what notes do I have" - that is what the Tags page
// and search are for.
func TestFilesPage_EmbeddedNotesAppearAsMdDirectory(t *testing.T) {
	found := false
	for _, f := range embeddedFiles() {
		if f.path == "md/UserManual.md" {
			found = true
		}
	}
	if !found {
		t.Error("the embedded md/ tree is missing from the walk")
	}

	a := newTestApp(t)
	root := getFilesPage(t, a, "").Body.String()
	if !strings.Contains(root, ">md/<") {
		t.Error("md/ is not offered as a directory at the embedded root")
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

	body := getFilesPage(t, a, "dir=js%2F").Body.String()
	if !strings.Contains(body, `href="/js/mine.js?edit=true"`) {
		t.Error("no edit link on a .js file")
	}

	root := getFilesPage(t, a, "").Body.String()
	if strings.Contains(root, `Welcome.html?edit=true`) {
		t.Error("edit link on a compiled page")
	}
	imgs := getFilesPage(t, a, "dir=images%2F").Body.String()
	if !strings.Contains(imgs, "photo.png") {
		t.Error("images/ content must still be LISTED")
	}
	// Scoped to the row's own href: the page SHELL carries an Edit button of
	// its own (index.html), so a bare search for "?edit=true" would always hit.
	if strings.Contains(imgs, "/images/photo.png?edit=true") {
		t.Error("edit link on an image")
	}
}

// ----------------------------------------------------------------------
// Provenance
// ----------------------------------------------------------------------

func TestFilesPage_EmbeddedStateAndOwnership(t *testing.T) {
	a := newTestApp(t)
	// omn-go-core.js ships and is version-dependent; extract one copy so the
	// two states can be told apart in the same listing.
	writeDiskFile(t, a, "js/omn-go-core.js", "// extracted")

	body := getFilesPage(t, a, "dir=js%2F").Body.String()
	if !strings.Contains(body, "on disk") {
		t.Error("an extracted embedded file is not reported as being on disk")
	}
	if !strings.Contains(body, "not yet") {
		t.Error("no embedded file is reported as unextracted; at least one should be")
	}
	if !strings.Contains(body, "app-owned") {
		t.Error("a versionDependentAssets entry is not marked app-owned")
	}

	if !isVersionDependent("js/omn-go-core.js") {
		t.Error("omn-go-core.js should be app-owned")
	}
	if isVersionDependent("json/bookmarker-tags.json") {
		t.Error("a user-owned asset was reported as app-owned")
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
	getFilesPage(t, a, "dir=js%2F")
	getFilesPage(t, a, "dir=md%2F")

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
	req := httptest.NewRequest(http.MethodGet, "/OMNGoFiles.html", nil)
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
	if body := getFilesPage(t, a, "").Body.String(); !strings.Contains(body, "secret-name.js") == false {
		_ = body // listing is at dir=js/, checked below
	}
	if body := getFilesPage(t, a, "dir=js%2F").Body.String(); !strings.Contains(body, "secret-name.js") {
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

// hasRole is now the single definition of the rule, so authMiddleware must
// still behave exactly as it did when it carried the condition inline.
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

	for _, q := range []string{"dir=..%2F..%2Fetc", "dir=%2Fetc%2Fpasswd", "dir=nope%2F", "dir=..%2F.."} {
		body := getFilesPage(t, a, q).Body.String()
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
	for _, q := range []string{"dir=%2Fetc%2Fpasswd", "dir=nope%2F"} {
		if body := getFilesPage(t, a, q).Body.String(); strings.Contains(body, "files-row") {
			t.Errorf("%s listed something; it names no directory in the tree", q)
		}
	}

	// The ones that name nothing say so, rather than rendering a bare heading.
	empty := getFilesPage(t, a, "dir=nope%2F").Body.String()
	if !strings.Contains(empty, "Nothing is") {
		t.Error("an empty directory does not say it is empty")
	}
	if !strings.Contains(empty, `href="/OMNGoFiles.html"`) {
		t.Error("an empty directory offers no way back to the root")
	}
}

func TestFilesPage_CapIsHonest(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < 250; i++ {
		writeDiskFile(t, a, filepath.ToSlash(filepath.Join("many", itoa(i)+".txt")), "x")
	}

	body := getFilesPage(t, a, "dir=many%2F").Body.String()
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

	all := getFilesPage(t, a, "dir=many%2F&all=1").Body.String()
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
	body := getFilesPage(t, a, "").Body.String()
	if !strings.Contains(body, "dir=d249%2F") {
		t.Error("the 250th directory is missing; directory rows must not be capped")
	}
}

// File names come from uploads and note titles, and this page is assembled by
// hand like every other in templates.go.
func TestFilesPage_EscapesFileNames(t *testing.T) {
	a := newTestApp(t)
	writeDiskFile(t, a, `js/<img src=x onerror=alert(1)>.js`, "// x")

	body := getFilesPage(t, a, "dir=js%2F").Body.String()
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

	body := getFilesPage(t, a, "").Body.String()
	if !strings.Contains(body, "2800 not shown") {
		t.Errorf("3 000 files did not render capped at %d", filesDirLimit)
	}
	// Each of the two sections caps independently, so the page as a whole can
	// carry two capped lists and no more.
	if n := strings.Count(body, `class="files-row"`); n > 2*filesDirLimit {
		t.Errorf("%d file rows, want at most %d", n, 2*filesDirLimit)
	}
	if len(body) > 256*1024 {
		t.Errorf("the page is %d KB; the cap exists so that it cannot be", len(body)/1024)
	}
}
