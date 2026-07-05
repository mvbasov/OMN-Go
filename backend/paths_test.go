package backend

import (
	"path/filepath"
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

// The three spellings of the same page must resolve identically - this is
// the invariant that used to be violated by four diverged inline
// implementations (the "Welcome.md.html" bug family).
func TestResolvePageNameEquivalence(t *testing.T) {
	a := &App{StorageDir: "/store"}
	spellings := []string{"Welcome", "Welcome.md", "Welcome.html"}

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
