package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTagSlug(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Hydroponics", "Hydroponics"},
		{"3D Print", "3D-Print"},
		{"QR code", "QR-code"},
		{"OMN documentation", "OMN-documentation"},
		{"R&D", "R-D"},
		{"  spaced  ", "spaced"},
		{"a--b__c", "a-b-c"},
		{"Гидропоника", "Гидропоника"}, // unicode letters kept
		{"", ""},
		{"!!!", ""},
	}
	for _, c := range cases {
		if got := TagSlug(c.in); got != c.want {
			t.Errorf("TagSlug(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractTitleTags(t *testing.T) {
	title, tags := ExtractTitleTags("Title: My Page\nTags: Red, Blue ,, Green\n\nbody")
	if title != "My Page" {
		t.Errorf("title = %q, want %q", title, "My Page")
	}
	if strings.Join(tags, "|") != "Red|Blue|Green" {
		t.Errorf("tags = %v, want [Red Blue Green] (empties dropped, trimmed)", tags)
	}

	// No header -> empty title, nil tags.
	if ti, tg := ExtractTitleTags("no header here"); ti != "" || tg != nil {
		t.Errorf("no-header = (%q,%v), want (\"\",nil)", ti, tg)
	}

	// Header without Title/Tags.
	if ti, tg := ExtractTitleTags("Date: 2026-01-01\n\nbody"); ti != "" || len(tg) != 0 {
		t.Errorf("no title/tags = (%q,%v), want empty", ti, tg)
	}
}

// writeNote is a tiny helper for the index/generate tests.
func writeNote(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, "md", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestBuildTagIndex(t *testing.T) {
	dir := t.TempDir()
	a := &testApp{StorageDir: dir}

	writeNote(t, dir, "Alpha.md", "Title: Alpha Note\nTags: Red, Blue\n\nx")
	writeNote(t, dir, "sub/Beta.md", "Title: Beta\nTags: Blue\n\nx")
	writeNote(t, dir, "Dup.md", "Title: Dup\nTags: Red, Red\n\nx")       // per-page de-dupe
	writeNote(t, dir, "Untagged.md", "Title: Nothing\n\nx")              // omitted
	writeNote(t, dir, "local/Scratch.md", "Title: S\nTags: Red\n\nx")    // md/local excluded
	writeNote(t, dir, "OMNGoTags.md", "Title: Tags\nTags: Ignored\n\nx") // self excluded

	idx := a.testRenderer().BuildTagIndex()

	if len(idx["Blue"]) != 2 {
		t.Errorf("Blue has %d pages, want 2 (Alpha, Beta)", len(idx["Blue"]))
	}
	if len(idx["Red"]) != 2 {
		t.Errorf("Red has %d pages, want 2 (Alpha, Dup) - local excluded, per-page de-duped", len(idx["Red"]))
	}
	if _, ok := idx["Ignored"]; ok {
		t.Error("the index holds OMNGoTags.md. The index must not hold it.")
	}
	// Untagged note contributes no tag.
	for tag, refs := range idx {
		for _, r := range refs {
			if r.Path == "Untagged" {
				t.Errorf("untagged note appeared under tag %q", tag)
			}
			if r.Path == "local/Scratch" {
				t.Errorf("md/local note appeared under tag %q", tag)
			}
		}
	}
	// Subdir path preserved (used as a relative link later).
	foundBeta := false
	for _, r := range idx["Blue"] {
		if r.Path == "sub/Beta" && r.Title == "Beta" {
			foundBeta = true
		}
	}
	if !foundBeta {
		t.Errorf("Blue missing sub/Beta: %+v", idx["Blue"])
	}
}

func TestGenerateTagsPage(t *testing.T) {
	dir := t.TempDir()
	a := &testApp{StorageDir: dir}

	writeNote(t, dir, "Alpha.md", "Title: Alpha Note\nTags: Red, Blue\n\nx")
	writeNote(t, dir, "sub/Beta.md", "Title: Beta\nTags: Blue\n\nx")
	writeNote(t, dir, "Untagged.md", "Title: Nothing\n\nx")

	if err := a.generateTagsPage(); err != nil {
		t.Fatalf("generateTagsPage: %v", err)
	}

	mdBytes, err := os.ReadFile(filepath.Join(dir, "md", "OMNGoTags.md"))
	if err != nil {
		t.Fatalf("md/OMNGoTags.md not written: %v", err)
	}
	md := string(mdBytes)

	for _, want := range []string{
		"Title: Tags",
		`<div class="omn-tags-cloud">`,
		`<a href="#Blue" class="omn-taglink">`,
		`<a href="#Red" class="omn-taglink">`,
		`<h2 id="Blue" class="omn-tags-section">Blue (2)</h2>`,
		`<h2 id="Red" class="omn-tags-section">Red (1)</h2>`,
		`<li><a href="Alpha.html">Alpha Note</a></li>`,
		`<li><a href="sub/Beta.html">Beta</a></li>`,
	} {
		if !strings.Contains(md, want) {
			t.Errorf("generated md missing %q:\n%s", want, md)
		}
	}

	// Untagged note must not be linked anywhere.
	if strings.Contains(md, "Untagged.html") {
		t.Error("untagged note was linked on the Tags page")
	}
	// Tags alphabetical: Blue's section before Red's.
	if strings.Index(md, `id="Blue"`) > strings.Index(md, `id="Red"`) {
		t.Error("tag sections not alphabetical (Blue should precede Red)")
	}
	// Pages within Blue by title: "Alpha Note" before "Beta".
	blueBlock := md[strings.Index(md, `id="Blue"`):strings.Index(md, `id="Red"`)]
	if strings.Index(blueBlock, "Alpha.html") > strings.Index(blueBlock, "sub/Beta.html") {
		t.Error("pages within a tag not sorted by title (Alpha Note should precede Beta)")
	}

	// The compiled HTML cache exists and carries the links.
	htmlBytes, err := os.ReadFile(filepath.Join(dir, "html", "OMNGoTags.html"))
	if err != nil {
		t.Fatalf("html/OMNGoTags.html not written: %v", err)
	}
	html := string(htmlBytes)
	for _, want := range []string{`href="Alpha.html"`, `href="sub/Beta.html"`, `id="Blue"`} {
		if !strings.Contains(html, want) {
			t.Errorf("compiled OMNGoTags.html missing %q", want)
		}
	}
}

func TestTagsPageStaleness(t *testing.T) {
	dir := t.TempDir()
	a := &testApp{StorageDir: dir}
	writeNote(t, dir, "Alpha.md", "Title: A\nTags: X\n\nx")
	writeNote(t, dir, "sub/Beta.md", "Title: B\nTags: X\n\nx")

	if err := a.generateTagsPage(); err != nil {
		t.Fatalf("generateTagsPage: %v", err)
	}
	htmlPath := a.layout().PageHTML("OMNGoTags")

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)   // notes
	mid := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)    // html (newer than notes)
	future := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC) // a later change

	// Explicit mtimes avoid filesystem-granularity flakiness. Set every note
	// source (files AND their directories) to `past`, the html cache to `mid`.
	setSources := func(tm time.Time) {
		for _, rel := range []string{"", "Alpha.md", "sub", "sub/Beta.md", "OMNGoTags.md"} {
			os.Chtimes(filepath.Join(dir, "md", filepath.FromSlash(rel)), tm, tm)
		}
	}
	setSources(past)
	os.Chtimes(htmlPath, mid, mid)

	if a.testRenderer().TagsPageStale(false) {
		t.Error("not stale expected: html is newer than every note source")
	}
	if !a.testRenderer().TagsPageStale(true) {
		t.Error("forceRefresh must report stale")
	}

	// Editing a note makes it newer than the cache.
	os.Chtimes(filepath.Join(dir, "md", "Alpha.md"), future, future)
	if !a.testRenderer().TagsPageStale(false) {
		t.Error("edited note must report stale")
	}

	// A dir mtime bump alone (as an add/delete/rename would cause) is detected.
	setSources(past)
	os.Chtimes(htmlPath, mid, mid)
	os.Chtimes(filepath.Join(dir, "md", "sub"), future, future)
	if !a.testRenderer().TagsPageStale(false) {
		t.Error("directory mtime bump (add/delete/rename) must report stale")
	}

	// The derived OMNGoTags.md itself must never drive staleness.
	setSources(past)
	os.Chtimes(htmlPath, mid, mid)
	os.Chtimes(filepath.Join(dir, "md", "OMNGoTags.md"), future, future)
	if a.testRenderer().TagsPageStale(false) {
		t.Error("derived OMNGoTags.md must be excluded from the staleness scan")
	}

	// Missing cache is stale.
	os.Remove(htmlPath)
	if !a.testRenderer().TagsPageStale(false) {
		t.Error("missing html cache must report stale")
	}
}
