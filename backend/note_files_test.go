package backend

// Tests for the md/ <-> html/ mirror of plain files kept beside a note
// (note_files.go).
//
// The property under test is not "a copy happened". It is that the pair
// SETTLES: each direction runs on its own event, a copy carries the time of
// its source, and a start that follows a save must find nothing to do. A
// mirror that copies a file back and forth on every start would pass a
// naive "the content matches" test and rewrite the user's notes tree
// forever.

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeStamped writes body at rel inside tree ("md" or "html") and gives the
// file an explicit modification time, because every decision in this file is
// made on that time.
func writeStamped(t *testing.T, a *App, tree, rel, body string, mod time.Time) string {
	t.Helper()
	full := filepath.Join(a.StorageDir, tree, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(full, mod, mod); err != nil {
		t.Fatal(err)
	}
	return full
}

// readOrMissing keeps a missing file readable as an assertion instead of a
// fatal, so a test can state that a file must NOT be there.
func readOrMissing(t *testing.T, a *App, tree, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(a.StorageDir, tree, filepath.FromSlash(rel)))
	if err != nil {
		return "<missing>"
	}
	return string(b)
}

func modTimeOf(t *testing.T, a *App, tree, rel string) time.Time {
	t.Helper()
	info, err := os.Stat(filepath.Join(a.StorageDir, tree, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("stat %s/%s: %v", tree, rel, err)
	}
	return info.ModTime()
}

func TestIsSyncedNoteFile(t *testing.T) {
	for _, c := range []struct {
		name string
		want bool
	}{
		{"log.txt", true},
		{"SHOUT.TXT", true}, // the extension is matched case-insensitively
		{"project/data.txt", true},
		// A markdown file in md/ is a NOTE. Copying it into html/ would put
		// the source of a page next to that page's compiled cache.
		{"Note.md", false},
		{"Note.html", false},
		{"photo.png", false},
		{"app.js", false},
		{"README", false},
	} {
		if got := isSyncedNoteFile(c.name); got != c.want {
			t.Errorf("isSyncedNoteFile(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSyncNoteFilesToHTML(t *testing.T) {
	a := newTestApp(t)
	old := time.Now().Add(-2 * time.Hour)
	mid := time.Now().Add(-1 * time.Hour)
	recent := time.Now().Add(-1 * time.Minute)

	writeStamped(t, a, "md", "log.txt", "from md", mid)          // no html copy
	writeStamped(t, a, "md", "newer.txt", "md wins", recent)     // md is newer
	writeStamped(t, a, "html", "newer.txt", "stale html", old)   //
	writeStamped(t, a, "md", "older.txt", "md older", old)       // html is newer
	writeStamped(t, a, "html", "older.txt", "html wins", recent) //
	writeStamped(t, a, "md", "same.txt", "md same", mid)         // same time
	writeStamped(t, a, "html", "same.txt", "html same", mid)     //
	writeStamped(t, a, "md", "Note.md", "# a note", mid)         // never copied
	writeStamped(t, a, "md", "project/data.txt", "nested", mid)  // subdirectory
	writeStamped(t, a, "md", "SHOUT.TXT", "upper", mid)          // uppercase ext

	a.syncNoteFilesToHTML()

	for _, c := range []struct{ what, tree, rel, want string }{
		{"a file with no html copy is copied", "html", "log.txt", "from md"},
		{"a newer md file replaces the html copy", "html", "newer.txt", "md wins"},
		{"a newer html copy is left alone", "html", "older.txt", "html wins"},
		{"and its md file is not touched either", "md", "older.txt", "md older"},
		{"equal times mean nothing to do", "html", "same.txt", "html same"},
		{"a note is not copied", "html", "Note.md", "<missing>"},
		{"a subdirectory is kept", "html", "project/data.txt", "nested"},
		{"the extension is case-insensitive", "html", "SHOUT.TXT", "upper"},
	} {
		if got := readOrMissing(t, a, c.tree, c.rel); got != c.want {
			t.Errorf("%s: %s/%s = %q, want %q", c.what, c.tree, c.rel, got, c.want)
		}
	}

	// The copy carries the time of its source. This is what makes the run
	// below a no-op instead of an endless ping-pong.
	if got := modTimeOf(t, a, "html", "log.txt"); !got.Equal(mid) {
		t.Errorf("copy time = %v, want the source's %v", got, mid)
	}
}

// A second start must find nothing to do. Without the time being carried
// across, every copy is newer than its source and the pair never settles.
func TestSyncNoteFilesToHTMLIsIdempotent(t *testing.T) {
	a := newTestApp(t)
	writeStamped(t, a, "md", "log.txt", "body", time.Now().Add(-time.Hour))

	a.syncNoteFilesToHTML()
	first := modTimeOf(t, a, "html", "log.txt")

	a.syncNoteFilesToHTML()
	if second := modTimeOf(t, a, "html", "log.txt"); !second.Equal(first) {
		t.Errorf("the second run rewrote the copy: %v then %v", first, second)
	}
}

func TestSyncNoteFileToMD(t *testing.T) {
	a := newTestApp(t)
	recent := time.Now().Add(-time.Minute)

	// The editor writes html/; the file goes back to md/ with it.
	saved := writeStamped(t, a, "html", "log.txt", "edited in the browser", recent)
	a.syncNoteFileToMD(saved)
	if got := readOrMissing(t, a, "md", "log.txt"); got != "edited in the browser" {
		t.Errorf("md copy = %q, want the saved text", got)
	}
	if !modTimeOf(t, a, "md", "log.txt").Equal(modTimeOf(t, a, "html", "log.txt")) {
		t.Error("the write-back did not carry the time, so the next start would copy it again")
	}

	// And the pair is settled: a start right after a save changes nothing.
	before := modTimeOf(t, a, "html", "log.txt")
	a.syncNoteFilesToHTML()
	if after := modTimeOf(t, a, "html", "log.txt"); !after.Equal(before) {
		t.Error("a start after a save copied md/ back over the file it came from")
	}

	// A kind that is not mirrored is not written back.
	js := writeStamped(t, a, "html", "app.js", "var x = 1;", recent)
	a.syncNoteFileToMD(js)
	if got := readOrMissing(t, a, "md", "app.js"); got != "<missing>" {
		t.Errorf("a .js file reached md/: %q", got)
	}
}

// resolvePageName builds its html path with filepath.Join, which RESOLVES a
// "../" in a name instead of refusing it. The write-back must not carry such
// a path into the notes tree.
func TestSyncNoteFileToMDRefusesEscape(t *testing.T) {
	a := newTestApp(t)
	outside := filepath.Join(a.StorageDir, "escape.txt")
	if err := os.WriteFile(outside, []byte("should not travel"), 0644); err != nil {
		t.Fatal(err)
	}

	a.syncNoteFileToMD(outside)

	if got := readOrMissing(t, a, "md", "escape.txt"); got != "<missing>" {
		t.Errorf("a path outside html/ was copied into md/: %q", got)
	}
	if _, err := os.Stat(filepath.Join(a.StorageDir, "md", "..", "escape.txt")); err != nil {
		t.Error("the original file outside the trees was disturbed")
	}
}

// The mirror is only useful if the copy is then SERVED, and served as text.
// ".txt" is not in Go's own MIME table, so without the builtinMIME row the
// file has no content type on a device with no /etc/mime.types - and
// editableFileType, which reads the same table, then calls it "not text".
func TestTxtIsServedAndEditableText(t *testing.T) {
	a := newTestApp(t)
	if ct := a.resolveContentType("log.txt"); ct != "text/plain; charset=utf-8" {
		t.Errorf("content type of .txt = %q, want text/plain; charset=utf-8", ct)
	}
	if !a.editableFileType("log.txt") {
		t.Error("a .txt file is text and must open in the editor")
	}
	if !a.filesEditable("log.txt") {
		t.Error("a .txt file must offer an edit link in the file index")
	}
}
