package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePageName(t *testing.T) {
	l := Layout("/store")

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
		{"static css asset", "css/OMN-Go/omn-go-core.css", "", html(filepath.Join("css", "OMN-Go", "omn-go-core.css")), "css/OMN-Go/omn-go-core.css", false},
		{"static image", "images/pic.png", "", html(filepath.Join("images", "pic.png")), "images/pic.png", false},
		{"asset with extra dots", "js/app.min.js", "", html(filepath.Join("js", "app.min.js")), "js/app.min.js", false},

		// A note name may hold a dot. The LAST extension decides, and ".2026" is not
		// an extension this install serves, thus each of the three spellings is the
		// same note.
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
			gotMD, gotHTML, gotBase, gotIsPage := ResolvePageName(l, nil, tt.in)
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

// The three spellings of one page must resolve to one answer.
//
// A page whose name holds a dot gets the same treatment. The name
// "Welcome.md" is not a mistake: a person may name a note that way, and
// its source is then md/Welcome.md.md.
func TestResolvePageNameEquivalence(t *testing.T) {
	l := Layout("/store")
	for _, spellings := range [][]string{
		{"Welcome", "Welcome.md", "Welcome.html"},
		{"Report.2026", "Report.2026.md", "Report.2026.html"},
	} {
		firstMD, firstHTML, firstBase, _ := ResolvePageName(l, nil, spellings[0])
		for _, s := range spellings[1:] {
			gotMD, gotHTML, gotBase, isPage := ResolvePageName(l, nil, s)
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

// ContainedName is the guard of each path that a request names. The
// table holds the shapes that climb out, and the normal names that must
// not change.
func TestContainedName(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"Welcome", "Welcome"},
		{"Test/OMN-Go/DBTest", "Test/OMN-Go/DBTest"},
		{"Report.2026", "Report.2026"},
		{"/Welcome", "Welcome"},
		{"./a/./b", "a/b"},
		{"a/b/../c", "a/c"},
		{"../../x", "x"},
		{"a/../../b", "b"},
		{"/../../etc/x", "etc/x"},
		{"..", ""},
		{"", ""},
	} {
		if got := ContainedName(tc.in); got != tc.want {
			t.Errorf("ContainedName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// A new version drops each compiled page, and it keeps each other .html
// file. A compiled page holds the page shell of the old templates. An .html
// file with no .md source is a file of the user, and nothing compiles it
// again.
func TestDropCompiledPagesRemovesOnlyCompiledPages(t *testing.T) {
	l := Layout(t.TempDir())
	write := func(p, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"Doc", "sub/Deep"} {
		write(l.MD(filepath.FromSlash(name+".md")), "Title: X\n\nbody")
		write(l.PageHTML(name), "<html>old shell</html>")
	}
	own := l.HTML("js", "demo.html")
	write(own, "<p>mine</p>")

	n, err := DropCompiledPages(l)
	if err != nil || n != 2 {
		t.Fatalf("DropCompiledPages = %d, %v, want 2, nil", n, err)
	}
	for _, name := range []string{"Doc", "sub/Deep"} {
		if FileExists(l.PageHTML(name)) {
			t.Errorf("the compiled page of %s is still there", name)
		}
		if !FileExists(l.MD(filepath.FromSlash(name + ".md"))) {
			t.Errorf("md/%s.md went away", name)
		}
	}
	if !FileExists(own) {
		t.Error("the .html file of the user went away")
	}

	// A storage directory with no html/ is not a fault.
	if n, err := DropCompiledPages(Layout(t.TempDir())); n != 0 || err != nil {
		t.Errorf("an empty storage directory: %d, %v", n, err)
	}
}
