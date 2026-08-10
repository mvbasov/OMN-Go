package backend

// Tests for note exchange (note_exchange.go) and the two header-key helpers
// it needs (header_block.go).
//
// Two properties carry the weight here.
//
// A NAME FROM ANOTHER DEVICE IS NOT A PATH. FileName: is a line of text that
// arrived from a stranger's phone through Telegram. The sanitizer table below
// is the list of things it has tried to be, and the containment test asserts
// the resolved path, not the string - because filepath.Join RESOLVES a "..".
//
// A KEY IS SET, NOT APPENDED. A note can hop twice, and a header block that
// carries "Imported:" two times has no defined meaning: parseHeaderBlock
// hands the first one to whatever reads it, and which is first is an accident
// of the order the hops ran in.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
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
	hb := parseHeaderBlock(idx)
	if !hb.HasHeader {
		t.Fatal("the index does not parse as a note with a header")
	}
	if strings.Contains(hb.Header, "omn-incoming-when") {
		t.Errorf("a list line was swallowed into the header block:\n%s", hb.Header)
	}
	if n := strings.Count(hb.Body, `* <span class="omn-incoming-when">`); n != 3 {
		t.Errorf("%d list lines in the body, want 3:\n%s", n, hb.Body)
	}

	// The link TEXT is the note's Title with the collision index carried
	// into it; the TARGET is the path under incoming/, which is where the
	// index itself lives. The date leads the line in its own element, so it
	// can be set smaller than the title a reader is looking for.
	const when = `* <span class="omn-incoming-when">2026-08-09 12:34</span> · `
	for _, want := range []string{
		when + `[Weekly plan](project/Sub/WeeklyPlan)` + "\n",
		when + `[Weekly plan (2)](project/Sub/WeeklyPlan-2)` + "\n",
		when + `[Weekly plan (3)](project/Sub/WeeklyPlan-3)` + "\n",
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

// ----------------------------------------------------------------------
// The endpoints
// ----------------------------------------------------------------------

// exchangeReq drives one handler directly. The handlers are registered behind
// authMiddleware in server.go and TestBaseline_RouteSet pins that; what these
// tests are about is the request and answer shapes on the other side of it.
func exchangeReq(t *testing.T, h http.HandlerFunc, method, target string,
	body io.Reader, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, body)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func decodeExchange(t *testing.T, rec *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	out := map[string]string{}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("answer is not a JSON object: %v\n%s", err, rec.Body.String())
	}
	return out
}

func TestHandleExportNote(t *testing.T) {
	a := newTestApp(t)
	dir := filepath.Join(a.StorageDir, "md", "project", "Sub")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "Orig.md"),
		[]byte("Title: Orig\nDate: 2026-01-01 00:00:00\n\nBody.\n"), 0644)

	rec := exchangeReq(t, a.handleExportNote, http.MethodGet,
		"/api/export/note?name=project/Sub/Orig", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/markdown") {
		t.Errorf("Content-Type %q", ct)
	}
	// flattenExportName leaves only A-Za-z0-9._- , so the name needs no
	// quoting and cannot close the header early. This asserts that property
	// where it is relied on.
	if cd := rec.Header().Get("Content-Disposition"); cd != `attachment; filename="project-Sub-Orig.md"` {
		t.Errorf("Content-Disposition %q", cd)
	}
	if !strings.Contains(rec.Body.String(), "FileName: project/Sub/Orig") {
		t.Errorf("the export carries no FileName:\n%s", rec.Body.String())
	}

	if rec := exchangeReq(t, a.handleExportNote, http.MethodGet, "/api/export/note", nil, ""); rec.Code != http.StatusBadRequest {
		t.Errorf("no name gave %d, want 400", rec.Code)
	}
	if rec := exchangeReq(t, a.handleExportNote, http.MethodGet, "/api/export/note?name=Nope", nil, ""); rec.Code != http.StatusNotFound {
		t.Errorf("a missing note gave %d, want 404", rec.Code)
	}
	if rec := exchangeReq(t, a.handleExportNote, http.MethodPost, "/api/export/note?name=x", nil, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST gave %d, want 405", rec.Code)
	}
}

// The Android caller: raw bytes, with the attachment's name as ?name= .
func TestHandleImportNoteRawBody(t *testing.T) {
	a := newTestApp(t)
	rec := exchangeReq(t, a.handleImportNote, http.MethodPost,
		"/api/import/note?name=project-Sub-WeeklyPlan.md",
		strings.NewReader(sampleNote), "text/markdown")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeExchange(t, rec)
	if got["status"] != "success" ||
		got["name"] != "incoming/project/Sub/WeeklyPlan" ||
		got["base"] != "WeeklyPlan" ||
		got["url"] != "/incoming/project/Sub/WeeklyPlan.html" {
		t.Errorf("answer = %v", got)
	}
	if _, ok := got["warning"]; ok {
		t.Errorf("unexpected warning: %v", got)
	}
	if !strings.Contains(incomingFile(t, a, "project/Sub/WeeklyPlan.md"), "The body.") {
		t.Error("the note is not on disk")
	}
}

// The desktop caller: a browser file input.
func TestHandleImportNoteMultipart(t *testing.T) {
	a := newTestApp(t)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "Shared.md")
	if err != nil {
		t.Fatal(err)
	}
	fw.Write([]byte("Title: From a browser\n\nbody\n"))
	mw.Close()

	rec := exchangeReq(t, a.handleImportNote, http.MethodPost,
		"/api/import/note", &buf, mw.FormDataContentType())
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	// No FileName: in the note, so the upload's own name is the fallback.
	if got := decodeExchange(t, rec); got["base"] != "Shared" {
		t.Errorf("answer = %v, want base Shared", got)
	}
}

func TestHandleImportNoteRefusals(t *testing.T) {
	a := newTestApp(t)

	if rec := exchangeReq(t, a.handleImportNote, http.MethodGet, "/api/import/note", nil, ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET gave %d, want 405", rec.Code)
	}
	if rec := exchangeReq(t, a.handleImportNote, http.MethodPost, "/api/import/note",
		strings.NewReader("   \n"), "text/markdown"); rec.Code != http.StatusBadRequest {
		t.Errorf("an empty note gave %d, want 400", rec.Code)
	}
	// Larger than the upload limit: refused whole rather than imported
	// truncated.
	a.WithConfig(func(c *Config) { c.MaxUploadSizeMB = 1 })
	big := strings.Repeat("x", 2*1024*1024)
	rec := exchangeReq(t, a.handleImportNote, http.MethodPost, "/api/import/note",
		strings.NewReader(big), "text/markdown")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("an oversized note gave %d, want 413", rec.Code)
	}
}

// ----------------------------------------------------------------------
// The starter index and its marker
// ----------------------------------------------------------------------

func TestEnsureIncomingIndex(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureIncomingIndex(testNow); err != nil {
		t.Fatal(err)
	}
	idx := incomingFile(t, a, "incoming.md")

	if !strings.HasPrefix(idx, "Title: Incoming notes\n") {
		t.Errorf("no header block:\n%s", idx)
	}
	if !strings.Contains(idx, "Date: 2026-08-09 12:34:56") {
		t.Error("the %%DATE%% placeholder was not filled")
	}
	if strings.Contains(idx, "%%") {
		t.Errorf("an unfilled placeholder is left:\n%s", idx)
	}
	// The marker is BODY, not metadata: it opens with "<", so
	// isHeaderFirstLine ends the header block on it.
	if strings.Index(idx, incomingListMarker) < 0 {
		t.Errorf("the starter carries no list marker:\n%s", idx)
	}
	hb := parseHeaderBlock(idx)
	if !hb.HasHeader || strings.Contains(hb.Header, incomingListMarker) {
		t.Errorf("the marker was read as part of the header block:\n%s", hb.Header)
	}

	// The note belongs to the user from here. A second call must not touch it.
	mine := "Title: Mine\n\nmy own page\n"
	os.WriteFile(filepath.Join(a.StorageDir, "md", "incoming", "incoming.md"), []byte(mine), 0644)
	if err := a.ensureIncomingIndex(testNow.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got := incomingFile(t, a, "incoming.md"); got != mine {
		t.Errorf("the user's page was overwritten:\n%s", got)
	}
}

// With the starter in place, a line goes BELOW the marker so the receive box
// stays at the top, and the note script survives every insertion.
func TestIncomingIndexMarker(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureIncomingIndex(testNow); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if _, err := a.importNote([]byte(sampleNote), "", testNow.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}
	idx := incomingFile(t, a, "incoming.md")

	marker := strings.Index(idx, incomingListMarker)
	if marker < 0 {
		t.Fatalf("the marker is gone:\n%s", idx)
	}
	for _, target := range []string{"WeeklyPlan", "WeeklyPlan-2", "WeeklyPlan-3"} {
		at := strings.Index(idx, `](project/Sub/`+target+`)`)
		if at < 0 {
			t.Errorf("no line for %s", target)
			continue
		}
		if at < marker {
			t.Errorf("the line for %s went above the marker", target)
		}
	}
	if strings.Index(idx, "WeeklyPlan-3)") > strings.Index(idx, "WeeklyPlan-2)") {
		t.Errorf("not newest first:\n%s", idx)
	}
	// The note holds NOTHING but its header block, the marker and the list.
	// The receive box is application chrome and lives in modals.html.
	if strings.Contains(idx, "<script") || strings.Contains(idx, "omnIncoming") {
		t.Errorf("the note carries chrome of its own:\n%s", idx)
	}
	// The header block still ends immediately at the marker, with no blank
	// line accumulating in front of it.
	if !strings.Contains(idx, "Category: Notes\n\n"+incomingListMarker) {
		t.Errorf("the gap above the marker changed:\n%s", idx)
	}
}

// A user who rewrote the page and dropped the marker still gets a list, at
// the top of the body - which is what every note did before the marker.
func TestIncomingIndexWithoutMarker(t *testing.T) {
	a := newTestApp(t)
	dir := filepath.Join(a.StorageDir, "md", "incoming")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "incoming.md"), []byte("Title: Mine\n\nold text\n"), 0644)

	if _, err := a.importNote([]byte(sampleNote), "", testNow); err != nil {
		t.Fatal(err)
	}
	idx := incomingFile(t, a, "incoming.md")
	if !strings.HasPrefix(idx, `Title: Mine`+"\n\n"+`* <span class="omn-incoming-when">2026-08-09 12:34</span> · [Weekly plan](project/Sub/WeeklyPlan)`) {
		t.Errorf("the line did not go to the top of the body:\n%s", idx)
	}
	if !strings.Contains(idx, "old text") {
		t.Errorf("the user's text was lost:\n%s", idx)
	}
}

// ----------------------------------------------------------------------
// The documentation links (26.08.40)
// ----------------------------------------------------------------------

// Two bundled notes link to the Incoming notes page, and that link is the
// ONLY way a desktop user finds the receive box - nothing in the header
// leads there. The target is spelled by hand in Markdown, so a rename of
// incomingDirName or incomingIndexBase would leave two dead links behind
// with nothing to say so. This test is that "something".
//
// The manual is also checked for its own table-of-contents entry, because
// the anchor is derived from the heading text and the two are written
// separately.
func TestBundledNotesLinkToTheIncomingIndex(t *testing.T) {
	want := "(" + incomingDirName + "/" + incomingIndexBase + ")"

	for _, name := range []string{"UserManual.md", "Welcome.md"} {
		data, err := staticFS.ReadFile("frontend/md/" + name)
		if err != nil {
			t.Fatalf("cannot read the embedded %s: %v", name, err)
		}
		if !strings.Contains(string(data), "[Incoming notes]"+want) {
			t.Errorf("%s has no [Incoming notes]%s link", name, want)
		}
	}

	manual, err := staticFS.ReadFile("frontend/md/UserManual.md")
	if err != nil {
		t.Fatal(err)
	}
	const heading = "## Send and receive one note"
	const tocEntry = "- [Send and receive one note](#send-and-receive-one-note)"
	if !strings.Contains(string(manual), heading) {
		t.Errorf("UserManual.md has no %q heading", heading)
	}
	if !strings.Contains(string(manual), tocEntry) {
		t.Errorf("UserManual.md has no table-of-contents entry %q", tocEntry)
	}
}

// The link the manual writes must be the link the renderer produces, and
// that is a question about rewriteInternalLink, not about the Markdown.
func TestIncomingIndexLinkRewrites(t *testing.T) {
	a := newTestApp(t)
	in := incomingDirName + "/" + incomingIndexBase
	want := in + ".html"
	if got := a.rewriteInternalLink(in); got != want {
		t.Errorf("rewriteInternalLink(%q) = %q, want %q", in, got, want)
	}
}

// ----------------------------------------------------------------------
// The description block (26.08.41)
// ----------------------------------------------------------------------

// The text a note offers as the MESSAGE beside the file. The fence is
// deliberately forgiving, so the table is mostly about what still counts as
// a description and what does not.
func TestNoteDescription(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"the documented form",
			"Title: T\nTags: Test, Document\n\n<!--- DESCRIPTION:\nThere is some\ndescription\n--->\n\nBody.\n",
			"There is some\ndescription"},
		{"the standard comment spelling", "<!-- DESCRIPTION:\nOne line\n-->", "One line"},
		{"no colon, lower case", "<!--- description\nText here\n--->", "Text here"},
		{"all on one line", "<!--- DESCRIPTION: Short one --->", "Short one"},
		{"CRLF source", "<!--- DESCRIPTION:\r\nTwo\r\nlines\r\n--->", "Two\nlines"},
		{"non-ASCII", "<!--- DESCRIPTION:\nПривет, это заметка\n--->", "Привет, это заметка"},
		{"the first block wins",
			"<!--- DESCRIPTION:\nfirst\n--->\n<!--- DESCRIPTION:\nsecond\n--->", "first"},
		{"lower down in the note still counts",
			"Title: T\n\nBody first.\n\n<!--- DESCRIPTION:\nlate\n--->", "late"},
		{"no description", "Title: T\n\nJust a note.\n", ""},
		{"empty description", "<!--- DESCRIPTION:\n\n--->", ""},
		// The incoming index carries one of these on every install, and it
		// must not read as a description of anything.
		{"an ordinary comment is not one", "Title: T\n\n" + incomingListMarker + "\n\n* a line\n", ""},
		{"the word inside prose is not one", "Title: T\n\nDESCRIPTION: not in a comment\n", ""},
	}
	for _, tt := range tests {
		if got := noteDescription(tt.in); got != tt.want {
			t.Errorf("%s: noteDescription = %q, want %q", tt.name, got, tt.want)
		}
	}
}

// Telegram refuses a caption over 1024 characters outright, so the cap is
// what keeps a long description from arriving as no description.
func TestNoteDescriptionCap(t *testing.T) {
	// Runes, not bytes: a Cyrillic description of 1500 letters is 3000 bytes
	// and must still come back as 1000 letters.
	long := strings.Repeat("ф", descriptionMaxRunes+500)
	got := noteDescription("<!--- DESCRIPTION:\n" + long + "\n--->")
	if n := len([]rune(got)); n != descriptionMaxRunes {
		t.Errorf("a long description came back as %d runes, want %d", n, descriptionMaxRunes)
	}
}

// The export answers with the description in a header, and the value is safe
// to put in one: base64, so no newline of the description can end the field
// and no non-ASCII letter can be mangled by it.
func TestHandleExportNoteDescriptionHeader(t *testing.T) {
	a := newTestApp(t)
	os.WriteFile(filepath.Join(a.StorageDir, "md", "WithDesc.md"),
		[]byte("Title: T\nTags: Test, Document\n\n<!--- DESCRIPTION:\nПривет\nмир\n--->\n\nBody.\n"), 0644)
	os.WriteFile(filepath.Join(a.StorageDir, "md", "Plain.md"),
		[]byte("Title: T\n\nBody.\n"), 0644)

	rec := exchangeReq(t, a.handleExportNote, http.MethodGet,
		"/api/export/note?name=WithDesc", nil, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	value := rec.Header().Get(headerDescription)
	if value == "" {
		t.Fatalf("no %s header", headerDescription)
	}
	if strings.ContainsAny(value, "\r\n") {
		t.Errorf("the header value holds a line break: %q", value)
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("the header value is not base64: %v", err)
	}
	if string(raw) != "Привет\nмир" {
		t.Errorf("decoded %q, want %q", raw, "Привет\nмир")
	}

	// The description STAYS in the note that travels: it is part of the
	// note, and the receiver has to be able to send it on with one.
	if !strings.Contains(rec.Body.String(), "DESCRIPTION:") {
		t.Errorf("the export dropped the description block:\n%s", rec.Body.String())
	}

	// No description, no header. An empty one is not the same as none: on
	// the Android side an empty EXTRA_TEXT opens a message body and waits.
	rec = exchangeReq(t, a.handleExportNote, http.MethodGet,
		"/api/export/note?name=Plain", nil, "")
	if got := rec.Header().Get(headerDescription); got != "" {
		t.Errorf("a note with no description answered %s: %q", headerDescription, got)
	}
}

// ----------------------------------------------------------------------
// The incoming index: link text and the receive box (26.08.44)
// ----------------------------------------------------------------------

// The link text is the note's own Title. The FILE name is what OMN-Go had to
// call it - a sanitized path - and it is the fallback, not the first choice.
func TestIncomingLabel(t *testing.T) {
	for _, tt := range []struct{ name, title, base, index, want string }{
		{"the title wins", "Weekly plan", "WeeklyPlan", "", "Weekly plan"},
		{"no title falls back to the file name", "", "WeeklyPlan", "", "WeeklyPlan"},
		{"a blank title falls back too", "   ", "WeeklyPlan", "", "WeeklyPlan"},
		{"the collision index is carried", "Weekly plan", "WeeklyPlan-2", "2", "Weekly plan (2)"},
		{"the fallback already carries its own", "", "WeeklyPlan-2", "2", "WeeklyPlan-2"},
		{"whitespace is collapsed to one line", "  Weekly \t plan  ", "X", "", "Weekly plan"},
		{"control characters are dropped", "Week\x00ly\x07 plan", "X", "", "Weekly plan"},
		{"non-ASCII is kept", "Мой план", "X", "", "Мой план"},
		{"link syntax is taken out", "A [b] c", "X", "", "A b c"},
		// Only the dangerous characters go. The "/" of a closing tag is
		// harmless on its own and is left where it fell.
		{"raw HTML markers are taken out", "A <b>c", "X", "", "A b c"},
		{"a closing tag leaves its slash", "A <b>c</b>", "X", "", "A b c /b"},
		{"an ampersand is taken out", "Cats & Dogs", "X", "", "Cats Dogs"},
		{"emphasis and code marks go", "a *b* `c` ~d~", "X", "", "a b c d"},
		{"a backslash goes", `a \ b`, "X", "", "a b"},
		{"an underscore stays - it does not emphasize inside a word", "my_note", "X", "", "my_note"},
		{"a title of nothing but markup falls back", "[]<>&", "X", "", "X"},
	} {
		if got := incomingLabel(tt.title, tt.base, tt.index); got != tt.want {
			t.Errorf("%s: incomingLabel(%q, %q, %q) = %q, want %q",
				tt.name, tt.title, tt.base, tt.index, got, tt.want)
		}
	}

	long := strings.Repeat("ф", 200)
	got := incomingLabel(long, "X", "")
	if n := len([]rune(got)); n != incomingLabelMaxRunes+1 {
		t.Errorf("a long title came back as %d runes, want %d and an ellipsis",
			n, incomingLabelMaxRunes)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("a title that was cut does not say so: %q", got)
	}
}

// A Title arrives from another device. Everything in it that would end the
// link early, start markup of its own, or reach the page as raw HTML is
// taken out before the line is written - because a Markdown link label is
// not escaped by anything downstream.
func TestIncomingIndexLineIsSafeMarkdown(t *testing.T) {
	a := newTestApp(t)
	if err := a.ensureIncomingIndex(testNow); err != nil {
		t.Fatal(err)
	}
	err := a.addIncomingIndexLine(importResult{
		Rel:   "My Notes/Plan A",
		Base:  "Plan A",
		Label: incomingLabel("Cats & Dogs <script>x</script> ] * ~ `code`", "Plan A", ""),
	}, testNow)
	if err != nil {
		t.Fatal(err)
	}

	// Only the appended line: the page itself legitimately carries a
	// <script>, the receive box's own.
	line := ""
	for _, l := range strings.Split(incomingFile(t, a, "incoming.md"), "\n") {
		if strings.HasPrefix(l, "* <span") {
			line = l
		}
	}
	if line == "" {
		t.Fatal("no list line was written")
	}
	label := line[strings.Index(line, "· [")+len("· ") : strings.LastIndex(line, "](")+1]
	for _, bad := range []string{"<", ">", "&", "]", "*", "~", "`", "\\", "|"} {
		if strings.Contains(strings.TrimSuffix(strings.TrimPrefix(label, "["), "]"), bad) {
			t.Errorf("%q survived into the link label %q", bad, label)
		}
	}
	if !strings.Contains(line, "Cats Dogs script x") {
		t.Errorf("the readable part of the title was lost:\n%s", line)
	}

	// A note name may hold a space, and a bare Markdown destination may not.
	if !strings.Contains(line, "](My%20Notes/Plan%20A)") {
		t.Errorf("the space in the path was not encoded:\n%s", line)
	}
}

// The template and the constants that read it must agree, or a line lands in
// the wrong half of the page. The starter is deliberately almost empty: the
// receive box is application chrome (modals.html) and not the user's note.
func TestIncomingTemplateCarriesItsMarkers(t *testing.T) {
	starter := incomingIndexStarter(testNow)
	if !strings.Contains(starter, incomingListMarker) {
		t.Errorf("the incoming index template does not carry %q", incomingListMarker)
	}
	for _, unwanted := range []string{"<script", "omnIncoming", "<details"} {
		if strings.Contains(starter, unwanted) {
			t.Errorf("the starter note carries %q, which belongs to the application", unwanted)
		}
	}
	// One header block, one marker, and nothing else.
	hb := parseHeaderBlock(starter)
	if !hb.HasHeader {
		t.Fatal("the starter has no header block")
	}
	if strings.TrimSpace(hb.Body) != incomingListMarker {
		t.Errorf("the starter body is not just the marker:\n%q", hb.Body)
	}
}
