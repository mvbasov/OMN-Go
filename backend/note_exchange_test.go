package backend

// Tests for note exchange (note_exchange.go) and the two header-key helpers
// it needs (frontmatter.go).
//
// Two properties carry the weight here.
//
// A NAME FROM ANOTHER DEVICE IS NOT A PATH. FileName: is a line of text that
// arrived from a stranger's phone through Telegram. The sanitizer table below
// is the list of things it has tried to be, and the containment test asserts
// the resolved path, not the string - because filepath.Join RESOLVES a "..".
//
// A KEY IS SET, NOT APPENDED. A note can hop twice, and a header block that
// carries "Imported:" two times has no defined meaning: splitFrontMatter
// hands the first one to whatever reads it, and which is first is an accident
// of the order the hops ran in.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testNow is fixed so that a test can state exactly what Imported: and the
// index line must say. importNote takes the clock as a parameter for this.
var testNow = time.Date(2026, 8, 9, 12, 34, 56, 0, time.UTC)

func incomingFile(t *testing.T, a *App, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.StorageDir, "md", "incoming", filepath.FromSlash(rel)))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

// ----------------------------------------------------------------------
// Header keys
// ----------------------------------------------------------------------

func TestSetHeaderKey(t *testing.T) {
	for _, c := range []struct{ what, in, want string }{
		{
			"a new key goes last in the block",
			"Title: T\nDate: 2026-01-01\n\nBody line\n",
			"Title: T\nDate: 2026-01-01\nFileName: a/b/C\n\nBody line\n",
		},
		{
			"an existing key is replaced where it stands",
			"Title: T\nFileName: old\nDate: d\n\nB\n",
			"Title: T\nFileName: new\nDate: d\n\nB\n",
		},
		{
			"a note with no header block gets one",
			"Just a body\n",
			"FileName: X\n\nJust a body\n",
		},
		{
			"empty content",
			"",
			"FileName: X\n",
		},
		{
			// The separator is ONE newline when the header ended at a line
			// that is not metadata. Rebuilding with a fixed "\n\n" would
			// insert a blank line into the note.
			"a single-newline separator is kept",
			"Title: T\n<style>x</style>\n",
			"Title: T\nFileName: X\n<style>x</style>\n",
		},
	} {
		value := "X"
		if strings.Contains(c.want, "a/b/C") {
			value = "a/b/C"
		} else if strings.Contains(c.want, "FileName: new") {
			value = "new"
		}
		if got := setHeaderKey(c.in, "FileName", value); got != c.want {
			t.Errorf("%s:\n got  %q\n want %q", c.what, got, c.want)
		}
	}

	// A sender may write the key in another case. It is matched without
	// regard to case and written back in ours.
	got := setHeaderKey("Title: T\nfilename: old\n\nB\n", "FileName", "new")
	if got != "Title: T\nFileName: new\n\nB\n" {
		t.Errorf("case-insensitive replace: got %q", got)
	}
}

func TestTakeHeaderKey(t *testing.T) {
	value, rest := takeHeaderKey("Title: T\nFileName: a/b/C\nDate: d\n\nB\n", "FileName")
	if value != "a/b/C" {
		t.Errorf("value = %q, want a/b/C", value)
	}
	if want := "Title: T\nDate: d\n\nB\n"; rest != want {
		t.Errorf("rest = %q, want %q", rest, want)
	}

	value, rest = takeHeaderKey("Title: T\n\nB\n", "FileName")
	if value != "" {
		t.Errorf("an absent key gave %q", value)
	}
	if rest != "Title: T\n\nB\n" {
		t.Errorf("an absent key changed the note: %q", rest)
	}

	// Taking the only header line takes the block with it. A note that
	// carried nothing but FileName: must not begin with a blank line.
	value, rest = takeHeaderKey("FileName: only\n\nB\n", "FileName")
	if value != "only" || rest != "B\n" {
		t.Errorf("sole key: value %q rest %q", value, rest)
	}
}

// ----------------------------------------------------------------------
// Names
// ----------------------------------------------------------------------

func TestFlattenExportName(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"project/Sub/WeeklyPlan", "project-Sub-WeeklyPlan"},
		{"Welcome", "Welcome"},
		// A folder that already holds a "-" makes the flattening ambiguous
		// to the eye. That costs nothing: this name is a label for a human
		// and is never read back - FileName: inside the file is what the
		// importer uses.
		{"my-project/Note", "my-project-Note"},
		{"a//b", "a-b"},
		// Not in A-Za-z0-9._- : the recipient's filesystem is not ours to
		// assume, so the set is what Windows can store as well as Android.
		{"Заметка", "note"},
		{"проект/Plan", "Plan"},
	} {
		if got := flattenExportName(c.in); got != c.want {
			t.Errorf("flattenExportName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if n := len(flattenExportName(strings.Repeat("x", 150))); n != exportNameMaxRunes {
		t.Errorf("a long name capped at %d, want %d", n, exportNameMaxRunes)
	}
}

func TestSanitizeImportPath(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"project/Sub/WeeklyPlan", "project/Sub/WeeklyPlan"},
		{"project/Sub/WeeklyPlan.md", "project/Sub/WeeklyPlan"},
		{"../../etc/passwd", "etc/passwd"},
		{"/etc/passwd", "etc/passwd"},
		{"..", ""},
		{"../..", ""},
		{`C:\Users\bob\Note`, "Users/bob/Note"},
		{`project\Sub\Note`, "project/Sub/Note"},
		{"a/./b", "a/b"},
		{"  spaced  /  Note  ", "spaced/Note"},
		{".hidden", "hidden"},     // a leading dot hides the file
		{"trailing.", "trailing"}, // Windows cannot store a trailing dot
		{"a\x00b/N", "ab/N"},      // a control character is dropped
		{"with\nnewline", "withnewline"},
		{"", ""},
		{"...", ""},
		{"a/b/c/d/e/f/g/h/i/j", "a/b/c/d/e/f/g/h"}, // importMaxSegments
		{"Note (2)", "Note -2"},                    // one dash, not "Note -2-"
	} {
		if got := sanitizeImportPath(c.in); got != c.want {
			t.Errorf("sanitizeImportPath(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	if n := len(sanitizeImportPath(strings.Repeat("y", 200))); n != importSegmentMaxRunes {
		t.Errorf("a long segment capped at %d, want %d", n, importSegmentMaxRunes)
	}
}

// ----------------------------------------------------------------------
// Import
// ----------------------------------------------------------------------

const sampleNote = "Title: Weekly plan\n" +
	"Date: 2026-08-01 09:00:00\n" +
	"Modified: 2026-08-02 10:00:00\n" +
	"Category: Notes\n" +
	"FileName: project/Sub/WeeklyPlan\n" +
	"\nThe body.\n"

func TestImportNote(t *testing.T) {
	a := newTestApp(t)

	res, err := a.importNote([]byte(sampleNote), "project-Sub-WeeklyPlan.md", testNow)
	if err != nil {
		t.Fatal(err)
	}
	if res.Name != "incoming/project/Sub/WeeklyPlan" ||
		res.Rel != "project/Sub/WeeklyPlan" || res.Base != "WeeklyPlan" {
		t.Fatalf("landed at %+v", res)
	}

	// FileName: is gone, because the note no longer lives there. Imported:
	// is added. The sender's Date: and Modified: are facts about their note
	// and are untouched.
	want := "Title: Weekly plan\n" +
		"Date: 2026-08-01 09:00:00\n" +
		"Modified: 2026-08-02 10:00:00\n" +
		"Category: Notes\n" +
		"Imported: 2026-08-09 12:34:56\n" +
		"\nThe body.\n"
	if got := incomingFile(t, a, "project/Sub/WeeklyPlan.md"); got != want {
		t.Errorf("imported note:\n got  %q\n want %q", got, want)
	}
}

func TestImportNoteCollisions(t *testing.T) {
	a := newTestApp(t)
	var bases []string
	for i := 0; i < 3; i++ {
		res, err := a.importNote([]byte(sampleNote), "", testNow)
		if err != nil {
			t.Fatal(err)
		}
		bases = append(bases, res.Base)
	}
	if got := strings.Join(bases, ","); got != "WeeklyPlan,WeeklyPlan-2,WeeklyPlan-3" {
		t.Errorf("collision names = %s", got)
	}
	if !strings.Contains(incomingFile(t, a, "project/Sub/WeeklyPlan-2.md"), "The body.") {
		t.Error("the second copy is not on disk")
	}
}

func TestImportNoteNameFallbacks(t *testing.T) {
	a := newTestApp(t)
	titled := "Title: A Shared Thought\n\nbody\n"

	res, _ := a.importNote([]byte(titled), "SharedFile.md", testNow)
	if res.Base != "SharedFile" {
		t.Errorf("with a display name: %q, want SharedFile", res.Base)
	}
	res, _ = a.importNote([]byte(titled), "", testNow)
	if res.Base != "A Shared Thought" {
		t.Errorf("with no display name: %q, want the Title:", res.Base)
	}
	res, _ = a.importNote([]byte("just a body, no header\n"), "", testNow)
	if res.Base != "note-2026-08-09-123456" {
		t.Errorf("with nothing at all: %q, want the timestamp", res.Base)
	}
	// A note with no header block still imports, and gains one.
	if got := incomingFile(t, a, "note-2026-08-09-123456.md"); !strings.HasPrefix(got, "Imported: 2026-08-09 12:34:56\n\njust a body") {
		t.Errorf("headerless import: %q", got)
	}
}

// The one that matters. filepath.Join RESOLVES a "..", so the assertion is
// on the tree, not on the string the sanitizer returned.
func TestImportNoteCannotEscape(t *testing.T) {
	a := newTestApp(t)

	for _, name := range []string{
		"../../../../etc/pwned",
		"/etc/pwned",
		`..\..\pwned`,
		"../pwned",
	} {
		if _, err := a.importNote([]byte("FileName: "+name+"\n\nbad\n"), "", testNow); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}

	var strays []string
	filepath.Walk(a.StorageDir, func(p string, fi os.FileInfo, e error) error {
		if e != nil || fi.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(a.StorageDir, p)
		if rel = filepath.ToSlash(rel); !strings.HasPrefix(rel, "md/incoming/") {
			strays = append(strays, rel)
		}
		return nil
	})
	if len(strays) > 0 {
		t.Errorf("an import wrote outside md/incoming/: %v", strays)
	}
}

func TestImportNoteNormalizesNewlines(t *testing.T) {
	a := newTestApp(t)
	crlf := "Title: T\r\nFileName: CR/Note\r\n\r\nbody\r\n"
	if _, err := a.importNote([]byte(crlf), "", testNow); err != nil {
		t.Fatal(err)
	}
	want := "Title: T\nImported: 2026-08-09 12:34:56\n\nbody\n"
	if got := incomingFile(t, a, "CR/Note.md"); got != want {
		t.Errorf("CRLF note:\n got  %q\n want %q", got, want)
	}
}

func TestImportNoteRejectsEmpty(t *testing.T) {
	a := newTestApp(t)
	if _, err := a.importNote([]byte("   \n\n"), "", testNow); err == nil {
		t.Error("an empty note was imported")
	}
}

// ----------------------------------------------------------------------
// The incoming index
// ----------------------------------------------------------------------

// The index is a LIST, and a blank line after the header block is what makes
// it one. "* 2026-08-09 12:34 · [x](y)" holds a colon and does not begin with
// a space, '#' or '<', so isHeaderFirstLine reads it as another "Key: value":
// with one newline in front of it the line joins the header block, never
// renders, and the next arrival is appended AFTER it - which silently turns
// "newest first" into oldest first.
func TestIncomingIndex(t *testing.T) {
	a := newTestApp(t)
	for i := 0; i < 3; i++ {
		if _, err := a.importNote([]byte(sampleNote), "", testNow); err != nil {
			t.Fatal(err)
		}
	}
	idx := incomingFile(t, a, "incoming.md")

	if !strings.HasPrefix(idx, "Title: Incoming notes\n") {
		t.Errorf("the index has no header block:\n%s", idx)
	}
	fm := splitFrontMatter(idx)
	if !fm.HasHeader {
		t.Fatal("the index does not parse as a note with a header")
	}
	if strings.Contains(fm.Header, "[WeeklyPlan]") {
		t.Errorf("a list line was swallowed into the header block:\n%s", fm.Header)
	}
	if n := strings.Count(fm.Body, "* 2026-08-09 12:34 · ["); n != 3 {
		t.Errorf("%d list lines in the body, want 3:\n%s", n, fm.Body)
	}

	// The link text carries the collision index; the target is the path
	// under incoming/, which is where the index itself lives.
	for _, want := range []string{
		"* 2026-08-09 12:34 · [WeeklyPlan](project/Sub/WeeklyPlan)\n",
		"* 2026-08-09 12:34 · [WeeklyPlan-2](project/Sub/WeeklyPlan-2)\n",
		"* 2026-08-09 12:34 · [WeeklyPlan-3](project/Sub/WeeklyPlan-3)\n",
	} {
		if !strings.Contains(idx, want) {
			t.Errorf("missing line %q in:\n%s", want, idx)
		}
	}
	if strings.Index(idx, "WeeklyPlan-3") > strings.Index(idx, "WeeklyPlan-2") {
		t.Errorf("the list is not newest first:\n%s", idx)
	}
}

// ----------------------------------------------------------------------
// Export, and the two-hop rule
// ----------------------------------------------------------------------

func TestExportNoteSource(t *testing.T) {
	a := newTestApp(t)
	dir := filepath.Join(a.StorageDir, "md", "project", "Sub")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	src := "Title: Orig\nDate: 2026-01-01 00:00:00\n\nOriginal body.\n"
	if err := os.WriteFile(filepath.Join(dir, "Orig.md"), []byte(src), 0644); err != nil {
		t.Fatal(err)
	}

	data, filename, err := a.exportNoteSource("project/Sub/Orig")
	if err != nil {
		t.Fatal(err)
	}
	if filename != "project-Sub-Orig.md" {
		t.Errorf("attachment name = %q", filename)
	}
	want := "Title: Orig\nDate: 2026-01-01 00:00:00\nFileName: project/Sub/Orig\n\nOriginal body.\n"
	if string(data) != want {
		t.Errorf("exported bytes:\n got  %q\n want %q", data, want)
	}

	// The stored note is not touched. An export is a read.
	if after, _ := os.ReadFile(filepath.Join(dir, "Orig.md")); string(after) != src {
		t.Errorf("the export wrote to the note: %q", after)
	}

	if _, _, err := a.exportNoteSource("style.css"); err == nil {
		t.Error("a static asset was exported as a note")
	}
}

// A -> B -> C. The second import must REPLACE the Imported: line, not add a
// second one, and the second export must replace FileName: rather than let
// two of them travel.
func TestExportImportTwoHops(t *testing.T) {
	a := newTestApp(t)
	dir := filepath.Join(a.StorageDir, "md", "project", "Sub")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "Orig.md"),
		[]byte("Title: Orig\nDate: 2026-01-01 00:00:00\n\nOriginal body.\n"), 0644)

	data, filename, err := a.exportNoteSource("project/Sub/Orig")
	if err != nil {
		t.Fatal(err)
	}
	hop1, err := a.importNote(data, filename, testNow)
	if err != nil {
		t.Fatal(err)
	}

	data2, filename2, err := a.exportNoteSource(hop1.Name)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(data2), "FileName:"); n != 1 {
		t.Errorf("%d FileName: lines in flight, want 1:\n%s", n, data2)
	}
	later := testNow.Add(time.Hour)
	hop2, err := a.importNote(data2, filename2, later)
	if err != nil {
		t.Fatal(err)
	}

	got := incomingFile(t, a, hop2.Rel+".md")
	if n := strings.Count(got, "Imported:"); n != 1 {
		t.Errorf("%d Imported: lines after two hops, want 1:\n%s", n, got)
	}
	if !strings.Contains(got, "Imported: 2026-08-09 13:34:56") {
		t.Errorf("Imported: is not the latest hop:\n%s", got)
	}
	if strings.Contains(got, "FileName:") {
		t.Errorf("FileName: survived an import:\n%s", got)
	}
	if !strings.Contains(got, "Date: 2026-01-01 00:00:00") ||
		!strings.Contains(got, "Original body.") {
		t.Errorf("the note did not survive two hops:\n%s", got)
	}
}
