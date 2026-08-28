package backend

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePageName(t *testing.T) {
	a := &App{StorageDir: "/store"}

	md := func(rel string) string { return filepath.Join("/store", "md", rel) }
	html := func(rel string) string { return filepath.Join("/store", "html", rel) }

	tests := []struct {
		name         string
		in           string
		wantMD       string
		wantHTML     string
		wantBaseName string
		wantIsPage   bool
	}{
		{"bare page name", "Welcome", md("Welcome.md"), html("Welcome.html"), "Welcome", true},
		{"markdown filename", "Welcome.md", md("Welcome.md"), html("Welcome.html"), "Welcome", true},
		{"compiled html filename", "Welcome.html", md("Welcome.md"), html("Welcome.html"), "Welcome", true},
		{"nested page", "dir/Note.md", md(filepath.Join("dir", "Note.md")), html(filepath.Join("dir", "Note.html")), "dir/Note", true},
		{"static js asset", "app.js", "", html("app.js"), "app.js", false},
		{"static css asset", "css/omn-go-core.css", "", html(filepath.Join("css", "omn-go-core.css")), "css/omn-go-core.css", false},
		{"static image", "images/pic.png", "", html(filepath.Join("images", "pic.png")), "images/pic.png", false},
		{"asset with extra dots", "js/app.min.js", "", html(filepath.Join("js", "app.min.js")), "js/app.min.js", false},

		// 26.08.76: a note name may hold a dot. The LAST extension
		// decides, and ".2026" is not an extension this install serves,
		// thus each of the three spellings is the same note.
		{"dotted bare name", "Report.2026", md("Report.2026.md"), html("Report.2026.html"), "Report.2026", true},
		{"dotted markdown filename", "Report.2026.md", md("Report.2026.md"), html("Report.2026.html"), "Report.2026", true},
		{"dotted compiled filename", "Report.2026.html", md("Report.2026.md"), html("Report.2026.html"), "Report.2026", true},
		{"two dots", "a.b.c.html", md("a.b.c.md"), html("a.b.c.html"), "a.b.c", true},

		// A name that ends in an extension this install serves is a file,
		// whatever comes before it. The note of that name carries its own
		// ".md" or ".html", thus the two never collide.
		{"file name that looks like a note", "Draft.txt", "", html("Draft.txt"), "Draft.txt", false},
		{"note behind that file name", "Draft.txt.md", md("Draft.txt.md"), html("Draft.txt.html"), "Draft.txt", true},
		{"compiled note behind it", "Draft.txt.html", md("Draft.txt.md"), html("Draft.txt.html"), "Draft.txt", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotMD, gotHTML, gotBase, gotIsPage := a.resolvePageName(tt.in)
			if gotIsPage != tt.wantIsPage {
				t.Fatalf("isPage = %v, want %v", gotIsPage, tt.wantIsPage)
			}
			if gotBase != tt.wantBaseName {
				t.Errorf("baseName = %q, want %q", gotBase, tt.wantBaseName)
			}
			if gotMD != tt.wantMD {
				t.Errorf("mdPath = %q, want %q", gotMD, tt.wantMD)
			}
			if gotHTML != tt.wantHTML {
				t.Errorf("htmlPath = %q, want %q", gotHTML, tt.wantHTML)
			}
		})
	}
}

// The three spellings of one page must resolve to one answer. Four diverged
// inline implementations broke this invariant before resolvePageName existed.
//
// A page whose name holds a dot gets the same treatment since 26.08.76. The
// name "Welcome.md" no longer reads as a mistake: a person may name a note
// that way, and its source is then md/Welcome.md.md.
func TestResolvePageNameEquivalence(t *testing.T) {
	a := &App{StorageDir: "/store"}
	for _, spellings := range [][]string{
		{"Welcome", "Welcome.md", "Welcome.html"},
		{"Report.2026", "Report.2026.md", "Report.2026.html"},
	} {
		firstMD, firstHTML, firstBase, _ := a.resolvePageName(spellings[0])
		for _, s := range spellings[1:] {
			gotMD, gotHTML, gotBase, isPage := a.resolvePageName(s)
			if !isPage {
				t.Fatalf("%q not detected as page", s)
			}
			if gotMD != firstMD || gotHTML != firstHTML || gotBase != firstBase {
				t.Errorf("%q resolves differently: (%q, %q, %q) vs (%q, %q, %q)",
					s, gotMD, gotHTML, gotBase, firstMD, firstHTML, firstBase)
			}
		}
	}
}

// TestHasKnownAssetExtension pins the rule that decides a note from a file.
// The LAST extension decides, and only the two tables of this install answer.
//
// The stdlib table takes no part on purpose. mime.TypeByExtension reads
// /etc/mime.types, thus a desktop with mime-support knows ".doc" and a phone
// does not. A note named "Plan.doc" would then be a file on one device and a
// note on the other. Git sync carries that name to both.
func TestHasKnownAssetExtension(t *testing.T) {
	a := &App{StorageDir: "/store"}
	cases := map[string]bool{
		"Welcome":       false,
		"Report.2026":   false,
		"a.b.c":         false,
		"Plan.doc":      false, // the stdlib knows this one. This function must not.
		"Draft.txt":     true,
		"app.js":        true,
		"js/app.min.js": true,
		"images/x.png":  true,
		"page.html":     true,
		"note.md":       true,
	}
	for name, want := range cases {
		if got := a.hasKnownAssetExtension(name); got != want {
			t.Errorf("hasKnownAssetExtension(%q) = %v, want %v", name, got, want)
		}
	}

	// A per-install override in config.json counts, because that install
	// really does serve the extension.
	b := &App{StorageDir: "/store"}
	b.Config.MimeTypes = map[string]string{".2026": "text/plain"}
	if !b.hasKnownAssetExtension("Report.2026") {
		t.Error("a mime_types override in config.json did not count")
	}
}

// ---------------------------------------------------------------------
// A note name that holds a dot, end to end
//
// The unit tests above pin the classifier. These two pin what a person
// meets: the page comes back, and a save reaches the markdown source.
// ---------------------------------------------------------------------

// TestDottedNoteNameServesAndSaves exists because /Report.2026.html gave
// 404 until 26.08.76, and a save for a note named "Draft.txt" wrote
// html/Draft.txt while it reported success. The note kept the old text, and
// nothing said so.
func TestDottedNoteNameServesAndSaves(t *testing.T) {
	a := newTestApp(t)

	notes := map[string]string{
		"Report.2026.md": "Title: Report\n\nthe body of the report",
		"a.b.c.md":       "Title: Deep\n\nthe deep body",
		"Draft.txt.md":   "Title: Draft\n\nthe draft body",
	}
	for rel, body := range notes {
		baseWriteMD(t, a, rel, body)
	}

	// 1. Each one comes back as a page, with its body in it.
	for _, tc := range []struct{ url, want string }{
		{"/Report.2026.html", "the body of the report"},
		{"/a.b.c.html", "the deep body"},
		{"/Draft.txt.html", "the draft body"},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.url, nil)
		rec := httptest.NewRecorder()
		a.serveFrontend(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200. A note name may hold a dot.", tc.url, rec.Code)
			continue
		}
		if !strings.Contains(rec.Body.String(), tc.want) {
			t.Errorf("GET %s does not carry %q", tc.url, tc.want)
		}
	}

	// 2. A save reaches the markdown source, and writes nothing under html/
	//    beside the compiled page. "Draft.txt" is the dangerous name: ".txt"
	//    is an extension this install serves, thus the bare form resolves to
	//    a file. The editor sends "Draft.txt.md", which cannot.
	postForm(t, a.handleSaveNote, "/api/save", url.Values{
		"name":    {"Draft.txt.md"},
		"content": {"Title: Draft\n\nthe new draft body"},
	})
	src, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "Draft.txt.md"))
	if err != nil {
		t.Fatalf("reading the note source: %v", err)
	}
	if !strings.Contains(string(src), "the new draft body") {
		t.Error("the save did not reach md/Draft.txt.md")
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "html", "Draft.txt")); err == nil {
		t.Error("the save wrote html/Draft.txt. A save for a note must never " +
			"land beside the files, where nothing reads it and the note keeps " +
			"the old text.")
	}
}

// TestDottedNoteNameReachesTheEditor exists because the editor gets its
// text from /api/note. The editor sent the bare base name until 26.08.76.
// The editor for a note named "Draft.txt" thus asked for the file
// html/Draft.txt, and it opened on a refusal.
func TestDottedNoteNameReachesTheEditor(t *testing.T) {
	a := newTestApp(t)
	baseWriteMD(t, a, "Draft.txt.md", "Title: Draft\n\nthe draft body")

	// The editor page names what it will ask for.
	rec := httptest.NewRecorder()
	a.renderInternalEditor(rec, "Draft.txt.html")
	page := rec.Body.String()
	if !strings.Contains(page, `OMN_EDIT_NAME = 'Draft.txt.md'`) {
		t.Errorf("the editor page does not send the unambiguous name. It must "+
			"ask for %q, or /api/note answers with the file of that name.",
			"Draft.txt.md")
	}

	// And that name really does bring the note back.
	req := httptest.NewRequest(http.MethodGet, "/api/note?name=Draft.txt.md", nil)
	got := httptest.NewRecorder()
	a.handleGetNote(got, req)
	if got.Code != http.StatusOK {
		t.Fatalf("/api/note?name=Draft.txt.md = %d, want 200", got.Code)
	}
	if !strings.Contains(got.Body.String(), "the draft body") {
		t.Error("/api/note did not answer with the note text")
	}
}

// TestDottedNoteCompilesAsMarkdown exists because the compiled page carries
// IS_MARKDOWN, and the frontend reads it to decide which controls a page
// gets. compilePageWithBody cannot answer from the name alone: a note named
// "Draft.txt" and the file html/Draft.txt look the same there. It reads the
// caller instead. An empty customBody means a note.
func TestDottedNoteCompilesAsMarkdown(t *testing.T) {
	a := newTestApp(t)
	for _, name := range []string{"Welcome", "Report.2026", "Draft.txt", "a.b.c"} {
		page := string(a.compilePage(name, []byte("Title: X\n\nbody")))
		if !strings.Contains(page, "var IS_MARKDOWN = true;") {
			t.Errorf("the compiled page of note %q is not markdown. The page "+
				"then loses each control that belongs to a note.", name)
		}
		if !strings.Contains(page, "var PAGE_EXT = '';") {
			t.Errorf("the compiled page of note %q carries a file extension "+
				"in PAGE_EXT", name)
		}
	}

	// A server-built view of a file keeps the extension of that file.
	wait := string(a.compilePageWithBody("js/app.min.js",
		[]byte("Title: Wait\n\n"), "<p>waiting</p>"))
	if !strings.Contains(wait, "var PAGE_EXT = '.js';") {
		t.Error("a server-built view of a .js file lost its extension")
	}
}
